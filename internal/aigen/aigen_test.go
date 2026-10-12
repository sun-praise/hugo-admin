package aigen

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetFMCache 清空 frontmatter 缓存，保证测试间互不影响。
func resetFMCache(t *testing.T) {
	t.Helper()
	fmCacheMu.Lock()
	fmCache = map[string]fmCacheEntry{}
	fmCacheMu.Unlock()
}

func chatSrv(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(401)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestGenerateFrontmatterValidation(t *testing.T) {
	resetFMCache(t)
	if ok, msg := GenerateFrontmatter("内容", "", "http://x", "m", time.Now()); ok {
		t.Fatalf("无 Key 应失败: %v", msg)
	}
	if ok, msg := GenerateFrontmatter("  ", "k", "http://x", "m", time.Now()); ok {
		t.Fatalf("空内容应失败: %v", msg)
	}
}

func TestGenerateFrontmatterFromAI(t *testing.T) {
	resetFMCache(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	srv := chatSrv(t, `{"description":"摘要","tags":["a","b"],"categories":["c"]}`, 0)
	ok, result := GenerateFrontmatter("独特内容一：测试 AI 生成", "test-key", srv.URL, "m", now)
	if !ok {
		t.Fatalf("ok = %v, result = %v", ok, result)
	}
	fm, _ := result.(map[string]any)
	if fm["description"] != "摘要" || fmt.Sprint(fm["tags"]) != "[a b]" || fmt.Sprint(fm["categories"]) != "[c]" {
		t.Fatalf("fm = %#v", fm)
	}

	// markdown 代码块包裹
	srv2 := chatSrv(t, "```json\n{\"description\":\"代码块\"}\n```", 0)
	ok, result = GenerateFrontmatter("独特内容二：代码块包裹", "test-key", srv2.URL, "m", now)
	if !ok {
		t.Fatalf("代码块包裹应可解析: %v", result)
	}
	if fm, _ := result.(map[string]any); fm["description"] != "代码块" {
		t.Fatalf("fm = %#v", fm)
	}

	// 无效 JSON
	srv3 := chatSrv(t, "不是 JSON", 0)
	if ok, msg := GenerateFrontmatter("独特内容三：坏 JSON", "test-key", srv3.URL, "m", now); ok {
		t.Fatalf("坏 JSON 应失败: %v", msg)
	}

	// 服务端错误
	srv4 := chatSrv(t, "", http.StatusInternalServerError)
	if ok, _ := GenerateFrontmatter("独特内容四：服务错误", "test-key", srv4.URL, "m", now); ok {
		t.Fatal("服务端错误应失败")
	}
}

func TestGenerateFrontmatterCache(t *testing.T) {
	resetFMCache(t)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"description\":\"缓存\"}"}}]}`)
	}))
	t.Cleanup(srv.Close)

	content := "独特内容五：缓存测试"
	t0 := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	if ok, _ := GenerateFrontmatter(content, "test-key", srv.URL, "m", t0); !ok {
		t.Fatal("首次生成应成功")
	}
	// TTL 内命中缓存，不发起请求
	if ok, _ := GenerateFrontmatter(content, "test-key", srv.URL, "m", t0.Add(10*time.Minute)); !ok {
		t.Fatal("缓存命中应成功")
	}
	if hits != 1 {
		t.Fatalf("hits = %d, TTL 内应只有 1 次请求", hits)
	}
	// TTL 过期重新请求
	if ok, _ := GenerateFrontmatter(content, "test-key", srv.URL, "m", t0.Add(31*time.Minute)); !ok {
		t.Fatal("过期后重新生成应成功")
	}
	if hits != 2 {
		t.Fatalf("hits = %d, 过期后应有 2 次请求", hits)
	}
}

func TestSanitizeFrontmatter(t *testing.T) {
	// 超限截断 + 数量约束 + 空值过滤
	in := map[string]any{
		"description": "  " + strings.Repeat("字", 600) + "  ",
		"tags":        []any{" a", " ", 1, "b", "c", "d", "e", "f"},
		"categories":  []any{"c1", "c2", "c3"},
	}
	out := SanitizeFrontmatter(in)
	desc, _ := out["description"].(string)
	if len([]rune(desc)) != 500 {
		t.Fatalf("description 长度 = %d", len([]rune(desc)))
	}
	tags, _ := out["tags"].([]string)
	if len(tags) != 5 || tags[0] != "a" { // 最多 5 个，非字符串/空白被过滤
		t.Fatalf("tags = %#v", tags)
	}
	cats, _ := out["categories"].([]string)
	if len(cats) != 2 { // 最多 2 个
		t.Fatalf("categories = %#v", cats)
	}

	// 全空输入 → 空结果；类型不匹配 → 忽略
	if out := SanitizeFrontmatter(map[string]any{"tags": "not-array", "description": " "}); len(out) != 0 {
		t.Fatalf("out = %#v", out)
	}
}

func TestChatCompletion(t *testing.T) {
	// 非 2xx
	srv := chatSrv(t, "", http.StatusInternalServerError)
	if _, ok := chatCompletion("k", srv.URL, "m", "p", 0.3, time.Second); ok {
		t.Fatal("500 应失败")
	}
	// 无 choices
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	}))
	t.Cleanup(srv2.Close)
	if _, ok := chatCompletion("k", srv2.URL, "m", "p", 0.3, time.Second); ok {
		t.Fatal("空 choices 应失败")
	}
	// 连接失败
	if _, ok := chatCompletion("k", "http://127.0.0.1:0", "m", "p", 0.3, time.Second); ok {
		t.Fatal("连接失败应返回 false")
	}
	// 正常（顺带验证 Bearer 头）
	srv3 := chatSrv(t, "  hi  ", 0)
	if text, ok := chatCompletion("test-key", srv3.URL, "m", "p", 0.3, time.Second); !ok || text != "hi" {
		t.Fatalf("text = %q, ok = %v", text, ok)
	}
	// 错误的 Key → 401 失败
	if _, ok := chatCompletion("wrong", srv3.URL, "m", "p", 0.3, time.Second); ok {
		t.Fatal("错误 Key 应 401 失败")
	}
}

func overrideCover(t *testing.T, url string) {
	t.Helper()
	old := CoverEndpointOverride
	CoverEndpointOverride = url
	t.Cleanup(func() { CoverEndpointOverride = old })
}

func coverSrv(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, "boom")
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

const coverDataURL = "data:image/png;base64," + "ZmFrZS1wbmctYnl0ZXM=" // "fake-png-bytes"

func TestGenerateCoverImageNoKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	if ok, msg := GenerateCoverImage("t", "d", "c", "", ""); ok {
		t.Fatalf("无 Key 应失败: %v", msg)
	}
}

func TestGenerateCoverImage(t *testing.T) {
	// images 数组形态
	srv := coverSrv(t, fmt.Sprintf(
		`{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":%q}}]}}]}`, coverDataURL), 0)
	overrideCover(t, srv.URL)
	ok, result := GenerateCoverImage("标题", "描述", "内容", "key", "model-x")
	if !ok {
		t.Fatalf("ok = %v, result = %v", ok, result)
	}
	if b, _ := result.([]byte); string(b) != "fake-png-bytes" {
		t.Fatalf("result = %v", result)
	}

	// content parts 形态
	srv2 := coverSrv(t, fmt.Sprintf(
		`{"choices":[{"message":{"content":[{"type":"image_url","image_url":{"url":%q}}]}}]}`, coverDataURL), 0)
	overrideCover(t, srv2.URL)
	if ok, result := GenerateCoverImage("t", "d", "c", "key", ""); ok {
		if b, _ := result.([]byte); string(b) != "fake-png-bytes" {
			t.Fatalf("result = %v", result)
		}
	} else {
		t.Fatalf("content parts 应成功: %v", result)
	}

	// content 字符串内嵌 data URL
	srv3 := coverSrv(t, fmt.Sprintf(
		`{"choices":[{"message":{"content":"图片如下 %s 结束"}}]}`, coverDataURL), 0)
	overrideCover(t, srv3.URL)
	if ok, _ := GenerateCoverImage("t", "", "", "key", ""); !ok {
		t.Fatal("内嵌 data URL 应成功")
	}
}

