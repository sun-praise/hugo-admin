package httpapi

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/svtter/hugo-admin/internal/plugin"
)

// newPluginTestServer 构建真 testplugin 二进制并注入 manager。
func newPluginTestServer(t *testing.T) (*httptest.Server, *plugin.Manager) {
	t.Helper()
	base := t.TempDir()
	pluginDir := filepath.Join(base, "plugins", "test-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`
[plugin]
name = "test-plugin"
version = "1.0.0"
entry = "testplugin"
description = "测试插件"
[capabilities]
image_upload = true
tts_generation = true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(pluginDir, "testplugin")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/testplugin")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建测试插件失败: %v\n%s", err, out)
	}
	os.Chmod(bin, 0o755)

	mgr := plugin.NewManager(base)
	mgr.StartAll()
	t.Cleanup(mgr.StopAll)

	contentDir := t.TempDir()
	os.MkdirAll(filepath.Join(contentDir, "post"), 0o755)
	os.WriteFile(filepath.Join(contentDir, "post", "a.md"), []byte("# A\n"), 0o644)
	ts := newTestServerOpts(t, contentDir, Options{Plugins: mgr})
	return ts, mgr
}

func TestPluginEndpoints(t *testing.T) {
	srv, _ := newPluginTestServer(t)
	base := srv.URL
	client := newLoggedClient(t, base)

	// 列表
	status, body := doJSON(t, client, "GET", base+"/api/plugins", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("list = %d %#v", status, body)
	}
	plugins := body["plugins"].([]any)
	if len(plugins) != 1 {
		t.Fatalf("plugins = %#v", plugins)
	}
	p := plugins[0].(map[string]any)
	if p["name"] != "test-plugin" || p["status"] != "running" || p["enabled"] != true {
		t.Fatalf("plugin = %#v", p)
	}

	// config-schema（gRPC）
	status, body = doJSON(t, client, "GET", base+"/api/plugins/test-plugin/config-schema", nil)
	if status != 200 {
		t.Fatalf("schema = %d %#v", status, body)
	}
	schema := body["schema"].(map[string]any)
	if schema["type"] != "object" {
		t.Fatalf("schema = %#v", schema)
	}

	// config 写读
	status, body = doJSON(t, client, "PUT", base+"/api/plugins/test-plugin/config",
		map[string]any{"api_key": "k-1"})
	if status != 200 || body["message"] != "Configuration saved" {
		t.Fatalf("set config = %d %#v", status, body)
	}
	_, body = doJSON(t, client, "GET", base+"/api/plugins/test-plugin/config", nil)
	if body["config"].(map[string]any)["api_key"] != "k-1" {
		t.Fatalf("config = %#v", body)
	}

	// 未知插件 404
	status, _ = doJSON(t, client, "GET", base+"/api/plugins/none/config", nil)
	if status != 404 {
		t.Fatalf("404 = %d", status)
	}
}

func TestImageUploadViaPluginCDN(t *testing.T) {
	srv, _ := newPluginTestServer(t)
	base := srv.URL
	client := newLoggedClient(t, base)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "cdn图.png")
	fw.Write(bytes.Repeat([]byte{1}, 100))
	mw.WriteField("article_path", "post/a.md")
	mw.Close()

	resp, err := client.Post(base+"/api/image/upload", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	// 插件上传优先：URL 来自插件（CDN），而非本地 pics/
	if body["url"] != "https://cdn.test-plugin.example.com/cdn图.png" {
		t.Fatalf("url = %#v（应走插件 CDN）", body)
	}

	// 插件直接代理端点也可用
	status, body2 := doJSON(t, client, "DELETE", base+"/api/plugins/test-plugin/image/img-x", nil)
	if status != 200 || body2["message"] != "deleted img-x" {
		t.Fatalf("image delete = %d %#v", status, body2)
	}
}

func TestTTSGenerateViaSSE(t *testing.T) {
	srv, _ := newPluginTestServer(t)
	base := srv.URL
	client := newLoggedClient(t, base)

	// 先订阅 SSE 捕获 tts.progress
	sseResp, err := client.Get(base + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	// 发起 TTS 生成（非流式 JSON 结果）
	status, body := doJSON(t, client, "POST", base+"/api/plugins/test-plugin/tts/generate",
		map[string]any{"text": "你好世界", "event_scope": "scope-1"})
	if status != 200 {
		t.Fatalf("tts = %d %#v", status, body)
	}
	if body["url"] != "https://cdn.test-plugin.example.com/audio.mp3" ||
		body["audio_id"] != "audio-1" || body["format"] != "mp3" {
		t.Fatalf("tts result = %#v", body)
	}

	// SSE 帧里应有 tts.progress（带 scope）
	buf := make([]byte, 8192)
	deadline := time.Now().Add(3 * time.Second)
	got := ""
	for time.Now().Before(deadline) {
		n, _ := sseResp.Body.Read(buf)
		if n > 0 {
			got += string(buf[:n])
			if strings.Contains(got, "tts.progress") {
				break
			}
		}
	}
	if !strings.Contains(got, "event: tts.progress") || !strings.Contains(got, `"scope":"scope-1"`) {
		t.Fatalf("SSE 进度帧 = %q", got)
	}

	// 非法 speed → 400
	status, body = doJSON(t, client, "POST", base+"/api/plugins/test-plugin/tts/generate",
		map[string]any{"text": "x", "speed": "abc"})
	if status != 400 || !strings.Contains(body["message"].(string), "无效的参数") {
		t.Fatalf("bad speed = %d %#v", status, body)
	}
}

func newLoggedClient(t *testing.T, base string) *http.Client {
	t.Helper()
	c := &http.Client{}
	jar, _ := cookiejar.New(nil)
	c.Jar = jar
	login(t, c, base)
	return c
}
