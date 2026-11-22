package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sun-praise/hugo-admin/v3/internal/auth"
	"github.com/sun-praise/hugo-admin/v3/internal/config"
	"github.com/sun-praise/hugo-admin/v3/internal/realtime"
)

// newStaticTestServer 构造带 HugoRoot 的最小服务端，返回 (server, contentDir, hugoRoot)。
// 与 newTestServerOpts 同构但额外暴露目录路径；静态路由均公开，无需登录态。
func newStaticTestServer(t *testing.T) (*httptest.Server, string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	contentDir := t.TempDir()
	cfg := &config.Config{
		Port:          "0",
		SecretKey:     devSecret,
		AuthStorePath: filepath.Join(t.TempDir(), "auth.json"),
		AdminUIDir:    dir,
		ContentDir:    contentDir,
		HugoRoot:      dir,
		Version:       "2.6.0",
	}
	store, err := auth.OpenStore(cfg.AuthStorePath)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	ts := httptest.NewServer(New(cfg, store, realtime.NewBroker(), Options{}))
	t.Cleanup(ts.Close)
	return ts, contentDir, dir
}

// TestContentRouteServesCoverImage 验证 /content/ 静态路由（Python 版
// page_routes.serve_content_files 的对齐项）：cover_url 的 /content/...
// 前缀必须能取到 content 目录内的图片，而不是 SPA 的 index.html。
func TestContentRouteServesCoverImage(t *testing.T) {
	srv, contentDir, _ := newStaticTestServer(t)
	pics := filepath.Join(contentDir, "post", "demo", "pics")
	if err := os.MkdirAll(pics, 0o755); err != nil {
		t.Fatal(err)
	}
	png := []byte("fake-png-bytes")
	if err := os.WriteFile(filepath.Join(pics, "cover.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/content/post/demo/pics/cover.png")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("/content cover = %d", resp.StatusCode)
	}
	if string(body) != string(png) {
		t.Fatalf("/content cover body = %q, want image bytes", body)
	}

	// 隐藏文件仍 403（与 /api/content 同防护）
	resp2, err := http.Get(srv.URL + "/content/post/demo/.hidden")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("/content hidden = %d, want 403", resp2.StatusCode)
	}
}

// TestHugoStaticServesCover 验证 SPA 兜底前先服务 <HugoRoot>/static/<path>：
// cover 指向 static/ 的绝对路径（如 /images/...）应返回真实文件。
func TestHugoStaticServesCover(t *testing.T) {
	srv, _, hugoRoot := newStaticTestServer(t)
	staticDir := filepath.Join(hugoRoot, "static", "images", "2026", "11", "16")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	svg := []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>")
	if err := os.WriteFile(filepath.Join(staticDir, "cover.svg"), svg, 0o644); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/images/2026/11/16/cover.svg")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("static cover = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "<svg") {
		t.Fatalf("static cover body = %q, want svg bytes", body)
	}
}

// TestHugoStaticFallbacksToSPA：static 未命中的路径、点路径与越界路径都
// 回落 SPA index.html，不泄露文件存在性。
func TestHugoStaticFallbacksToSPA(t *testing.T) {
	srv, _, hugoRoot := newStaticTestServer(t)
	staticDir := filepath.Join(hugoRoot, "static", "images")
	if err := os.MkdirAll(staticDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staticDir, ".hidden"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/images/2026/missing.svg",  // 文件不存在
		"/images/.hidden",           // 点路径拒绝
		"/images/%2e%2e/index.html", // 越界拒绝（mux 400 或 SPA 兜底，均不读 static 外文件）
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		spaFallback := resp.StatusCode == 200 && strings.Contains(string(body), "<html>spa</html>")
		rejected := resp.StatusCode == http.StatusBadRequest
		if !spaFallback && !rejected {
			t.Fatalf("GET %s = %d %q, want SPA index fallback or 400", path, resp.StatusCode, body)
		}
	}
}