func TestGenerateCoverImageErrors(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		status  int
		wantSub string
	}{
		{"服务端错误", "", http.StatusInternalServerError, "API error 500"},
		{"无效 JSON", "not-json", 0, "invalid response"},
		{"错误字段", `{"error":{"message":"boom"}}`, 0, "API error"},
		{"无 choices", `{"choices":[]}`, 0, "no choices"},
		{"无图片", `{"choices":[{"message":{"content":"纯文本"}}]}`, 0, "No image found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			overrideCover(t, coverSrv(t, tc.body, tc.status).URL)
			ok, result := GenerateCoverImage("t", "d", "c", "key", "")
			if ok || !strings.Contains(fmt.Sprint(result), tc.wantSub) {
				t.Fatalf("ok = %v, result = %v, want 子串 %q", ok, result, tc.wantSub)
			}
		})
	}
	// images 数组带坏 URL → 落到“无图片”
	srv := coverSrv(t, `{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":"data:invalid!!"}}]}}]}`, 0)
	overrideCover(t, srv.URL)
	if ok, result := GenerateCoverImage("t", "d", "c", "key", ""); ok || !strings.Contains(fmt.Sprint(result), "No image found") {
		t.Fatalf("坏 data URL 应失败: %v", result)
	}
}

func TestExtractImageFromURL(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("bytes"))
	// data URL
	if b, ok := ExtractImageFromURL("data:image/png;base64," + b64); !ok || string(b) != "bytes" {
		t.Fatalf("data URL: %v %v", b, ok)
	}
	if _, ok := ExtractImageFromURL("data:;;;bad"); ok {
		t.Fatal("坏 data URL 应失败")
	}
	// http URL
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "img-bytes")
	}))
	t.Cleanup(srv.Close)
	if b, ok := ExtractImageFromURL(srv.URL + "/img.png"); !ok || string(b) != "img-bytes" {
		t.Fatalf("http URL: %v %v", b, ok)
	}
	// http 错误 / 404 / 未知协议
	if _, ok := ExtractImageFromURL("http://127.0.0.1:0/x"); ok {
		t.Fatal("连接失败应返回 false")
	}
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	t.Cleanup(srv404.Close)
	if _, ok := ExtractImageFromURL(srv404.URL); ok {
		t.Fatal("404 应返回 false")
	}
	if _, ok := ExtractImageFromURL("ftp://example.com/x"); ok {
		t.Fatal("非 http 协议应返回 false")
	}
}

