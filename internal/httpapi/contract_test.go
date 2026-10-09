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
)

// 回放 contracts/api_samples.jsonl（由 RECORD_CONTRACT=1 pytest 生成）中
// 已实现端点的样本，逐条比对状态码与响应体。未实现端点跳过并计数，
// 随迁移推进逐步纳入，保证 Go 侧与 Python 侧行为一致。

type contractSample struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	Query        string `json:"query"`
	RequestJSON  any    `json:"request_json"`
	Status       int    `json:"status"`
	ResponseJSON any    `json:"response_json"`
	Source       string `json:"source"`
}

// implementedRoutes 是 Go 侧已实现并通过契约比对的端点。
var implementedRoutes = map[string]bool{
	"GET /api/auth/me":                     true,
	"POST /api/auth/login":                 true,
	"POST /api/auth/logout":                true,
	"POST /api/auth/password":              true,
	"GET /api/version":                     true,
	"GET /api/posts":                       true,
	"GET /api/posts/tags":                  true,
	"GET /api/posts/categories":            true,
	"POST /api/file/read":                  true,
	"POST /api/file/read-with-frontmatter": true,
	"POST /api/file/save":                  true,
	"POST /api/post/create":                true,
}

// envFieldIgnores 列出随机器/时间变化的顶层字段，回放比对时剔除。
var envFieldIgnores = map[string][]string{
	"POST /api/file/read":                  {"mtime"},
	"POST /api/file/read-with-frontmatter": {"mtime"},
	"POST /api/file/save":                  {"mtime", "current_mtime"},
	"POST /api/post/create":                {"path"}, // path 含当日日期
}

// postsFixtureDir 与 Python 测试 tests/test_posts_http_api.py 共享的
// fixture 内容目录（该测试 setup 时写入）。
const postsFixtureDir = "../../contracts/fixtures/posts"

// seedWriteFixture 复刻 tests/test_file_http_api.py 的 seed 文件，
// 写域样本按录制顺序回放，文件状态随之流转。
func seedWriteFixture(t *testing.T, contentDir string) {
	t.Helper()
	write := func(rel, content string) {
		path := filepath.Join(contentDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("post/hello.md", "---\ntitle: Hello\ndate: 2026-01-01\ndraft: false\n---\n\nHello body。\n")
	write("post/dual.md", "---\ntitle: Dual\n---\n---\ninner\n---\nreal body\n")
	write("post/lock.md", "锁定基准\n")
}

// normalizeEnvFields 剔除环境相关字段，其余严格比对。
func normalizeEnvFields(body map[string]any, key string) {
	if strings.HasPrefix(key, "GET /api/posts") {
		if posts, ok := body["posts"].([]any); ok {
			for _, p := range posts {
				if m, ok := p.(map[string]any); ok {
					delete(m, "full_path")
					delete(m, "mod_time")
				}
			}
		}
		return
	}
	for _, field := range envFieldIgnores[key] {
		delete(body, field)
	}
}

func TestContractReplay(t *testing.T) {
	data, err := os.ReadFile("../../contracts/api_samples.jsonl")
	if err != nil {
		t.Skipf("契约样本不存在: %v", err)
	}

	ts := newTestServer(t)

	// posts 样本复用录制侧的 fixture 目录，保证 path/full_path 一致
	var postsTS *httptest.Server
	// 写域样本：全新 temp content 目录 + 固定 seed，按样本顺序回放
	writeContentDir := t.TempDir()
	seedWriteFixture(t, writeContentDir)
	writeTS := newTestServerWithContent(t, writeContentDir)

	serverFor := func(path string) *httptest.Server {
		if strings.HasPrefix(path, "/api/posts") {
			if postsTS == nil {
				if _, err := os.Stat(postsFixtureDir); err != nil {
					t.Skipf("posts fixture 不存在（先跑 pytest tests/test_posts_http_api.py）: %v", err)
				}
				postsTS = newTestServerWithContent(t, postsFixtureDir)
			}
			return postsTS
		}
		if strings.HasPrefix(path, "/api/file") || strings.HasPrefix(path, "/api/post") {
			return writeTS
		}
		return ts
	}

	bareClient := &http.Client{}

	// 每条样本用全新登录的客户端：logout 等样本会破坏共享登录态
	freshLoggedClient := func(base string) *http.Client {
		c := &http.Client{}
		jar, _ := cookiejar.New(nil)
		c.Jar = jar
		login(t, c, base)
		return c
	}

	replayed, skipped := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var s contractSample
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			t.Fatalf("样本解析失败: %v (%s)", err, line)
		}

		key := s.Method + " " + s.Path
		isAPI404 := s.Status == 404 && strings.HasPrefix(s.Path, "/api/")
		if !implementedRoutes[key] && !isAPI404 {
			skipped++
			continue
		}

		target := serverFor(s.Path)

		// 样本是守卫 401 → 用未登录客户端；否则用全新登录的客户端
		client := bareClient
		if s.Status != 401 {
			client = freshLoggedClient(target.URL)
		}

		url := target.URL + s.Path
		if s.Query != "" {
			url += "?" + s.Query
		}
		var reqBody string
		if m, ok := s.RequestJSON.(map[string]any); ok {
			reqBody = mustJSON(m)
		}
		req, _ := http.NewRequest(s.Method, url, strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("%s %s: %v", s.Method, s.Path, err)
			continue
		}
		var got map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&got)
		resp.Body.Close()

		normalizeEnvFields(got, key)
		if want, ok := s.ResponseJSON.(map[string]any); ok {
			normalizeEnvFields(want, key)
		}

		if resp.StatusCode != s.Status {
			t.Errorf("%s %s（%s）: status got %d want %d", s.Method, s.Path, s.Source, resp.StatusCode, s.Status)
		}
		if mustJSON(got) != mustJSON(s.ResponseJSON) {
			t.Errorf("%s %s（%s）: body\n got: %s\nwant: %s", s.Method, s.Path, s.Source, mustJSON(got), mustJSON(s.ResponseJSON))
		}
		replayed++
	}
	t.Logf("契约回放：%d 条通过比对，%d 条对应端点待迁移", replayed, skipped)
}
