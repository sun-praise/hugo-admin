package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============ BuildAssistPrompts ============

func TestBuildAssistPrompts(t *testing.T) {
	sys, user, err := BuildAssistPrompts("translate", "选区", "前文", "后文", "")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if !strings.Contains(sys, "翻译") {
		t.Fatalf("translate system prompt missing marker")
	}
	for _, marker := range []string{"<selection>\n选区\n</selection>", "<context_before>\n前文\n</context_before>", "<context_after>\n后文\n</context_after>"} {
		if !strings.Contains(user, marker) {
			t.Fatalf("translate user prompt missing %q", marker)
		}
	}
	if strings.Contains(user, "<document>") {
		t.Fatalf("translate user prompt must not contain document")
	}

	if _, user, err := BuildAssistPrompts("polish", "片段", "", "", ""); err != nil {
		t.Fatalf("polish: %v", err)
	} else if strings.Contains(user, "<context_before>") || strings.Contains(user, "<context_after>") {
		t.Fatalf("polish user prompt should omit empty contexts")
	}

	sysReview, userReview, err := BuildAssistPrompts("review", "片段", "", "", "全文内容")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !strings.Contains(sysReview, "overclaim") {
		t.Fatalf("review system prompt missing blog dimensions")
	}
	if !strings.Contains(userReview, "<document>\n全文内容\n</document>") ||
		!strings.Contains(userReview, "<selection>\n片段\n</selection>") {
		t.Fatalf("review user prompt missing document/selection blocks")
	}

	if _, _, err := BuildAssistPrompts("custom", "x", "", "", ""); err == nil {
		t.Fatalf("unsupported task should error")
	}
}

// ============ ExtractJSON ============

func TestExtractJSON(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // 期望字段 a 的值；"" 表示应返回 nil
	}{
		{"plain", `{"a": 1}`, "1"},
		{"fenced", "```json\n{\"a\": 1}\n```", "1"},
		{"prose wrapped", `以下是结果：{"a": {"b": 2}} 希望有帮助`, "obj"},
		{"trailing comma", `{"a": 1, "b": [1, 2,],}`, "1"},
	}
	for _, tc := range cases {
		obj := ExtractJSON(tc.raw)
		if obj == nil {
			t.Fatalf("%s: ExtractJSON(%q) = nil", tc.name, tc.raw)
		}
		if tc.name == "prose wrapped" {
			if _, ok := obj["a"].(map[string]any); !ok {
				t.Fatalf("%s: nested object not parsed: %#v", tc.name, obj)
			}
			continue
		}
		if obj["a"] != float64(1) {
			t.Fatalf("%s: a = %#v", tc.name, obj["a"])
		}
	}
	for _, raw := range []string{"抱歉，无法输出。", "", `{"a": [1, 2`} {
		if obj := ExtractJSON(raw); obj != nil {
			t.Fatalf("ExtractJSON(%q) = %#v, want nil", raw, obj)
		}
	}
}

// ============ normalizeTranslate ============

func TestNormalizeTranslate(t *testing.T) {
	raw := map[string]any{
		"translation": "The translation",
		"hard_words": []any{
			map[string]any{"word": "translation", "source": "翻译", "gloss": "n. 翻译"},
		},
	}
	result := normalizeTranslate(raw)
	if result == nil || result["translation"] != "The translation" {
		t.Fatalf("happy path: %#v", result)
	}
	words := result["hard_words"].([]any)
	if len(words) != 1 || words[0].(map[string]any)["word"] != "translation" {
		t.Fatalf("hard_words: %#v", words)
	}

	// 字段回退：zh/terms/label/en/note
	fallback := normalizeTranslate(map[string]any{
		"zh": "译文",
		"terms": []any{
			map[string]any{"label": "词", "en": "word", "note": "释义"},
		},
	})
	hw := fallback["hard_words"].([]any)[0].(map[string]any)
	if fallback["translation"] != "译文" || hw["word"] != "词" || hw["source"] != "word" || hw["gloss"] != "释义" {
		t.Fatalf("fallbacks: %#v", fallback)
	}

	// 难词上限 8
	many := map[string]any{"translation": "t"}
	list := []any{}
	for i := 0; i < 20; i++ {
		list = append(list, map[string]any{"word": "w"})
	}
	many["hard_words"] = list
	if got := normalizeTranslate(many)["hard_words"].([]any); len(got) != 8 {
		t.Fatalf("cap = %d, want 8", len(got))
	}

	// 缺 translation → nil
	if normalizeTranslate(map[string]any{"hard_words": []any{}}) != nil {
		t.Fatalf("missing translation should return nil")
	}
}

// ============ normalizePolish ============

func threeCandidates() []any {
	return []any{
		map[string]any{"style": "formal", "note": "a", "text": "A"},
		map[string]any{"style": "concise", "note": "b", "text": "B"},
		map[string]any{"style": "restructure", "note": "c", "text": "C"},
	}
}

