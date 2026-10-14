package ai

// 回归测试：mock Anthropic SSE server 驱动完整链路，
// 逐帧断言 SSE 协议（对齐 Python ai_routes 的流格式）。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sun-praise/hugo-admin/v3/internal/git"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// newMockAnthropic 两轮：文本+tool_use → 工具结果回传 → 收尾文本。
func newMockAnthropic(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(raw)
		reqText := string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		send := func(event, data string) {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			flush.Flush()
		}
		send("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"mock","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		if strings.Contains(reqText, "tool_result") {
			send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"搜索完成。"}}`)
			send("content_block_stop", `{"type":"content_block_stop","index":0}`)
			send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		} else {
			send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
			send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"我来搜索。"}}`)
			send("content_block_stop", `{"type":"content_block_stop","index":0}`)
			send("content_block_start", `{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_001","name":"search_posts","input":{}}}`)
			send("content_block_delta", `{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"query\":\"go\"}"}}`)
			send("content_block_stop", `{"type":"content_block_stop","index":1}`)
			send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":5}}`)
		}
		send("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newTestService(t *testing.T, mock *httptest.Server) *Service {
	gitSvc, err := git.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return New("test-key", mock.URL, "mock-model", Deps{
		ContentDir: t.TempDir(),
		Git:        gitSvc,
	})
}

func TestChatSSEProtocol(t *testing.T) {
	mock := newMockAnthropic(t)
	svc := newTestService(t, mock)
	if !svc.Enabled() {
		t.Fatal("应启用")
	}

	ch, err := svc.ChatSSE(context.Background(), "搜索 go")
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	var frames []string
	for frame := range ch {
		frames = append(frames, frame)
	}

	want := []string{
		"data: 我来搜索。\n\n",
		`data: {"args":{"query":"go"},"tool":"search_posts","tool_call_id":"tu_001","type":"tool_call"}` + "\n\n",
		`data: {"result":"未找到匹配 'go' 的文章","tool_call_id":"tu_001","type":"tool_result"}` + "\n\n",
		"data: 搜索完成。\n\n",
		"data: [DONE]\n\n",
	}
	if len(frames) != len(want) {
		t.Fatalf("帧数 = %d: %#v", len(frames), frames)
	}
	for i := range want {
		if frames[i] != want[i] {
			t.Errorf("帧 %d\n got: %q\nwant: %q", i, frames[i], want[i])
		}
	}
}