func TestSaveGeneratedImage(t *testing.T) {
	contentDir := t.TempDir()
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)

	// 相对路径（相对 contentDir）
	ok, rel := SaveGeneratedImage("post/a/index.md", []byte("img"), contentDir, now)
	if !ok || rel != fmt.Sprintf("pics/cover_%d.png", now.Unix()) {
		t.Fatalf("ok=%v rel=%q", ok, rel)
	}
	if _, err := os.Stat(filepath.Join(contentDir, "post", "a", rel)); err != nil {
		t.Fatalf("图片未落盘: %v", err)
	}

	// 绝对路径
	abs := filepath.Join(t.TempDir(), "b", "index.md")
	ok, rel = SaveGeneratedImage(abs, []byte("img"), contentDir, now)
	if !ok {
		t.Fatalf("绝对路径应成功: %q", rel)
	}

	// pics 父路径被同名文件占据 → MkdirAll 失败
	blockDir := t.TempDir()
	blocker := filepath.Join(blockDir, "x", "index.md")
	if err := os.MkdirAll(filepath.Dir(blocker), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(blocker), "pics"), []byte("f"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, msg := SaveGeneratedImage("x/index.md", []byte("img"), blockDir, now); ok {
		t.Fatalf("MkdirAll 失败应返回错误: %v", msg)
	}

	// 写文件失败：cover_<ts>.png 已是目录
	failDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(failDir, "y", "pics", fmt.Sprintf("cover_%d.png", now.Unix())), 0o755); err != nil {
		t.Fatal(err)
	}
	if ok, _ := SaveGeneratedImage("y/index.md", []byte("img"), failDir, now); ok {
		t.Fatal("WriteFile 失败应返回错误")
	}
}

func TestHashStringAndMin(t *testing.T) {
	if hashString("a") == hashString("b") {
		t.Fatal("不同输入应有不同哈希")
	}
	if hashString("x") != hashString("x") {
		t.Fatal("相同输入哈希应一致")
	}
	if min(3, 5) != 3 || min(5, 3) != 3 {
		t.Fatal("min 行为错误")
	}
}
