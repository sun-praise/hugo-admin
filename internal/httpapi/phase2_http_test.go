package httpapi

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sun-praise/hugo-admin/v3/internal/aigen"
	"github.com/sun-praise/hugo-admin/v3/internal/db"
	"github.com/sun-praise/hugo-admin/v3/internal/plugin"
)

// fakeHugoBin 把输出日志后挂住的假 hugo 放进 PATH（httpapi 无法直改 Manager 私有字段）。
func fakeHugoBin(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho 'Web Server is available at http://localhost:1313/'\nwhile true; do sleep 1; done\n"
	if err := os.WriteFile(filepath.Join(bin, "hugo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestHealthWithoutLogin(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health = %d", resp.StatusCode)
	}
}

func TestServerLifecycleHandlers(t *testing.T) {
	fakeHugoBin(t)
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	get := func(path string) (int, map[string]any) {
		return doJSON(t, client, "GET", ts.URL+path, nil)
	}

	status, body := get("/api/server/status")
	if status != 200 || body["running"] != false {
		t.Fatalf("初始 status: %d %v", status, body)
	}

	status, body = doJSON(t, client, "POST", ts.URL+"/api/server/start", map[string]any{})
	if status != 200 || body["success"] != true {
		t.Fatalf("start: %d %v", status, body)
	}
	if st, _ := body["status"].(map[string]any); st["running"] != true {
		t.Fatalf("start 后 status = %v", body["status"])
	}

	status, body = get("/api/server/logs?count=10")
	if status != 200 {
		t.Fatalf("logs: %d", status)
	}
	logs, _ := body["logs"].([]any)
	if len(logs) == 0 {
		t.Fatal("应有启动日志")
	}

	status, body = doJSON(t, client, "POST", ts.URL+"/api/server/stop", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("stop: %d %v", status, body)
	}
	status, body = get("/api/server/status")
	if status != 200 || body["running"] != false {
		t.Fatalf("stop 后 status: %d %v", status, body)
	}
}

func TestServerStartHugoMissing(t *testing.T) {
	// PATH 里没有 hugo → 启动失败并提示安装
	t.Setenv("PATH", t.TempDir())
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/server/start", nil)
	if status != 200 || body["success"] != false {
		t.Fatalf("start: %d %v", status, body)
	}
	if msg, _ := body["message"].(string); msg == "" {
		t.Fatal("失败应带 message")
	}
}

func TestEmailParamValidation(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/email/push-article",
		map[string]any{"url": ""})
	if status != 400 || body["success"] != false {
		t.Fatalf("push-article 缺 url: %d %v", status, body)
	}

	resp, err := client.Get(ts.URL + "/api/email/preview-article")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("preview-article 缺 url: %d", resp.StatusCode)
	}
}

func TestPluginMarketAndImageUploadValidation(t *testing.T) {
	ts := newTestServerOpts(t, "", Options{Plugins: plugin.NewManager(t.TempDir())})
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "GET", ts.URL+"/api/plugins/market", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("market: %d %v", status, body)
	}

	// 未提供文件字段 → 400
	status, body = doJSON(t, client, "POST", ts.URL+"/api/plugins/ghost/image/upload",
		map[string]any{})
	if status != 400 {
		t.Fatalf("upload 无文件应 400: %d %v", status, body)
	}

	// 提供文件但插件未运行 → 404
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "cover.png")
	fw.Write([]byte("x"))
	mw.Close()
	req, _ := http.NewRequest("POST", ts.URL+"/api/plugins/ghost/image/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("upload 未运行插件应 404: %d", resp.StatusCode)
	}
}

func overrideCoverEndpoint(t *testing.T, url string) {
	t.Helper()
	old := aigen.CoverEndpointOverride
	aigen.CoverEndpointOverride = url
	t.Cleanup(func() { aigen.CoverEndpointOverride = old })
}

func TestImageGenCoverHandler(t *testing.T) {
	const dataURL = "data:image/png;base64," + "ZmFrZS1wbmctYnl0ZXM="
	overrideCoverEndpoint(t, coverEndpointSrv(t, fmt.Sprintf(
		`{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":%q}}]}}]}`, dataURL), 0))
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("IMAGE_GEN_MODEL", "")

	contentDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contentDir, "post", "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerWithContent(t, contentDir)
	client := ts.Client()
	login(t, client, ts.URL)

	post := func(payload map[string]any) (int, map[string]any) {
		return doJSON(t, client, "POST", ts.URL+"/api/image/generate-cover", payload)
	}

	// 缺 article_path
	status, body := post(map[string]any{})
	if status != 400 || body["success"] != false {
		t.Fatalf("缺 article_path: %d %v", status, body)
	}

	// 正常生成
	status, body = post(map[string]any{
		"article_path": "post/a/index.md", "title": "T", "description": "D", "content": "C",
	})
	if status != 200 || body["success"] != true {
		t.Fatalf("生成: %d %v", status, body)
	}
	if url, _ := body["url"].(string); url == "" {
		t.Fatal("应返回封面 url")
	}
	matches, _ := filepath.Glob(filepath.Join(contentDir, "post", "a", "pics", "cover_*.png"))
	if len(matches) == 0 {
		t.Fatal("封面文件未落盘")
	}

	// 上游生成失败 → 500
	overrideCoverEndpoint(t, coverEndpointSrv(t, "boom", http.StatusInternalServerError))
	status, body = post(map[string]any{"article_path": "post/a/index.md"})
	if status != 500 || body["success"] != false {
		t.Fatalf("上游失败: %d %v", status, body)
	}
}

