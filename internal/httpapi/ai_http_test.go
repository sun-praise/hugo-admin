package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/ai"
	"github.com/svtter/hugo-admin/internal/chathistory"
	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/git"
)

// mockAnthropicTextOnly：按请求 stream 与否返回 SSE 流或 JSON。
func mockAnthropicTextOnly(t *testing.T, text string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(raw)
		if !strings.Contains(string(raw), `"stream":true`) {
			// 非流式（QuickRewrite）
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"id":"m1","type":"message","role":"assistant","content":[{"type":"text","text":%s}],"model":"mock","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":5}}`, mustJSON(text))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		send := func(event, data string) {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			flush.Flush()
		}
		send("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"mock","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		send("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, mustJSON(text)))
		send("content_block_stop", `{"type":"content_block_stop","index":0}`)
		send("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`)
		send("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newAITestServer(t *testing.T, mock *httptest.Server) *httptest.Server {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	gitSvc, err := git.New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var aiSvc *ai.Service
	if mock != nil {
		aiSvc = ai.New("test-key", mock.URL, "mock-model", ai.Deps{ContentDir: t.TempDir(), Git: gitSvc})
	} else {
		aiSvc = ai.New("", "", "", ai.Deps{})
	}
	return newTestServerOpts(t, "", Options{
		Database: database,
		AI:       aiSvc,
		Chat:     chathistory.New(database),
	})
}

func TestAISessionsCRUD(t *testing.T) {
	ts := newAITestServer(t, nil)
	client := ts.Client()
	login(t, client, ts.URL)

	// 创建（默认标题）
	resp, body := doJSON(t, client, "POST", ts.URL+"/api/ai/sessions", map[string]string{})
	if resp != 201 {
		t.Fatalf("create status = %d %v", resp, body)
	}
	if body["title"] != "新对话" || body["message_count"] != float64(0) {
		t.Fatalf("create body = %#v", body)
	}
	sessionID := body["session_id"].(string)

	// 指定标题
	_, body = doJSON(t, client, "POST", ts.URL+"/api/ai/sessions", map[string]string{"title": "自定义"})
	if body["title"] != "自定义" {
		t.Fatalf("title = %#v", body["title"])
	}

	// 列表（倒序：自定义在前）
	_, body = doJSON(t, client, "GET", ts.URL+"/api/ai/sessions", nil)
	sessions := body["sessions"].([]any)
	if len(sessions) != 2 || sessions[0].(map[string]any)["title"] != "自定义" {
		t.Fatalf("sessions = %#v", sessions)
	}

	// 获取单个
	status, body := doJSON(t, client, "GET", ts.URL+"/api/ai/sessions/"+sessionID, nil)
	if status != 200 || body["message_count"] != float64(0) {
		t.Fatalf("get = %d %#v", status, body)
	}
	// 不存在
	status, _ = doJSON(t, client, "GET", ts.URL+"/api/ai/sessions/none", nil)
	if status != 404 {
		t.Fatalf("404 = %d", status)
	}

	// 删除
	status, body = doJSON(t, client, "DELETE", ts.URL+"/api/ai/sessions/"+sessionID, nil)
	if status != 200 || body["message"] != "Session deleted" {
		t.Fatalf("delete = %d %#v", status, body)
	}
}

