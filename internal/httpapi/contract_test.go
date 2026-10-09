package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/chathistory"
	"github.com/svtter/hugo-admin/internal/db"
	"github.com/svtter/hugo-admin/internal/git"
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
	"GET /api/git/status":                  true,
	"GET /api/git/commits":                 true,
	"POST /api/git/push":                   true,
	"GET /api/git/pushes":                  true,
	"POST /api/publish/system":             true,
	"GET /api/article/status":              true,
	"POST /api/article/status/bulk":        true,
	"POST /api/article/publish":            true,
	"POST /api/article/publish/bulk":       true,
	"GET /api/ai/sessions":                 true,
	"POST /api/ai/sessions":                true,
}

// implementedPrefixes 覆盖带路径参数的端点（id 段为随机值）。
var implementedPrefixes = []string{
	"GET /api/ai/sessions/",
	"DELETE /api/ai/sessions/",
}

// envFieldIgnores 列出随机器/时间变化的字段（支持嵌套路径：
// "a.b" 与数组通配 "a[].b"），回放比对时剔除。
var envFieldIgnores = map[string][]string{
	"POST /api/file/read":                  {"mtime"},
	"POST /api/file/read-with-frontmatter": {"mtime"},
	"POST /api/file/save":                  {"mtime", "current_mtime"},
	"POST /api/post/create":                {"path"}, // path 含当日日期
	"GET /api/article/status": {
		"status.file_path", "status.frontmatter", "status.last_published", // frontmatter 日期序列化两边不同；last_published 为发布时刻
	},
	"POST /api/article/status/bulk": {"results[].status.file_path", "results[].status.frontmatter"},
	"POST /api/article/publish":     {"published_at", "operation_id", "error"},
	"POST /api/article/publish/bulk": {
		"operation_id", "results[].message", "results[].published_at",
	},
	"GET /api/git/pushes": {"pushes[].pushed_at", "pushes[].pushed_at_iso"},
	"GET /api/ai/sessions": {
		"sessions[].session_id", "sessions[].created_at", "sessions[].updated_at",
	},
	"POST /api/ai/sessions": {"session_id", "created_at", "updated_at"},
}

// deletePath 按点分路径删除字段；段名带 "[]" 表示遍历数组元素。
func deletePath(v any, segs []string) {
	if len(segs) == 0 {
		return
	}
	cur, ok := v.(map[string]any)
	if !ok {
		return
	}
	name := strings.TrimSuffix(segs[0], "[]")
	child, exists := cur[name]
	if !exists {
		return
	}
	if strings.HasSuffix(segs[0], "[]") {
		if arr, ok := child.([]any); ok {
			for _, item := range arr {
				deletePath(item, segs[1:])
			}
		}
		return
	}
	if len(segs) == 1 {
		delete(cur, name)
		return
	}
	deletePath(child, segs[1:])
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

// seedArticleFixture 复刻 tests/test_article_http_api.py 的 seed。
func seedArticleFixture(t *testing.T, contentDir string) {
	t.Helper()
	seedWriteFixture(t, contentDir)
	write := func(rel, content string) {
		path := filepath.Join(contentDir, rel)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("post/draft.md", "---\ntitle: 草稿文章\ndraft: true\ntags:\n  - go\n---\n\n草稿正文\n")
	write("post/draft2.md", "---\ntitle: 第二草稿\ndraft: true\n---\n\n第二草稿\n")
}

// seedGitRepo 复刻 tests/test_git_http_api.py 的仓库序列（固定
// author/committer 日期 → commit hash 确定）。三处保持同步：
// internal/git/git_test.go 的 setupRepo、tests/test_git_http_api.py、此处。
func seedGitRepo(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	bare := filepath.Join(base, "remote.git")
	if err := os.MkdirAll(filepath.Join(repo, "content", "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(dir string, env map[string]string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(repo, "content", "post", rel)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run(repo, nil, "init", "-b", "main")
	run(repo, nil, "config", "user.name", "契约测试")
	run(repo, nil, "config", "user.email", "contract@test.local")

	d1 := map[string]string{
		"GIT_AUTHOR_DATE": "2026-01-01T12:00:00+08:00", "GIT_COMMITTER_DATE": "2026-01-01T12:00:00+08:00"}
	write("a.md", "a\n")
	run(repo, d1, "add", "-A")
	run(repo, d1, "commit", "-m", "init: 第一提交")

	d2 := map[string]string{
		"GIT_AUTHOR_DATE": "2026-01-02T08:30:00+08:00", "GIT_COMMITTER_DATE": "2026-01-02T08:30:00+08:00"}
	write("b.md", "b\n")
	run(repo, d2, "add", "-A")
	run(repo, d2, "commit", "-m", "feat: 第二提交（含|竖线）")

	write("d.md", "d\n")
	run(repo, nil, "add", "content/post/d.md")
	write("b.md", "b changed\n")
	write("c.md", "c\n")

	run(base, nil, "init", "-b", "main", "--bare", bare)
	run(repo, nil, "remote", "add", "origin", bare)
	return repo
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
		deletePath(body, strings.Split(field, "."))
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

	// git 契约样本：contentDir 即 repo/content（publish 流经 /api/file/save
	// 制造改动，再由 git 端点提交推送），按样本顺序回放
	gitRepo := seedGitRepo(t)
	gitSvc, err := git.New(gitRepo, nil)
	if err != nil {
		t.Fatalf("git service: %v", err)
	}
	gitTS := newTestServerFull(t, filepath.Join(gitRepo, "content"), gitSvc, nil)

	// pushes 样本：git 服务注入真实 db（push 落库后经 /api/git/pushes 查询）
	pushesRepo := seedGitRepo(t)
	pushesDB, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("pushes db: %v", err)
	}
	pushesGit, err := git.New(pushesRepo, pushesDB)
	if err != nil {
		t.Fatalf("pushes git: %v", err)
	}
	pushesTS := newTestServerFull(t, filepath.Join(pushesRepo, "content"), pushesGit, pushesDB)

	// article 样本：独立 content 目录（file 批次样本会改写共享文件）
	articleContentDir := t.TempDir()
	seedArticleFixture(t, articleContentDir)
	articleTS := newTestServerFull(t, articleContentDir, nil, nil)

	// AI sessions 样本：真实 sqlite + chat history 服务
	aiDB, err := db.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("ai db: %v", err)
	}
	aiTS := newTestServerOpts(t, "", Options{Database: aiDB, Chat: chathistory.New(aiDB)})

	serverFor := func(path, source string) *httptest.Server {
		// git 测试里的 file/save 样本必须落在 git repo 的 contentDir
		if strings.HasPrefix(source, "tests/test_git_http_api.py") {
			return gitTS
		}
		if strings.HasPrefix(source, "tests/test_pushes_http_api.py") {
			return pushesTS
		}
		if strings.HasPrefix(source, "tests/test_article_http_api.py") {
			return articleTS
		}
		if strings.HasPrefix(source, "tests/test_ai_sessions_http_api.py") {
			return aiTS
		}
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
		implemented := implementedRoutes[key]
		if !implemented {
			for _, prefix := range implementedPrefixes {
				if strings.HasPrefix(key, prefix) {
					implemented = true
					break
				}
			}
		}
		if !implemented && !isAPI404 {
			skipped++
			continue
		}

		target := serverFor(s.Path, s.Source)

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
