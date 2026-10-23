package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 机器客户端 Bearer 鉴权：API_TOKEN 配置后可等价通过全局守卫。
func TestBearerTokenGuard(t *testing.T) {
	ts := newTestServerOpts(t, "", Options{APIToken: "agent-secret"})
	client := ts.Client()

	// 未登录且无 token → 401
	status, _ := doJSON(t, client, "GET", ts.URL+"/api/posts", nil)
	if status != 401 {
		t.Fatalf("no token = %d", status)
	}

	// 错误 token → 401
	req, _ := http.NewRequest("GET", ts.URL+"/api/posts", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("wrong token = %d", resp.StatusCode)
	}

	// 正确 token → 200
	req, _ = http.NewRequest("GET", ts.URL+"/api/posts", nil)
	req.Header.Set("Authorization", "Bearer agent-secret")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("correct token = %d", resp.StatusCode)
	}
}

// API_TOKEN 未配置时 Bearer 通道关闭。
func TestBearerTokenDisabledByDefault(t *testing.T) {
	ts := newTestServerOpts(t, "", Options{})
	req, _ := http.NewRequest("GET", ts.URL+"/api/posts", nil)
	req.Header.Set("Authorization", "Bearer anything")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("token unset = %d, 应拒绝", resp.StatusCode)
	}
}

// JSON 模式导入：默认草稿、AI 开关默认关（无 OPENROUTER 警告），
// 已有 frontmatter 的 title 保留。
func TestArticleImportJSON(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	contentDir := t.TempDir()
	ts := newTestServerOpts(t, contentDir, Options{})
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/article/import", map[string]any{
		"filename": "agent-note.md",
		"content":  "---\ntitle: Agent 投递\n---\n\n正文内容。",
	})
	if status != 200 || body["success"] != true {
		t.Fatalf("import = %d %#v", status, body)
	}
	path, _ := body["path"].(string)
	if !strings.HasPrefix(path, "post/") || !strings.HasSuffix(path, "/index.md") {
		t.Fatalf("path = %q", path)
	}
	if w := body["warnings"].([]any); len(w) != 0 {
		t.Fatalf("warnings = %#v, 默认不应触发 AI 开关", w)
	}
	raw, err := os.ReadFile(filepath.Join(contentDir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"title: Agent 投递", "draft: true", "正文内容。"} {
		if !strings.Contains(text, want) {
			t.Fatalf("file 缺少 %q:\n%s", want, text)
		}
	}
}

// draft:false 可显式放行；Bearer token 可替代会话调用导入；
// 无 frontmatter 时标题从首个 H1 派生。
func TestArticleImportJSONDraftFalseWithToken(t *testing.T) {
	contentDir := t.TempDir()
	ts := newTestServerOpts(t, contentDir, Options{APIToken: "agent-secret"})

	req, _ := http.NewRequest("POST", ts.URL+"/api/article/import",
		strings.NewReader(mustJSON(map[string]any{
			"content": "# 无 frontmatter 标题\n\n内容",
			"draft":   false,
		})))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer agent-secret")
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 200 || body["success"] != true {
		t.Fatalf("import = %d %#v", resp.StatusCode, body)
	}
	raw, err := os.ReadFile(filepath.Join(contentDir, filepath.FromSlash(body["path"].(string))))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"draft: false", "title: 无 frontmatter 标题"} {
		if !strings.Contains(text, want) {
			t.Fatalf("file 缺少 %q:\n%s", want, text)
		}
	}
}

// 参数校验：空 content、非法扩展名。
func TestArticleImportJSONValidation(t *testing.T) {
	ts := newTestServerOpts(t, "", Options{})
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/article/import", map[string]any{"content": "  "})
	if status != 400 || !strings.Contains(body["message"].(string), "content") {
		t.Fatalf("empty content = %d %#v", status, body)
	}
	status, body = doJSON(t, client, "POST", ts.URL+"/api/article/import", map[string]any{
		"content": "x", "filename": "post.txt"})
	if status != 400 || !strings.Contains(body["message"].(string), "txt") {
		t.Fatalf("bad ext = %d %#v", status, body)
	}
}
