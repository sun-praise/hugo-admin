package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sun-praise/hugo-admin/v3/internal/ai"
	"github.com/sun-praise/hugo-admin/v3/internal/chathistory"
	"github.com/sun-praise/hugo-admin/v3/internal/db"
	"github.com/sun-praise/hugo-admin/v3/internal/settings"
)

// TestAIConfigSharedAcrossFeatures 钉住统一配置：设置页保存的 key 同时作用于
// frontmatter 生成与选区辅助，且无需重启服务。
func TestAIConfigSharedAcrossFeatures(t *testing.T) {
	var mu sync.Mutex
	var authHeaders []string
	var paths []string
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		raw, _ := io.ReadAll(r.Body)
		body := string(raw)
		if strings.Contains(body, `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			flush := w.(http.Flusher)
			send := func(data string) {
				fmt.Fprintf(w, "data: %s\n\n", data)
				flush.Flush()
			}
			send(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mock","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`)
			send(fmt.Sprintf(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mock","choices":[{"index":0,"delta":{"content":%s},"finish_reason":null}]}`,
				mustJSON(`{"translation":"Hello world","hard_words":[]}`)))
			send(`{"id":"chatcmpl-1","object":"chat.completion.chunk","model":"mock","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`)
			fmt.Fprint(w, "data: [DONE]\n\n")
			flush.Flush()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-2","object":"chat.completion","model":"mock","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`,
			mustJSON(`{"description":"共享配置摘要","tags":["ai"],"categories":["tech"]}`))
	}))
	t.Cleanup(mock.Close)

	database, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	settingsSvc := settings.NewService(filepath.Join(t.TempDir(), "settings.json"), map[string]string{
		"AI_BASE_URL": mock.URL,
		"AI_MODEL":    "mock-model",
	}, "")
	store := ai.NewConfigStore(settingsSvc, "")
	ts := newTestServerOpts(t, t.TempDir(), Options{
		Database: database,
		AIConfig: store,
		Settings: settingsSvc,
		Chat:     chathistory.New(database),
	})
	client := ts.Client()
	login(t, client, ts.URL)

	// 未配置 key：两个功能都应禁用。
	resp, err := client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
		strings.NewReader(`{"selected_text": "你好"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("assist before key = %d", resp.StatusCode)
	}
	status, _ := doJSON(t, client, "POST", ts.URL+"/api/frontmatter/generate",
		map[string]any{"content": "文章正文"})
	if status != http.StatusBadRequest {
		t.Fatalf("frontmatter before key = %d", status)
	}

	// 设置页保存 key：不重启，下一次请求立即生效。
	status, body := doJSON(t, client, "PUT", ts.URL+"/api/settings", map[string]any{
		"settings": map[string]any{
			"ai": map[string]any{"api_key": "sess-key-abcd1234"},
		},
	})
	if status != http.StatusOK {
		t.Fatalf("save settings = %d %#v", status, body)
	}
	aiPublic := body["settings"].(map[string]any)["ai"].(map[string]any)
	if aiPublic["api_key_source"] != "session" || aiPublic["api_key_hint"] != "sess...1234" {
		t.Fatalf("ai public = %#v", aiPublic)
	}

	// 选区辅助走同一份配置。
	resp, err = client.Post(ts.URL+"/api/ai/assist/translate", "application/json",
		strings.NewReader(`{"selected_text": "你好"}`))
	if err != nil {
		t.Fatal(err)
	}
	stream, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("assist after key = %d", resp.StatusCode)
	}
	if !strings.Contains(string(stream), `"translation":"Hello world"`) {
		t.Fatalf("assist stream = %q", stream)
	}

	// frontmatter 生成走同一份配置。
	status, body = doJSON(t, client, "POST", ts.URL+"/api/frontmatter/generate",
		map[string]any{"content": "文章正文"})
	if status != http.StatusOK {
		t.Fatalf("frontmatter after key = %d %#v", status, body)
	}
	fm := body["frontmatter"].(map[string]any)
	if fm["description"] != "共享配置摘要" {
		t.Fatalf("frontmatter = %#v", fm)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(authHeaders) < 2 {
		t.Fatalf("expected both features to call provider, got %d", len(authHeaders))
	}
	for _, h := range authHeaders {
		if h != "Bearer sess-key-abcd1234" {
			t.Fatalf("Authorization = %q", h)
		}
	}
	for _, p := range paths {
		if !strings.HasSuffix(p, "/chat/completions") {
			t.Fatalf("provider path = %q", p)
		}
	}

}
