package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/svtter/hugo-admin/internal/plugin"
)

// 文章级 TTS 集成测试：testplugin（固定音频 URL/时长）+
// SSE 事件断言 + frontmatter 写回校验。

func newTTSTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	base := t.TempDir()
	pluginDir := filepath.Join(base, "plugins", "test-plugin")
	os.MkdirAll(pluginDir, 0o755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.toml"), []byte(`
[plugin]
name = "test-plugin"
version = "1.0.0"
entry = "testplugin"
[capabilities]
tts_generation = true
`), 0o644)
	bin := filepath.Join(pluginDir, "testplugin")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/testplugin").CombinedOutput(); err != nil {
		t.Fatalf("构建测试插件失败: %v\n%s", err, out)
	}
	os.Chmod(bin, 0o755)
	mgr := plugin.NewManager(base)
	mgr.StartAll()
	t.Cleanup(mgr.StopAll)

	contentDir := t.TempDir()
	os.MkdirAll(filepath.Join(contentDir, "post"), 0o755)
	os.WriteFile(filepath.Join(contentDir, "post", "voice.md"),
		[]byte("---\ntitle: 语音文章\ndraft: false\n---\n\n这是要合成语音的正文。\n"), 0o644)

	ts := newTestServerOpts(t, contentDir, Options{Plugins: mgr})
	return ts, contentDir
}

// waitForSSEFrame 轮询 SSE 响应直到出现目标帧文本或超时。
func waitForSSEFrame(t *testing.T, body *strings.Builder, resp *http.Response, target string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	buf := make([]byte, 4096)
	for time.Now().Before(deadline) {
		if strings.Contains(body.String(), target) {
			return true
		}
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body.Write(buf[:n])
		}
		if err != nil {
			return strings.Contains(body.String(), target)
		}
	}
	return strings.Contains(body.String(), target)
}

func TestArticleTTSFullFlow(t *testing.T) {
	srv, contentDir := newTTSTestServer(t)
	base := srv.URL
	client := newLoggedClient(t, base)

	// status：可用
	status, body := doJSON(t, client, "GET", base+"/api/article/tts/status", nil)
	if status != 200 || body["available"] != true || body["plugin"] != "test-plugin" {
		t.Fatalf("status = %d %#v", status, body)
	}

	// 先订阅 SSE
	sseResp, err := client.Get(base + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	// 发起生成
	status, body = doJSON(t, client, "POST", base+"/api/article/tts", map[string]any{
		"article_path": "post/voice.md",
	})
	if status != 200 || body["pending"] != true {
		t.Fatalf("generate = %d %#v", status, body)
	}
	scope := body["event_scope"].(string)

	// 等 tts.done（后台任务）
	var sseBuf strings.Builder
	if !waitForSSEFrame(t, &sseBuf, sseResp, "event: tts.done") {
		t.Fatalf("未收到 tts.done: %q", sseBuf.String())
	}
	if !strings.Contains(sseBuf.String(), `"scope":"`+scope) {
		t.Fatalf("事件应带 scope: %q", sseBuf.String())
	}

	// frontmatter 写回校验
	raw, _ := os.ReadFile(filepath.Join(contentDir, "post", "voice.md"))
	text := string(raw)
	for _, want := range []string{
		"audio: https://cdn.test-plugin.example.com/audio.mp3",
		"audio_duration_seconds: 12.5",
		"audio_format: mp3",
		"_tts_audio_id: audio-1",
		"这是要合成语音的正文。",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("写回缺少 %q:\n%s", want, text)
		}
	}

	// 删除语音：字段清除
	status, body = doJSON(t, client, "DELETE", base+"/api/article/tts", map[string]any{
		"article_path": "post/voice.md",
	})
	if status != 200 || body["message"] != "已删除语音" {
		t.Fatalf("delete = %d %#v", status, body)
	}
	raw, _ = os.ReadFile(filepath.Join(contentDir, "post", "voice.md"))
	if strings.Contains(string(raw), "audio:") || strings.Contains(string(raw), "_tts_audio_id") {
		t.Fatalf("audio 字段未清除:\n%s", raw)
	}

	// 参数校验
	status, _ = doJSON(t, client, "POST", base+"/api/article/tts", map[string]any{})
	if status != 400 {
		t.Fatalf("缺路径 = %d", status)
	}
	status, _ = doJSON(t, client, "POST", base+"/api/article/tts", map[string]any{
		"article_path": "post/voice.md", "text": "   "})
	if status != 400 {
		t.Fatalf("空正文 = %d", status)
	}
}

func TestArticleTTSTextBypassesLock(t *testing.T) {
	srv, contentDir := newTTSTestServer(t)
	base := srv.URL
	client := newLoggedClient(t, base)

	// 订阅 SSE
	sseResp, err := client.Get(base + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer sseResp.Body.Close()

	// 直传 text（对齐 Python：expected_mtime 保持 None，跳过乐观锁）——
	// 即使文件刚被改过，写回也成功（tts.done 而非 tts.conflict）
	doJSON(t, client, "POST", base+"/api/file/save", map[string]any{
		"path":    "post/voice.md",
		"content": "---\ntitle: 语音文章\ndraft: false\n---\n\n被改动过的正文。\n",
	})

	status, body := doJSON(t, client, "POST", base+"/api/article/tts", map[string]any{
		"article_path": "post/voice.md",
		"text":         "直传的合成文本",
	})
	if status != 200 {
		t.Fatalf("generate = %d %#v", status, body)
	}
	var sseBuf strings.Builder
	if !waitForSSEFrame(t, &sseBuf, sseResp, "event: tts.done") {
		t.Fatalf("直传 text 应跳过乐观锁并成功: %q", sseBuf.String())
	}
	if strings.Contains(sseBuf.String(), "tts.conflict") {
		t.Fatal("直传 text 不应触发 conflict")
	}

	// 写回成功且保留被改过的正文
	raw, _ := os.ReadFile(filepath.Join(contentDir, "post", "voice.md"))
	if !strings.Contains(string(raw), "被改动过的正文。") {
		t.Fatalf("正文应保留:\n%s", raw)
	}
	if !strings.Contains(string(raw), "audio: https://cdn") {
		t.Fatalf("audio 应写回:\n%s", raw)
	}
}
