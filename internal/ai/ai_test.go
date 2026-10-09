package ai

// 回归测试：mock Anthropic SSE server 驱动完整链路，
// 逐帧断言 SSE 协议（对齐 Python ai_routes 的流格式）。

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/svtter/hugo-admin/internal/git"
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