func TestChatSSEMultiLineEscaped(t *testing.T) {
	// 文本含换行 → SSE 帧内转义为字面 \n
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		send := func(event, data string) {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			flush.Flush()
		}
		send("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"mock","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"第一行\n第二行\r\n"}}`)
		send("content_block_stop", `{"type":"content_block_stop","index":0}`)
		send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		send("message_stop", `{"type":"message_stop"}`)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	ch, err := svc.ChatSSE(context.Background(), "hi")
	if err != nil {
		t.Fatal(err)
	}
	var frames []string
	for f := range ch {
		frames = append(frames, f)
	}
	if len(frames) != 2 || frames[0] != "data: 第一行\\n第二行\\n\n\n" {
		t.Fatalf("frames = %#v", frames)
	}
	if frames[1] != "data: [DONE]\n\n" {
		t.Fatalf("结束帧 = %q", frames[1])
	}
}

func TestQuickRewrite(t *testing.T) {
	// 非流式：直接返回 JSON message
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"m1","type":"message","role":"assistant","content":[{"type":"text","text":"改写后的片段"}],"model":"mock","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":5}}`)
	}))
	defer srv.Close()

	svc := newTestService(t, srv)
	out, err := svc.QuickRewrite(context.Background(), "sys", "user", 5*time.Second)
	if err != nil || out != "改写后的片段" {
		t.Fatalf("rewrite = %q %v", out, err)
	}

	// 空结果
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"m2","type":"message","role":"assistant","content":[],"model":"mock","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":0}}`)
	}))
	defer empty.Close()
	svc2 := newTestService(t, empty)
	if _, err := svc2.QuickRewrite(context.Background(), "sys", "user", 5*time.Second); err == nil {
		t.Fatal("空结果应报错")
	}
}

func TestDisabledService(t *testing.T) {
	svc := New("", "", "", Deps{})
	if svc.Enabled() {
		t.Fatal("无 key 应禁用")
	}
	if _, err := svc.ChatSSE(context.Background(), "x"); err == nil {
		t.Fatal("禁用时应报错")
	}
}

// TestReadOnlyToolsBuilds 钉住只读工具暴露面：三个工具、名称固定。
func TestReadOnlyToolsBuilds(t *testing.T) {
	tools := readOnlyTools(Deps{ContentDir: t.TempDir()})
	if len(tools) != 3 {
		t.Fatalf("tools = %d, want 3", len(tools))
	}
	want := map[string]bool{"search_posts": false, "read_post": false, "git_status": false}
	for _, tl := range tools {
		name := tl.Declaration().Name
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("缺少工具 %q", name)
		}
	}
}

func TestSSEHelpers(t *testing.T) {
	// BuildInlinePrompts 拼装
	sys, user := BuildInlinePrompts("片段", "润色", "前文", "后文")
	if sys == "" || !strings.Contains(user, "片段") || !strings.Contains(user, "润色") ||
		!strings.Contains(user, "前文") || !strings.Contains(user, "后文") {
		t.Fatalf("sys=%q user=%q", sys, user)
	}

	// looksLikeMarkdown：空行、标记行、普通文本
	if !looksLikeMarkdown("  ") || !looksLikeMarkdown("# 标题") || !looksLikeMarkdown("- 列表") ||
		!looksLikeMarkdown("1. 有序") || !looksLikeMarkdown("**粗体**") || !looksLikeMarkdown("`code`") {
		t.Fatal("标记行应视为 Markdown")
	}
	if looksLikeMarkdown("普通文字一行") {
		t.Fatal("普通文字不应视为 Markdown")
	}

	// UnwrapSingleFence：单块解包 / 语言标签剥离 / 嵌套块不解 / 非块原样
	if got := UnwrapSingleFence("```go\nfmt.Println(1)\n```"); got != "fmt.Println(1)" {
		t.Fatalf("解包 = %q", got)
	}
	if got := UnwrapSingleFence("```\n纯文本\n```"); got != "纯文本" {
		t.Fatalf("无标签解包 = %q", got)
	}
	if got := UnwrapSingleFence("```\n外层\n```\n尾部"); got == "外层" {
		t.Fatal("非整体包裹应原样")
	}
	if got := UnwrapSingleFence("没有围栏"); got != "没有围栏" {
		t.Fatalf("普通文本 = %q", got)
	}

	// IsDangerousResponse：XSS 模式
	for _, dangerous := range []string{"<script>alert(1)</script>", "<IFRAME src=x>", "javascript:alert(1)", "<img onerror=alert(1)>"} {
		if !IsDangerousResponse(dangerous) {
			t.Fatalf("%q 应视为危险", dangerous)
		}
	}
	for _, safe := range []string{"普通 **Markdown**", "# 标题", "`code`"} {
		if IsDangerousResponse(safe) {
			t.Fatalf("%q 不应视为危险", safe)
		}
	}

	// decodeMaybeJSONString：JSON 字符串解码 / 原样返回
	if got := decodeMaybeJSONString(`"hello"`); got != "hello" {
		t.Fatalf("decode = %q", got)
	}
	if got := decodeMaybeJSONString("plain"); got != "plain" {
		t.Fatalf("非 JSON = %q", got)
	}
}

func TestModelName(t *testing.T) {
	s := New("k", "http://x", "my-model", Deps{ContentDir: t.TempDir()})
	if s.ModelName() != "my-model" {
		t.Fatalf("ModelName = %q", s.ModelName())
	}
	if !s.Enabled() {
		t.Fatal("配置了 apiKey + baseURL 的服务应启用")
	}
}

// TestReadOnlyToolsCallable 直接经 Call 调用工具闭包，钉住三个只读工具的行为。
func TestReadOnlyToolsCallable(t *testing.T) {
	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "post", "hello"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "post", "hello", "index.md"),
		[]byte("---\ntitle: Hello\n---\n正文"), 0o644); err != nil {
		t.Fatal(err)
	}

	gitSvc, err := git.New(contentDir, nil)
	if err != nil {
		t.Fatalf("git service: %v", err)
	}
	tools := readOnlyTools(Deps{ContentDir: contentDir, Git: gitSvc})
	call := func(t *testing.T, tl tool.Tool, args string) string {
		t.Helper()
		ct, ok := tl.(interface {
			Call(context.Context, []byte) (any, error)
		})
		if !ok {
			t.Fatalf("工具不可调用: %T", tl)
		}
		out, err := ct.Call(context.Background(), []byte(args))
		if err != nil {
			t.Fatalf("Call(%s): %v", args, err)
		}
		return fmt.Sprint(out)
	}

	// search_posts：命中与未命中
	if out := call(t, tools[0], `{"query":"hello"}`); !strings.Contains(out, "Hello") {
		t.Fatalf("search 命中 = %q", out)
	}
	if out := call(t, tools[0], `{"query":"不存在关键词xyz"}`); !strings.Contains(out, "未找到") {
		t.Fatalf("search 未命中 = %q", out)
	}

	// read_post：成功与失败
	if out := call(t, tools[1], `{"file_path":"post/hello/index.md"}`); !strings.Contains(out, "正文") {
		t.Fatalf("read = %q", out)
	}
	if out := call(t, tools[1], `{"file_path":"post/none.md"}`); !strings.Contains(out, "读取失败") {
		t.Fatalf("read 失败 = %q", out)
	}

	// git_status：真实 Service（temp 目录非 git 仓库，输出字段仍应成形）
	if out := call(t, tools[2], `{}`); !strings.Contains(out, "Git 仓库状态") {
		t.Fatalf("git_status = %q", out)
	}
}
