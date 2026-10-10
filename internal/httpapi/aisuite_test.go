package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/aigen"
	"github.com/svtter/hugo-admin/internal/settings"
)

// mockChatServer：OpenAI 兼容 chat/completions（frontmatter 生成用）。
func mockChatServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if strings.HasSuffix(r.URL.Path, "/chat/completions") {
			// 记录请求供断言
			fmt.Fprintf(w, `{"choices":[{"message":{"content":%s}}]}`, mustJSON(content))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockOpenRouterImage：images 数组返回 data URL 图片。
func mockOpenRouterImage(t *testing.T, pngBytes []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
		fmt.Fprintf(w, `{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":%s}}]}}]}`,
			mustJSON(dataURL))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// settingsWithAI 构造指向 mock chat 的设置服务。
func settingsWithAI(t *testing.T, baseURL, model string) *settings.Service {
	return settings.NewService(filepath.Join(t.TempDir(), "settings.json"), map[string]string{
		"AI_BASE_URL": baseURL,
		"AI_MODEL":    model,
	}, "")
}

func extractImageFromURLForTest(url string) ([]byte, bool) { return aigen.ExtractImageFromURL(url) }

func TestSanitizeFrontmatter(t *testing.T) {
	fm := aigen.SanitizeFrontmatter(map[string]any{
		"description": "  摘要  ",
		"tags":        []any{"go", " ", 42, "web", "extra1", "extra2", "extra3"},
		"categories":  []any{"tech", "life", "other"},
		"other":       "ignored",
	})
	if fm["description"] != "摘要" {
		t.Fatalf("desc = %v", fm["description"])
	}
	tags := fm["tags"].([]string)
	if len(tags) != 5 || tags[0] != "go" { // 空白与数字剔除，5 个上限
		t.Fatalf("tags = %#v", tags)
	}
	cats := fm["categories"].([]string)
	if len(cats) != 2 {
		t.Fatalf("cats = %#v", cats)
	}
	if _, exists := fm["other"]; exists {
		t.Fatal("未知字段应被剔除")
	}
}

func TestFMGenerateEndpoint(t *testing.T) {
	// mock chat（经 settings 的 base_url 指向）
	chat := mockChatServer(t, `{"description":"AI 摘要","tags":["ai"],"categories":["tech"]}`)

	contentDir := t.TempDir()
	ts := newTestServerOpts(t, contentDir, Options{
		EnvAPIKey: "test-key",
		Settings:  settingsWithAI(t, chat.URL, "mock-model"),
	})
	client := ts.Client()
	login(t, client, ts.URL)

	// 成功
	status, body := doJSON(t, client, "POST", ts.URL+"/api/frontmatter/generate",
		map[string]any{"content": "这是文章内容，足够长用于生成。"})
	if status != 200 || body["success"] != true {
		t.Fatalf("fm = %d %#v", status, body)
	}
	fm := body["frontmatter"].(map[string]any)
	if fm["description"] != "AI 摘要" {
		t.Fatalf("fm = %#v", fm)
	}

	// 空 content → 400
	status, body = doJSON(t, client, "POST", ts.URL+"/api/frontmatter/generate",
		map[string]any{"content": "  "})
	if status != 400 || body["message"] != "文章内容为空" {
		t.Fatalf("empty = %d %#v", status, body)
	}

	// 未配置 key → 400
	noKey := newTestServerOpts(t, t.TempDir(), Options{})
	c2 := noKey.Client()
	login(t, c2, noKey.URL)
	status, _ = doJSON(t, c2, "POST", noKey.URL+"/api/frontmatter/generate",
		map[string]any{"content": "x"})
	if status != 400 {
		t.Fatalf("no key = %d", status)
	}
}

func TestGenerateCoverEndpoint(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	contentDir := t.TempDir()
	os.MkdirAll(filepath.Join(contentDir, "post", "a"), 0o755)
	os.WriteFile(filepath.Join(contentDir, "post", "a", "index.md"), []byte("# A\n"), 0o644)

	// 注：OpenRouter URL 硬编码在 aigen 中，端点无法指向 mock——
	// 未配置 key 路径可测；extractImageFromURL 的 data URL 形态
	// 由下方直调覆盖
	png := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47}, 20)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	if b, ok := extractImageFromURLForTest(dataURL); !ok || !bytes.Equal(b, png) {
		t.Fatalf("data URL 提取失败")
	}

	ts := newTestServerWithContent(t, contentDir)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/image/generate-cover",
		map[string]any{"article_path": "post/a/index.md"})
	if status != 400 || !strings.Contains(body["message"].(string), "OPENROUTER_API_KEY") {
		t.Fatalf("no key = %d %#v", status, body)
	}

	// 缺 article_path
	status, _ = doJSON(t, client, "POST", ts.URL+"/api/image/generate-cover", map[string]any{})
	if status != 400 {
		t.Fatalf("missing path = %d", status)
	}
}

func TestArticleImport(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	chat := mockChatServer(t, `{"description":"导入摘要","tags":["imported"]}`)

	contentDir := t.TempDir()
	ts := newTestServerOpts(t, contentDir, Options{
		EnvAPIKey: "test-key",
		Settings:  settingsWithAI(t, chat.URL, "mock-model"),
	})
	client := ts.Client()
	login(t, client, ts.URL)

	// 导入带 H1 的 markdown
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "我的导入文章.md")
	fw.Write([]byte("# 导入的文章标题\n\n这是导入的正文内容。\n"))
	mw.Close()

	resp, err := client.Post(ts.URL+"/api/article/import", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != 200 || body["success"] != true {
		t.Fatalf("import = %d %#v", resp.StatusCode, body)
	}
	path := body["path"].(string)
	if !strings.HasPrefix(path, "post/") || !strings.HasSuffix(path, "/index.md") {
		t.Fatalf("path = %q", path)
	}
	// 警告应含 OPENROUTER key 未配置（封面跳过）
	warnings := body["warnings"].([]any)
	found := false
	for _, w := range warnings {
		if strings.Contains(w.(string), "OPENROUTER") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings = %#v", warnings)
	}

	// 文件内容验证：草稿 + 标题 + AI 富化
	raw, _ := os.ReadFile(filepath.Join(contentDir, filepath.FromSlash(path)))
	text := string(raw)
	for _, want := range []string{
		"title: 导入的文章标题",
		"draft: true",
		"description: 导入摘要",
		"- imported",
		"这是导入的正文内容。",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q:\n%s", want, text)
		}
	}

	// 不支持的类型
	buf.Reset()
	mw = multipart.NewWriter(&buf)
	fw, _ = mw.CreateFormFile("file", "x.txt")
	fw.Write([]byte("text"))
	mw.Close()
	resp2, _ := client.Post(ts.URL+"/api/article/import", mw.FormDataContentType(), &buf)
	defer resp2.Body.Close()
	if resp2.StatusCode != 400 {
		t.Fatalf("bad type = %d", resp2.StatusCode)
	}
}