// coverEndpointSrv 返回固定响应体的假封面服务 URL。
func coverEndpointSrv(t *testing.T, body string, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestPasswordChangeFlow(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	change := func(payload map[string]any) (int, map[string]any) {
		return doJSON(t, client, "POST", ts.URL+"/api/auth/password", payload)
	}

	// 缺参数
	if status, _ := change(map[string]any{"current_password": "admin"}); status != 400 {
		t.Fatal("缺新密码应 400")
	}
	// 当前密码错误
	if status, _ := change(map[string]any{"current_password": "wrong", "new_password": "n"}); status != 400 {
		t.Fatal("当前密码错误应 400")
	}
	// 成功修改 → 旧密码失效，新密码可登录
	if status, body := change(map[string]any{"current_password": "admin", "new_password": "new-pass-1"}); status != 200 || body["success"] != true {
		t.Fatalf("修改: %d %v", status, body)
	}
	status, _ := doJSON(t, client, "POST", ts.URL+"/api/auth/login",
		map[string]string{"username": "admin", "password": "admin"})
	if status != 401 {
		t.Fatalf("旧密码应失效: %d", status)
	}
}

func TestArticleStatusValidation(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	resp, err := client.Get(ts.URL + "/api/article/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("缺 file_path: %d", resp.StatusCode)
	}

	// 不存在的文件 → 404
	resp, err = client.Get(ts.URL + "/api/article/status?file_path=post/none/index.md")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("不存在文件应 404: %d", resp.StatusCode)
	}
}

func TestFileReadWithFMValidation(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "POST", ts.URL+"/api/file/read-with-frontmatter",
		map[string]any{"path": ""})
	if status != 400 {
		t.Fatalf("缺 path: %d %v", status, body)
	}
	status, body = doJSON(t, client, "POST", ts.URL+"/api/file/read-with-frontmatter",
		map[string]any{"path": "post/missing.md"})
	if status != 404 {
		t.Fatalf("文件不存在应 404: %d %v", status, body)
	}
}

func TestGitHandlersWithoutServices(t *testing.T) {
	// 默认 Options 不注入 Git 与 Database → 各 git 端点报服务不可用
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/git/status"},
		{"GET", "/api/git/commits"},
		{"GET", "/api/git/pushes"},
	} {
		req, _ := http.NewRequest(tc.method, ts.URL+tc.path, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("%s %s = %d, want 500", tc.method, tc.path, resp.StatusCode)
		}
	}
}

func TestGitPushesFromDatabase(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	database.RecordPush("origin", "main", "a", "b", 2, "msg", "ok", true)
	database.RecordPush("origin", "main", "b", "c", 1, "msg2", "fail", false)

	ts := newTestServerFull(t, "", nil, database)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "GET", ts.URL+"/api/git/pushes?page=1&per_page=10", nil)
	if status != 200 {
		t.Fatalf("pushes: %d %v", status, body)
	}
	if body["total"] != float64(2) {
		t.Fatalf("total = %v", body["total"])
	}
}

func TestDetectConfigMode(t *testing.T) {
	// 无任何配置
	if m := detectConfigMode(t.TempDir()); m != "none" {
		t.Fatalf("空目录 mode = %q", m)
	}
	// 根配置文件
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hugo.toml"), []byte("a = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := detectConfigMode(root); m != "root" {
		t.Fatalf("根配置 mode = %q", m)
	}
	// config/_default 目录
	dirMode := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dirMode, "config", "_default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirMode, "config", "_default", "hugo.toml"), []byte("a = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := detectConfigMode(dirMode); m != "dir" {
		t.Fatalf("目录配置 mode = %q", m)
	}
}

func TestSPAFallbackAndAPI404(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	// 非 API 路径回退到 SPA index
	resp, err := client.Get(ts.URL + "/some/spa/route")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("SPA 回退 = %d", resp.StatusCode)
	}

	// API 未知路径 → 404 JSON
	resp2, err := client.Get(ts.URL + "/api/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("API 404 = %d", resp2.StatusCode)
	}
}