func TestAIChatSSEWithSession(t *testing.T) {
	mock := mockAnthropicTextOnly(t, "这是回答。")
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	_, session := doJSON(t, client, "POST", ts.URL+"/api/ai/sessions", map[string]string{})
	sessionID := session["session_id"].(string)

	// 发起聊天（流式）
	req, _ := http.NewRequest("POST", ts.URL+"/api/ai/chat",
		strings.NewReader(mustJSON(map[string]any{
			"message":      "帮我找一篇关于部署的文章",
			"session_id":   sessionID,
			"current_file": "post/hello.md",
			"current_page": "editor",
		})))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	// 循环读流直到出现 [DONE]
	stream := ""
	buf := make([]byte, 4096)
	for !strings.Contains(stream, "[DONE]") {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			stream += string(buf[:n])
		}
		if err != nil {
			break
		}
	}
	if !strings.Contains(stream, "data: 这是回答。\n\n") || !strings.HasSuffix(stream, "data: [DONE]\n\n") {
		t.Fatalf("stream = %q", stream)
	}

	// 会话持久化：user 消息（含上下文前缀，自动标题）+ assistant 回答
	_, body := doJSON(t, client, "GET", ts.URL+"/api/ai/sessions/"+sessionID, nil)
	msgs := body["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messages = %#v", msgs)
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "user" || !strings.HasPrefix(first["content"].(string), "[当前页面: editor, 当前聚焦文章: post/hello.md]") {
		t.Fatalf("user msg = %#v", first)
	}
	second := msgs[1].(map[string]any)
	if second["role"] != "assistant" || second["content"] != "这是回答。" {
		t.Fatalf("assistant msg = %#v", second)
	}
	// 自动标题生效
	if !strings.HasPrefix(body["title"].(string), "[当前页面") {
		t.Fatalf("title = %#v", body["title"])
	}
}

func TestAIChatDisabled(t *testing.T) {
	ts := newAITestServer(t, nil)
	client := ts.Client()
	login(t, client, ts.URL)
	status, body := doJSON(t, client, "POST", ts.URL+"/api/ai/chat", map[string]string{"message": "hi"})
	if status != 503 {
		t.Fatalf("status = %d %v", status, body)
	}
	status, _ = doJSON(t, client, "POST", ts.URL+"/api/ai/inline-edit", map[string]string{
		"selected_text": "x", "instruction": "y"})
	if status != 503 {
		t.Fatalf("inline-edit status = %d", status)
	}
	// 缺消息
	enabled := newAITestServer(t, mockAnthropicTextOnly(t, "ok"))
	c2 := enabled.Client()
	login(t, c2, enabled.URL)
	status, body = doJSON(t, c2, "POST", enabled.URL+"/api/ai/chat", map[string]string{})
	if status != 400 || body["message"] != "缺少消息内容" {
		t.Fatalf("missing message = %d %v", status, body)
	}
}

func TestInlineEditFlow(t *testing.T) {
	mock := mockAnthropicTextOnly(t, "```markdown\n改写后的片段\n```")
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	// 成功：fence 解包
	status, body := doJSON(t, client, "POST", ts.URL+"/api/ai/inline-edit", map[string]string{
		"selected_text": "原文", "instruction": "更简洁", "context_before": "前", "context_after": "后",
	})
	if status != 200 || body["revised_text"] != "改写后的片段" || body["model"] != "mock-model" {
		t.Fatalf("inline-edit = %d %#v", status, body)
	}

	// 参数校验
	status, body = doJSON(t, client, "POST", ts.URL+"/api/ai/inline-edit", map[string]string{"instruction": "x"})
	if status != 400 || body["message"] != "缺少 selected_text" {
		t.Fatalf("missing = %d %v", status, body)
	}
	status, body = doJSON(t, client, "POST", ts.URL+"/api/ai/inline-edit", map[string]string{"selected_text": "x"})
	if status != 400 || body["message"] != "缺少 instruction" {
		t.Fatalf("missing = %d %v", status, body)
	}

	// 超长
	long := strings.Repeat("字", 5001)
	status, body = doJSON(t, client, "POST", ts.URL+"/api/ai/inline-edit", map[string]string{
		"selected_text": long, "instruction": "x"})
	if status != 400 || !strings.Contains(body["message"].(string), "selected_text 超过") {
		t.Fatalf("too long = %d %v", status, body)
	}

	// XSS 拒绝
	danger := newAITestServer(t, mockAnthropicTextOnly(t, "正常 <script>alert(1)</script>"))
	c3 := danger.Client()
	login(t, c3, danger.URL)
	status, body = doJSON(t, c3, "POST", danger.URL+"/api/ai/inline-edit", map[string]string{
		"selected_text": "x", "instruction": "y"})
	if status != 500 || body["message"] != "改写失败：模型返回不安全内容" {
		t.Fatalf("xss = %d %v", status, body)
	}
}
