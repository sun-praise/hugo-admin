package httpapi

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheEndpoints(t *testing.T) {
	contentDir := t.TempDir()
	os.MkdirAll(filepath.Join(contentDir, "post"), 0o755)
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(contentDir, "post", string(rune('a'+i))+".md"),
			[]byte("---\ntitle: T\n---\nx\n"), 0o644)
	}
	ts := newTestServerWithContent(t, contentDir)
	client := ts.Client()
	login(t, client, ts.URL)

	status, body := doJSON(t, client, "GET", ts.URL+"/api/cache/stats", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("stats = %d %#v", status, body)
	}
	stats := body["stats"].(map[string]any)
	if stats["total_posts"] != float64(3) {
		t.Fatalf("stats = %#v", stats)
	}

	status, body = doJSON(t, client, "POST", ts.URL+"/api/cache/refresh", nil)
	if status != 200 || body["message"] != "缓存刷新成功" {
		t.Fatalf("refresh = %d %#v", status, body)
	}
}

func TestProjectEndpoints(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	login(t, client, ts.URL)

	// active
	status, body := doJSON(t, client, "GET", ts.URL+"/api/project/active", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("active = %d %#v", status, body)
	}

	// init 校验：空路径
	status, body = doJSON(t, client, "POST", ts.URL+"/api/project/init", map[string]any{})
	if status != 400 || body["message"] != "目标路径不能为空" {
		t.Fatalf("empty path = %d %#v", status, body)
	}

	// init 校验：非法格式
	status, body = doJSON(t, client, "POST", ts.URL+"/api/project/init",
		map[string]any{"path": "/tmp/x", "config_format": "json"})
	if status != 400 || body["message"] != "配置文件格式仅支持 toml 或 yaml" {
		t.Fatalf("bad format = %d %#v", status, body)
	}

	// init 校验：非绝对路径
	status, body = doJSON(t, client, "POST", ts.URL+"/api/project/init",
		map[string]any{"path": "relative/path"})
	if status != 400 {
		t.Fatalf("relative = %d %#v", status, body)
	}

	// reset
	status, body = doJSON(t, client, "POST", ts.URL+"/api/project/active/reset", nil)
	if status != 200 || body["message"] != "已清除持久化记录" {
		t.Fatalf("reset = %d %#v", status, body)
	}

	// clean-layouts：无主题 → 400
	status, body = doJSON(t, client, "POST", ts.URL+"/api/project/clean-layouts", nil)
	if status != 400 {
		t.Fatalf("clean no theme = %d %#v", status, body)
	}
}