func TestNormalizePolish(t *testing.T) {
	raw := map[string]any{"candidates": append(threeCandidates(),
		map[string]any{"style": "formal", "text": "D"})}
	result := normalizePolish(raw)
	candidates := result["candidates"].([]any)
	if len(candidates) != 3 {
		t.Fatalf("cap = %d, want 3", len(candidates))
	}
	if candidates[0].(map[string]any)["text"] != "A" {
		t.Fatalf("candidates[0] = %#v", candidates[0])
	}

	// style 缺省按位置回退
	fallback := normalizePolish(map[string]any{"candidates": []any{
		map[string]any{"text": "A"}, map[string]any{"text": "B"}, map[string]any{"text": "C"},
	}})
	styles := []string{}
	for _, c := range fallback["candidates"].([]any) {
		styles = append(styles, c.(map[string]any)["style"].(string))
	}
	if strings.Join(styles, ",") != "formal,concise,restructure" {
		t.Fatalf("style fallback = %v", styles)
	}

	// 不足 3 个 → nil（契约收紧）
	if normalizePolish(map[string]any{"candidates": threeCandidates()[:2]}) != nil {
		t.Fatalf("fewer than 3 candidates should return nil")
	}
	if normalizePolish(map[string]any{"candidates": []any{map[string]any{"note": "no text"}}}) != nil {
		t.Fatalf("no valid candidates should return nil")
	}
}

// ============ normalizeReview ============

func TestNormalizeReview(t *testing.T) {
	raw := map[string]any{"issues": []any{
		map[string]any{
			"type": "overclaim", "severity": "high", "comment": "断言过强",
			"before": "所有人都知道", "after": "多数读者认为",
		},
	}}
	issue := normalizeReview(raw)["issues"].([]any)[0].(map[string]any)
	if issue["before"] != "所有人都知道" || issue["after"] != "多数读者认为" {
		t.Fatalf("fix: %#v", issue)
	}

	// fix 对象回退 + severity/type 别名
	alias := normalizeReview(map[string]any{"issues": []any{
		map[string]any{
			"category": "terminology", "sev": "low", "detail": "术语不一致",
			"fix": map[string]any{"find": "缓存", "replace": "高速缓存"},
		},
	}})["issues"].([]any)[0].(map[string]any)
	if alias["type"] != "terminology" || alias["severity"] != "info" ||
		alias["before"] != "缓存" || alias["after"] != "高速缓存" {
		t.Fatalf("alias: %#v", alias)
	}

	// 未知 type/severity 回退；medium → warn
	unknown := normalizeReview(map[string]any{"issues": []any{
		map[string]any{"type": "weird", "severity": "medium", "comment": "c"},
	}})["issues"].([]any)[0].(map[string]any)
	if unknown["type"] != "other" || unknown["severity"] != "warn" {
		t.Fatalf("unknown fallback: %#v", unknown)
	}

	// before/after 必须成对
	pair := normalizeReview(map[string]any{"issues": []any{
		map[string]any{"comment": "c", "before": "片段"},
	}})["issues"].([]any)[0].(map[string]any)
	if pair["before"] != nil || pair["after"] != nil {
		t.Fatalf("unpaired fix should drop: %#v", pair)
	}

	// 空 issues 合法（选区没有问题）；非法 shape → nil
	if empty := normalizeReview(map[string]any{"issues": []any{}}); empty == nil {
		t.Fatalf("empty issues should be valid")
	}
	if normalizeReview(map[string]any{"summary": "no issues key"}) != nil {
		t.Fatalf("missing issues key should return nil")
	}
	if normalizeReview(map[string]any{"issues": "not-a-list"}) != nil {
		t.Fatalf("non-list issues should return nil")
	}
}

// ============ NormalizeFor ============

func TestNormalizeFor(t *testing.T) {
	if result, err := NormalizeFor("translate", map[string]any{"translation": "t"}); err != nil || result["translation"] != "t" {
		t.Fatalf("dispatch: %v %#v", err, result)
	}
	if _, err := NormalizeFor("polish", map[string]any{"candidates": []any{}}); err == nil {
		t.Fatalf("contract violation should error")
	}
	if _, err := NormalizeFor("custom", map[string]any{"text": "x"}); err == nil {
		t.Fatalf("unsupported task should error")
	}
}

// ============ 评审补缺：issue 上限 / AssistStream 取消退出 ============

func TestNormalizeReviewIssueCap(t *testing.T) {
	issues := []any{}
	for i := 0; i < 12; i++ {
		issues = append(issues, map[string]any{
			"type": "other", "severity": "info", "comment": fmt.Sprintf("c%d", i),
		})
	}
	got := normalizeReview(map[string]any{"issues": issues})["issues"].([]any)
	if len(got) != 8 {
		t.Fatalf("issues cap = %d, want 8", len(got))
	}
}

// mockAnthropicSlowSSE 每 50ms 一个 delta 的慢速提供商，用于取消场景。
func mockAnthropicSlowSSE(t *testing.T) *httptest.Server {
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
		for i := 0; i < 1000; i++ {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
			send("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"x"}}`)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestAssistStreamStopsOnContextCancel 验证 ctx 取消后事件通道能及时关闭
// （转发 goroutine 不依赖 trpc 是否关闭 events，防泄漏——评审 Should-fix 回归）。
func TestAssistStreamStopsOnContextCancel(t *testing.T) {
	srv := mockAnthropicSlowSSE(t)
	svc := New("test-key", srv.URL, "mock-model", Deps{ContentDir: t.TempDir()})

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := svc.AssistStream(ctx, "translate", "hello")
	if err != nil {
		t.Fatalf("AssistStream: %v", err)
	}

	// 至少收到一个事件，确认流已建立
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before first event")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event within 5s")
	}

	cancel()

	// 取消后通道必须及时关闭（拖走全部剩余事件的 drain 不得悬挂）
	done := make(chan struct{})
	go func() {
		for range ch { // drain 直到关闭
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("AssistStream channel not closed after ctx cancel (goroutine leak)")
	}
}
