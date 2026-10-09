package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"os"
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
	"GET /api/auth/me":        true,
	"POST /api/auth/login":    true,
	"POST /api/auth/logout":   true,
	"POST /api/auth/password": true,
	"GET /api/version":        true,
}

func TestContractReplay(t *testing.T) {
	data, err := os.ReadFile("../../contracts/api_samples.jsonl")
	if err != nil {
		t.Skipf("契约样本不存在: %v", err)
	}

	ts := newTestServer(t)
	bareClient := &http.Client{}

	// 每条样本用全新登录的客户端：logout 等样本会破坏共享登录态
	freshLoggedClient := func() *http.Client {
		c := &http.Client{}
		jar, _ := cookiejar.New(nil)
		c.Jar = jar
		login(t, c, ts.URL)
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

		// 样本是守卫 401 → 用未登录客户端；否则用全新登录的客户端
		client := bareClient
		if s.Status != 401 {
			client = freshLoggedClient()
		}

		url := ts.URL + s.Path
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
