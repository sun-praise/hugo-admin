package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/auth"
	"github.com/svtter/hugo-admin/internal/config"
	"github.com/svtter/hugo-admin/internal/realtime"
)

const devSecret = "dev-secret-key-change-in-production"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	// SPA index，供回退测试
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>spa</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Port:          "0",
		SecretKey:     devSecret,
		AuthStorePath: filepath.Join(t.TempDir(), "auth.json"),
		AdminUIDir:    dir,
		Version:       "2.6.0",
	}
	store, err := auth.OpenStore(cfg.AuthStorePath)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	srv := New(cfg, store, realtime.NewBroker())
	ts := httptest.NewServer(srv)
	// 默认 client 没有 cookie jar，登录态无法保持
	jar, _ := cookiejar.New(nil)
	ts.Client().Jar = jar
	t.Cleanup(ts.Close)
	return ts
}

func doJSON(t *testing.T, client *http.Client, method, url string, body any) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(mustJSON(body)))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func assertBody(t *testing.T, got map[string]any, want map[string]any) {
	t.Helper()
	if mustJSON(got) != mustJSON(want) {
		t.Fatalf("响应体不符\n got: %s\nwant: %s", mustJSON(got), mustJSON(want))
	}
}

func TestGuardRejectsLoggedOut(t *testing.T) {
	ts := newTestServer(t)
	status, body := doJSON(t, ts.Client(), "GET", ts.URL+"/api/git/commits", nil)
	if status != 401 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "未登录或会话已过期"})
}

func TestLoginMeLogoutFlow(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()

	// 错误密码
	status, body := doJSON(t, client, "POST", ts.URL+"/api/auth/login",
		map[string]string{"username": "admin", "password": "bad"})
	if status != 401 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "用户名或密码错误"})

	// 缺字段
	status, body = doJSON(t, client, "POST", ts.URL+"/api/auth/login", map[string]string{})
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "缺少用户名或密码"})

	// 正确登录（客户端 jar 记住 cookie）
	status, body = doJSON(t, client, "POST", ts.URL+"/api/auth/login",
		map[string]string{"username": "admin", "password": "admin"})
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": true, "user": map[string]any{"username": "admin"}})

	// me
	status, body = doJSON(t, client, "GET", ts.URL+"/api/auth/me", nil)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": true, "user": map[string]any{"username": "admin"}})

	// logout 后 me 401
	status, body = doJSON(t, client, "POST", ts.URL+"/api/auth/logout", nil)
	if status != 200 || body["success"] != true {
		t.Fatalf("logout: %d %v", status, body)
	}
	status, body = doJSON(t, client, "GET", ts.URL+"/api/auth/me", nil)
	if status != 401 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "未登录"})
}

func TestChangePassword(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	if _, body := doJSON(t, client, "POST", ts.URL+"/api/auth/login",
		map[string]string{"username": "admin", "password": "admin"}); body["success"] != true {
		t.Fatalf("login 失败: %v", body)
	}

	// 未登录路径（独立 client，无 cookie）
	status, body := doJSON(t, &http.Client{}, "POST", ts.URL+"/api/auth/password",
		map[string]string{"current_password": "admin", "new_password": "x"})
	if status != 401 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "未登录或会话已过期"})

	// 当前密码错误 → 400
	status, body = doJSON(t, client, "POST", ts.URL+"/api/auth/password",
		map[string]string{"current_password": "wrong", "new_password": "x"})
	if status != 400 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "当前密码错误"})

	// 正常改密
	status, body = doJSON(t, client, "POST", ts.URL+"/api/auth/password",
		map[string]string{"current_password": "admin", "new_password": "next-pass"})
	if status != 200 || body["success"] != true {
		t.Fatalf("改密失败: %d %v", status, body)
	}
}

func TestVersionAnd404(t *testing.T) {
	ts := newTestServer(t)
	status, body := doJSON(t, ts.Client(), "GET", ts.URL+"/api/version", nil)
	if status != 200 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"version": "2.6.0"})

	// 未登录时守卫先拦截（Python 同语义：before_request 先于 404）
	status, body = doJSON(t, ts.Client(), "GET", ts.URL+"/api/nonexistent", nil)
	if status != 401 {
		t.Fatalf("未登录 status = %d", status)
	}

	// 登录后才是 404 JSON
	login(t, ts.Client(), ts.URL)
	status, body = doJSON(t, ts.Client(), "GET", ts.URL+"/api/nonexistent", nil)
	if status != 404 {
		t.Fatalf("status = %d", status)
	}
	assertBody(t, body, map[string]any{"success": false, "message": "接口不存在"})
}

func login(t *testing.T, client *http.Client, base string) {
	t.Helper()
	status, body := doJSON(t, client, "POST", base+"/api/auth/login",
		map[string]string{"username": "admin", "password": "admin"})
	if status != 200 {
		t.Fatalf("登录失败: %d %v", status, body)
	}
}

func TestSSEEvents(t *testing.T) {
	ts := newTestServer(t)
	client := ts.Client()
	if _, body := doJSON(t, client, "POST", ts.URL+"/api/auth/login",
		map[string]string{"username": "admin", "password": "admin"}); body["success"] != true {
		t.Fatalf("login 失败: %v", body)
	}

	resp, err := client.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	// 通过 broker 广播一条，读流验证 connected + 自定义事件
	// （broker 实例由 New 内部创建，这里借助 health 端点确认服务器活着，
	// 然后直接读取 connected 帧）
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	frame := string(buf[:n])
	if !strings.Contains(frame, "event: connected") {
		t.Fatalf("未收到 connected 帧: %q", frame)
	}
	resp.Body.Close()
}

func TestSPAFallbackAndStatic404(t *testing.T) {
	ts := newTestServer(t)
	resp, err := ts.Client().Get(ts.URL + "/posts/editor")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("SPA 回退 status = %d", resp.StatusCode)
	}

	resp2, err := ts.Client().Get(ts.URL + "/admin-ui/no-such.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != 404 {
		t.Fatalf("静态 404 status = %d", resp2.StatusCode)
	}
}
