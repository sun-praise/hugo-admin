package httpapi

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/hugo"
	"github.com/svtter/hugo-admin/internal/realtime"
	"github.com/svtter/hugo-admin/internal/settings"
)

func newSettingsTestServer(t *testing.T) *wrappedServer {
	t.Helper()
	// 造一个合法 Hugo 根（config 文件 + content + themes）
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "hugo.toml"), []byte(`theme = "demo-theme"
baseURL = "https://example.com"
`), 0o644)
	os.MkdirAll(filepath.Join(root, "themes", "demo-theme"), 0o755)
	os.MkdirAll(filepath.Join(root, "content", "post"), 0o755)
	os.WriteFile(filepath.Join(root, "content", "post", "img.md"), []byte("# 有图\n\n![x](pic.png)\n"), 0o644)
	os.WriteFile(filepath.Join(root, "content", "post", "pic.png"), []byte("png-bytes"), 0o644)

	cfg := &config.Config{
		Port: "0", SecretKey: devSecret,
		AuthStorePath: filepath.Join(t.TempDir(), "auth.json"),
		AdminUIDir:    t.TempDir(),
		ContentDir:    filepath.Join(root, "content"),
		HugoRoot:      root,
		Version:       "3.0.0",
	}
	store, err := auth.OpenStore(cfg.AuthStorePath)
	if err != nil {
		t.Fatal(err)
	}
	settingsSvc := settings.NewService(filepath.Join(root, ".admin", "settings.json"), nil, "")
	srv := New(cfg, store, realtime.NewBroker(), Options{
		Hugo:     hugo.NewManager(root, "", nil),
		Settings: settingsSvc, EnvAPIKey: "env-key-123456",
	})
	ts := httptest.NewServer(srv)
	jar, _ := cookiejar.New(nil)
	ts.Client().Jar = jar
	t.Cleanup(ts.Close)
	return &wrappedServer{ts, root}
}

type wrappedServer struct {
	*httptest.Server
	root string
}

func TestSettingsGetPut(t *testing.T) {
	srv := newSettingsTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	// 默认设置 + env key 来源
	status, body := doJSON(t, client, "GET", srv.URL+"/api/settings", nil)
	if status != 200 {
		t.Fatalf("get = %d", status)
	}
	pub := body["settings"].(map[string]any)
	ai := pub["ai"].(map[string]any)
	if ai["api_key_source"] != "env" || ai["api_key_configured"] != true {
		t.Fatalf("ai = %#v", ai)
	}
	hugoPub := pub["hugo"].(map[string]any)
	if hugoPub["base_dir"] != srv.root {
		t.Fatalf("base_dir = %#v", hugoPub["base_dir"])
	}
	if !strings.HasSuffix(hugoPub["server_url"].(string), ":1313") {
		t.Fatalf("server_url 应补端口: %#v", hugoPub["server_url"])
	}

	// PUT：带 api_key（session 化）+ model 更新
	status, body = doJSON(t, client, "PUT", srv.URL+"/api/settings", map[string]any{
		"settings": map[string]any{
			"ai": map[string]any{"model": "glm-4.6", "api_key": "sess-key-abcd1234"},
		},
	})
	if status != 200 || body["message"] != "设置已保存" {
		t.Fatalf("put = %d %#v", status, body)
	}
	ai2 := body["settings"].(map[string]any)["ai"].(map[string]any)
	if ai2["model"] != "glm-4.6" || ai2["api_key_source"] != "session" || ai2["api_key_hint"] != "sess...1234" {
		t.Fatalf("ai after put = %#v", ai2)
	}
	// api_key 不落盘
	raw, _ := os.ReadFile(filepath.Join(srv.root, ".admin", "settings.json"))
	if strings.Contains(string(raw), "sess-key") {
		t.Fatal("session api_key 不应落盘")
	}

	// 校验失败 400
	status, _ = doJSON(t, client, "PUT", srv.URL+"/api/settings", map[string]any{
		"settings": map[string]any{"ai": map[string]any{"base_url": "ftp://x"}},
	})
	if status != 400 {
		t.Fatalf("bad url = %d", status)
	}
}

func TestConfigEndpoints(t *testing.T) {
	srv := newSettingsTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	// 列表（root 模式）
	status, body := doJSON(t, client, "GET", srv.URL+"/api/config", nil)
	if status != 200 || body["mode"] != "root" {
		t.Fatalf("config list = %d %#v", status, body)
	}
	files := body["files"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["name"] != "hugo.toml" {
		t.Fatalf("files = %#v", files)
	}

	// 读取
	status, body = doJSON(t, client, "GET", srv.URL+"/api/config/hugo.toml", nil)
	if status != 200 || !strings.Contains(body["content"].(string), "demo-theme") {
		t.Fatalf("config get = %d %#v", status, body)
	}

	// 写入（TOML 语法校验）
	status, body = doJSON(t, client, "PUT", srv.URL+"/api/config/hugo.toml", map[string]any{
		"content": "theme = \"new-theme\"\nbaseURL = \"https://example.com\"\n",
	})
	if status != 200 || body["message"] != "配置已保存: hugo.toml" {
		t.Fatalf("config put = %d %#v", status, body)
	}
	// 语法错误 400
	status, body = doJSON(t, client, "PUT", srv.URL+"/api/config/hugo.toml", map[string]any{
		"content": "theme = [broken",
	})
	if status != 400 || !strings.Contains(body["message"].(string), "TOML 语法错误") {
		t.Fatalf("bad toml = %d %#v", status, body)
	}
	// 不存在 404
	status, _ = doJSON(t, client, "GET", srv.URL+"/api/config/nope.toml", nil)
	if status != 404 {
		t.Fatalf("404 = %d", status)
	}
}

func TestThemeEndpoints(t *testing.T) {
	srv := newSettingsTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	status, body := doJSON(t, client, "GET", srv.URL+"/api/themes", nil)
	if status != 200 {
		t.Fatalf("themes = %d %#v", status, body)
	}
	themes := body["themes"].([]any)
	if len(themes) != 1 || themes[0].(map[string]any)["name"] != "demo-theme" {
		t.Fatalf("themes = %#v", themes)
	}
	if body["active_theme"] != "demo-theme" {
		t.Fatalf("active = %#v", body["active_theme"])
	}

	// available
	_, body = doJSON(t, client, "GET", srv.URL+"/api/themes/available", nil)
	if len(body["available_themes"].([]any)) == 0 {
		t.Fatal("available_themes 空")
	}

	// activate 已存在
	status, _ = doJSON(t, client, "POST", srv.URL+"/api/themes/activate",
		map[string]any{"name": "demo-theme"})
	if status != 200 {
		t.Fatalf("activate = %d", status)
	}
	// 不存在 400
	status, _ = doJSON(t, client, "POST", srv.URL+"/api/themes/activate",
		map[string]any{"name": "nope"})
	if status != 400 {
		t.Fatalf("activate nope = %d", status)
	}
	// 空名预览 400
	status, _ = doJSON(t, client, "POST", srv.URL+"/api/themes/preview", map[string]any{})
	if status != 400 {
		t.Fatalf("preview empty = %d", status)
	}
}

func TestContentFile(t *testing.T) {
	srv := newSettingsTestServer(t)
	client := srv.Client()
	login(t, client, srv.URL)

	resp, err := client.Get(srv.URL + "/api/content/post/pic.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("content = %d", resp.StatusCode)
	}

	// 隐藏文件 403
	resp2, err := client.Get(srv.URL + "/api/content/post/.hidden")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("hidden = %d", resp2.StatusCode)
	}
}
