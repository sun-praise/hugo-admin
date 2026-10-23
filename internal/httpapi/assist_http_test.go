package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// assistTranslateJSON 是 mock provider 返回的模型原始输出（严格 JSON）。
func assistTranslateJSON() string {
	return `{"translation": "Hello world", "hard_words": [{"word": "Hello", "source": "你好", "gloss": "int. 打招呼"}]}`
}

func assistSSEEvents(t *testing.T, body string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, frame := range strings.Split(body, "\n\n") {
		if !strings.HasPrefix(frame, "data: ") {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, "data: ")), &payload); err != nil {
			t.Fatalf("bad SSE frame %q: %v", frame, err)
		}
		events = append(events, payload)
	}
	return events
}

func TestAssistTranslateHappyPath(t *testing.T) {
	mock := mockAnthropicTextOnly(t, assistTranslateJSON())
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
		strings.NewReader(`{"selected_text": "你好", "context_before": "前", "context_after": "后"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}
	raw, _ := io.ReadAll(resp.Body)

	events := assistSSEEvents(t, string(raw))
	types := []string{}
	for _, e := range events {
		types = append(types, e["type"].(string))
	}
	if len(types) < 3 || types[0] != "start" || types[len(types)-1] != "done" {
		t.Fatalf("event types = %v", types)
	}
	for _, ty := range types[1 : len(types)-1] {
		if ty != "delta" {
			t.Fatalf("middle events must be delta, got %v", types)
		}
	}
	start := events[0]
	if start["task"] != "translate" || start["model"] == "" {
		t.Fatalf("start = %#v", start)
	}
	// delta 逐帧拼接必须还原模型原始输出（防截断/转义破坏）
	var joined []byte
	for _, e := range events[1 : len(events)-1] {
		joined = append(joined, e["text"].(string)...)
	}
	if string(joined) != assistTranslateJSON() {
		t.Fatalf("delta join = %q, want %q", joined, assistTranslateJSON())
	}
	done := events[len(events)-1]
	result := done["result"].(map[string]any)
	if result["translation"] != "Hello world" {
		t.Fatalf("done result = %#v", result)
	}
}

func TestAssistPolishFewerThanThreeEndsWithError(t *testing.T) {
	mock := mockAnthropicTextOnly(t, `{"candidates": [{"style": "formal", "text": "A"}, {"style": "concise", "text": "B"}]}`)
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Post(ts.URL+"/api/ai/assist/polish", "application/json",
		strings.NewReader(`{"selected_text": "片段"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	events := assistSSEEvents(t, string(raw))
	last := events[len(events)-1]
	if last["type"] != "error" {
		t.Fatalf("last event = %#v", last)
	}
	if last["task"] != "polish" {
		t.Fatalf("error event missing task: %#v", last)
	}
}

func TestAssistReviewEmptyIssuesIsValid(t *testing.T) {
	mock := mockAnthropicTextOnly(t, `{"issues": []}`)
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Post(ts.URL+"/api/ai/assist/review", "application/json",
		strings.NewReader(`{"selected_text": "片段", "full_text": "整篇文档"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	events := assistSSEEvents(t, string(raw))
	last := events[len(events)-1]
	if last["type"] != "done" {
		t.Fatalf("last event = %#v", last)
	}
	result := last["result"].(map[string]any)
	if _, ok := result["issues"].([]any); !ok {
		t.Fatalf("result.issues missing: %#v", result)
	}
}

func TestAssistValidation(t *testing.T) {
	t.Run("unknown task 404", func(t *testing.T) {
		ts := newAITestServer(t, nil)
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/summarize", "application/json",
			strings.NewReader(`{"selected_text": "x"}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("missing selected_text 400", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(`{}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("oversized selected_text 400", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		oversized := strings.Repeat("选", 5001)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(fmt.Sprintf(`{"selected_text": %q}`, oversized)))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("oversized context 400", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(fmt.Sprintf(`{"selected_text": "x", "context_before": %q}`, strings.Repeat("前", 2001))))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("review without full_text 400", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/review", "application/json",
			strings.NewReader(`{"selected_text": "x"}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("review oversized full_text 400", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/review", "application/json",
			strings.NewReader(fmt.Sprintf(`{"selected_text": "x", "full_text": %q}`, strings.Repeat("长", 30001))))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("ai not configured 503", func(t *testing.T) {
		ts := newAITestServer(t, nil) // 无 mock → AI 未配置
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(`{"selected_text": "x"}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 503 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("unauthenticated 401", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		resp, err := ts.Client().Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(`{"selected_text": "x"}`))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		// 全局认证守卫：未登录一律 401（前端 401 拦截器依赖该语义）
		if resp.StatusCode != 401 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})
}

// ============ 评审补缺：边界正例 / 非 JSON / 断流上游 ============

// TestAssistBoundaryAccepts 恰好卡在上限的输入必须放行（与超限 400 成对）。
func TestAssistBoundaryAccepts(t *testing.T) {
	t.Run("selected_text exactly at limit", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(fmt.Sprintf(`{"selected_text": %q}`, strings.Repeat("选", 5000))))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("context exactly at limit", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
			strings.NewReader(fmt.Sprintf(
				`{"selected_text": "x", "context_before": %q, "context_after": %q}`,
				strings.Repeat("前", 2000), strings.Repeat("后", 2000))))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})

	t.Run("full_text exactly at limit", func(t *testing.T) {
		ts := newAITestServer(t, mockAnthropicTextOnly(t, "{}"))
		client := ts.Client()
		login(t, client, ts.URL)
		resp, err := client.Post(ts.URL+"/api/ai/assist/review", "application/json",
			strings.NewReader(fmt.Sprintf(
				`{"selected_text": "x", "full_text": %q}`, strings.Repeat("文", 30000))))
		if err != nil {
			t.Fatalf("post: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
	})
}

// TestAssistNonJSONOutputEndsWithError 模型输出不含 JSON → error 事件（非 done）。
func TestAssistNonJSONOutputEndsWithError(t *testing.T) {
	mock := mockAnthropicTextOnly(t, "抱歉，我无法输出 JSON。")
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
		strings.NewReader(`{"selected_text": "x"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	events := assistSSEEvents(t, string(raw))
	for _, e := range events {
		if e["type"] == "done" {
			t.Fatalf("non-JSON output must not produce done: %#v", events)
		}
	}
	last := events[len(events)-1]
	if last["type"] != "error" {
		t.Fatalf("last event = %#v", last)
	}
}

// mockAnthropicBrokenMidStream 在发出半截 JSON 后强制断开连接，
// 模拟流式中途上游失败。
func mockAnthropicBrokenMidStream(t *testing.T, partial string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flush := w.(http.Flusher)
		send := func(event, data string) {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
			flush.Flush()
		}
		send("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"mock","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":1}}}`)
		send("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		send("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, mustJSON(partial)))
		if hj, ok := w.(http.Hijacker); ok {
			if conn, _, err := hj.Hijack(); err == nil {
				_ = conn.Close()
				return
			}
		}
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAssistMidStreamUpstreamFailureEndsWithError 断流后必须以 error 结束，
// 不得把半截输出规整成 done。
func TestAssistMidStreamUpstreamFailureEndsWithError(t *testing.T) {
	mock := mockAnthropicBrokenMidStream(t, `{"translation": "半截输出`)
	ts := newAITestServer(t, mock)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
		strings.NewReader(`{"selected_text": "x"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	events := assistSSEEvents(t, string(raw))
	for _, e := range events {
		if e["type"] == "done" {
			t.Fatalf("broken stream must not produce done: %#v", events)
		}
	}
	last := events[len(events)-1]
	if last["type"] != "error" {
		t.Fatalf("last event = %#v", last)
	}
}
