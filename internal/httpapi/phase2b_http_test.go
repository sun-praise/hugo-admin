package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/hugo"
	"github.com/svtter/hugo-admin/internal/realtime"
)

// newTestServerHugoRoot 与 newTestServerOpts 相同，但允许指定 cfg.HugoRoot
// （theme/config 处理器依赖它定位站点）。
func newTestServerHugoRoot(t *testing.T, hugoRoot string) *httptest.Server {
	t.Helper()
	ui := t.TempDir()
	if err := os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:          "0",
		SecretKey:     devSecret,
		AuthStorePath: filepath.Join(t.TempDir(), "auth.json"),
		AdminUIDir:    ui,
		ContentDir:    t.TempDir(),
		HugoRoot:      hugoRoot,
		Version:       "2.6.0",
	}
	store, err := auth.OpenStore(cfg.AuthStorePath)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	srv := New(cfg, store, realtime.NewBroker(), Options{Hugo: hugo.NewManager(hugoRoot, "", nil)})
	ts := httptest.NewServer(srv)
	jar, _ := cookiejar.New(nil)
	ts.Client().Jar = jar
	t.Cleanup(ts.Close)
	return ts
}

// localThemeRepo 建一个可克隆的本地主题仓库。
func localThemeRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "theme.toml"), []byte("name = 'demo'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestThemeInstallAndPreview(t *testing.T) {
	fakeHugoBin(t)
	hugoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(hugoRoot, "hugo.toml"), []byte("title = 'x'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerHugoRoot(t, hugoRoot)
	client := ts.Client()
	login(t, client, ts.URL)

	post := func(path string, payload map[string]any) (int, map[string]any) {
		return doJSON(t, client, "POST", ts.URL+path, payload)
	}

	// 参数校验分支
	status, body := post("/api/themes/install", map[string]any{"repo_url": "", "name": "n", "mode": "clone"})
	if status != 400 {
		t.Fatalf("空 repo_url 应 400, got %d %v", status, body)
	}

	// clone 模式安装本地主题
	status, body = post("/api/themes/install", map[string]any{
		"repo_url": localThemeRepo(t), "name": "demo-theme", "mode": "clone"})
	if status != 200 || body["success"] != true {
		t.Fatalf("install: %d %v", status, body)
	}
	if _, err := os.Stat(filepath.Join(hugoRoot, "themes", "demo-theme", "theme.toml")); err != nil {
		t.Fatalf("主题未克隆: %v", err)
	}

	// 预览：空名 / 不存在 / 正常
	if status, _ := post("/api/themes/preview", map[string]any{"name": ""}); status != 400 {
		t.Fatal("空主题名应 400")
	}
	if status, _ := post("/api/themes/preview", map[string]any{"name": "nope"}); status != 400 {
		t.Fatal("不存在主题应 400")
	}
	status, body = post("/api/themes/preview", map[string]any{"name": "demo-theme"})
	if status != 200 || body["success"] != true {
		t.Fatalf("preview: %d %v", status, body)
	}
}

func TestConfigPutFlow(t *testing.T) {
	hugoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(hugoRoot, "hugo.toml"), []byte("title = 'old'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := newTestServerHugoRoot(t, hugoRoot)
	client := ts.Client()
	login(t, client, ts.URL)

	put := func(filename, content string) (int, map[string]any) {
		return doJSON(t, client, "PUT", ts.URL+"/api/config/"+filename,
			map[string]any{"content": content})
	}

	// 非法 TOML → 400
	if status, body := put("hugo.toml", "not [valid toml"); status != 400 || body["success"] != false {
		t.Fatalf("坏 TOML: %d %v", status, body)
	}
	// 空 content → 400
	if status, _ := put("hugo.toml", ""); status != 400 {
		t.Fatal("空内容应 400")
	}
	// 合法保存
	if status, body := put("hugo.toml", "title = 'new'\n"); status != 200 || body["success"] != true {
		t.Fatalf("保存: %d %v", status, body)
	}
	raw, _ := os.ReadFile(filepath.Join(hugoRoot, "hugo.toml"))
	if string(raw) != "title = 'new'\n" {
		t.Fatalf("落盘内容 = %q", raw)
	}
}

func TestPluginLifecycleHTTP(t *testing.T) {
	ts, mgr := newPluginTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)
	_ = mgr

	// disable → enable 覆盖两个分支
	status, body := doJSON(t, client, "POST", ts.URL+"/api/plugins/test-plugin/disable", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("disable: %d %v", status, body)
	}
	status, body = doJSON(t, client, "POST", ts.URL+"/api/plugins/test-plugin/enable", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("enable: %d %v", status, body)
	}
	// 不存在的插件
	if status, _ := doJSON(t, client, "POST", ts.URL+"/api/plugins/ghost/disable", nil); status != 404 {
		t.Fatal("未知插件 disable 应 404")
	}

	// 运行中的插件：TTS 删除（实现返回 success 或 Unimplemented 均覆盖）
	req, _ := http.NewRequest("DELETE", ts.URL+"/api/plugins/test-plugin/tts/audio-1", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 500 {
		t.Fatalf("tts delete = %d", resp.StatusCode)
	}

	// 运行中的插件：multipart 图片上传走 gRPC 流式
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "cover.png")
	fw.Write([]byte("png-bytes"))
	mw.Close()
	upReq, _ := http.NewRequest("POST", ts.URL+"/api/plugins/test-plugin/image/upload", &buf)
	upReq.Header.Set("Content-Type", mw.FormDataContentType())
	upResp, err := client.Do(upReq)
	if err != nil {
		t.Fatal(err)
	}
	upResp.Body.Close()
	if upResp.StatusCode != 200 {
		t.Fatalf("image upload = %d", upResp.StatusCode)
	}
}

func TestFormBool(t *testing.T) {
	if !formBool("true", false) {
		t.Fatal("字符串 true 应为 true")
	}
	if !formBool("yes", false) {
		t.Fatal("字符串 yes 应为 true")
	}
	if formBool("off", true) {
		t.Fatal("字符串 off 应为 false")
	}
	if !formBool("", true) {
		t.Fatal("空串应取默认值")
	}
	if formBool("", false) {
		t.Fatal("空串应取默认值 false")
	}
	if !formBool("junk", true) {
		t.Fatal("无法识别的值应取默认值")
	}
}
