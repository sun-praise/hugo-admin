package importsvc

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

	"github.com/sun-praise/hugo-admin/v3/internal/aigen"
	"github.com/sun-praise/hugo-admin/v3/internal/frontmatter"
	"github.com/sun-praise/hugo-admin/v3/internal/posts"
	"github.com/sun-praise/hugo-admin/v3/internal/realtime"
)

var fixedNow = time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)

// aiChatServer 返回 OpenAI 兼容 chat/completions 响应的假服务。
func aiChatServer(t *testing.T, content string, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, content)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// coverServer 返回 images 数组携带 data URL 的假封面服务。
func coverServer(t *testing.T) *httptest.Server {
	t.Helper()
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":%q}}]}}]}`, dataURL)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func overrideCoverEndpoint(t *testing.T, url string) {
	t.Helper()
	old := aigen.CoverEndpointOverride
	aigen.CoverEndpointOverride = url
	t.Cleanup(func() { aigen.CoverEndpointOverride = old })
}

func readArticle(t *testing.T, contentDir, path string) (string, map[string]any) {
	t.Helper()
	ok, body, fm, _ := posts.ReadFileWithFrontmatter(contentDir, path)
	if !ok {
		t.Fatalf("读取 %s 失败", path)
	}
	return body, fm
}

func TestImportDerivesTitleFromHeading(t *testing.T) {
	contentDir := t.TempDir()
	res := Import(contentDir, "file.md", []byte("# Hello Hugo\n\n正文"), "", false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)

	if res.Title != "Hello Hugo" {
		t.Fatalf("Title = %q", res.Title)
	}
	wantPath := "post/2026-10-12-Hello-Hugo/index.md"
	if res.Path != wantPath {
		t.Fatalf("Path = %q, want %q", res.Path, wantPath)
	}
	body, fm := readArticle(t, contentDir, res.Path)
	if !strings.Contains(body, "正文") {
		t.Fatalf("body = %q", body)
	}
	if fm["draft"] != true {
		t.Fatalf("导入应为草稿: %#v", fm)
	}
	// 无已有 date → 东八区当前时间
	if fm["date"] != "2026-10-12T18:00:00+08:00" {
		t.Fatalf("date = %#v", fm["date"])
	}
	if fmt.Sprintf("%v", fm["categories"]) != "[]" || fmt.Sprintf("%v", fm["tags"]) != "[]" {
		t.Fatalf("缺省分类/标签应为空数组: %#v", fm)
	}
}

func TestImportTitlePriority(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		raw      string
		title    string
		want     string
	}{
		{"显式标题优先", "f.md", "---\ntitle: FM\n---\n# Heading\n", "Explicit", "Explicit"},
		{"frontmatter 标题次之", "f.md", "---\ntitle: FM Title\n---\n# Heading\n", "", "FM Title"},
		{"正文一级标题再次", "some-file.md", "# A Heading\n", "", "A Heading"},
		{"文件名兜底", "my note.md", "正文无标题\n", "", "my note"},
		{"全部缺失用 untitled", "", "正文无标题\n", "", "untitled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Import(t.TempDir(), tc.filename, []byte(tc.raw), tc.title, false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)
			if res.Title != tc.want {
				t.Fatalf("Title = %q, want %q", res.Title, tc.want)
			}
		})
	}
}

func TestImportKeepsExistingFrontmatter(t *testing.T) {
	contentDir := t.TempDir()
	raw := "---\n" +
		"title: FM Title\n" +
		"date: 2026-01-02T03:04:05+08:00\n" +
		"description: keep-me\n" +
		"tags: [a, b]\n" +
		"categories: [cat]\n" +
		"cover: /img/old.png\n" +
		"---\n正文内容\n"
	res := Import(contentDir, "x.md", []byte(raw), "", false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)

	_, fm := readArticle(t, contentDir, res.Path)
	if fm["date"] != "2026-01-02T03:04:05+08:00" {
		t.Fatalf("已有 date 应保留: %#v", fm["date"])
	}
	if fm["description"] != "keep-me" {
		t.Fatalf("description = %#v", fm["description"])
	}
	if fm["cover"] != "/img/old.png" {
		t.Fatalf("cover = %#v", fm["cover"])
	}
	tags := fmt.Sprintf("%v", fm["tags"])
	if !strings.Contains(tags, "a") || !strings.Contains(tags, "b") {
		t.Fatalf("tags = %#v", fm["tags"])
	}
}

func TestImportUniqueDirectory(t *testing.T) {
	contentDir := t.TempDir()
	r1 := Import(contentDir, "f.md", []byte("# Same Title\n正文一"), "", false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)
	r2 := Import(contentDir, "f.md", []byte("# Same Title\n正文二"), "", false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)
	if r1.Path == r2.Path {
		t.Fatalf("重复标题应生成唯一目录: %q vs %q", r1.Path, r2.Path)
	}
	if !strings.Contains(r2.Path, "Same-Title-") {
		t.Fatalf("第二篇应带随机后缀: %q", r2.Path)
	}
}

func TestImportWriteFailure(t *testing.T) {
	// contentDir 是普通文件 → 创建目录与写入均失败
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Import(blocker, "f.md", []byte("# T\n"), "", false, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)
	if res.Path != "" {
		t.Fatalf("Path = %q, 应为空", res.Path)
	}
	if len(res.Warnings) == 0 || res.Warnings[0] != "写入文件失败" {
		t.Fatalf("Warnings = %#v", res.Warnings)
	}
}

func TestImportFrontmatterGenNoKey(t *testing.T) {
	res := Import(t.TempDir(), "f.md", []byte("# 独特标题一\n正文"), "", true, false, AICfg{}, ImageCfg{}, nil, "", fixedNow)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "AI API Key") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Warnings = %#v, 应包含未配置 AI Key 警告", res.Warnings)
	}
}

func TestImportFrontmatterGenAI(t *testing.T) {
	srv := aiChatServer(t, `{"description":"AI 摘要","tags":["ai-tag"],"categories":["ai-cat"]}`, 0)
	contentDir := t.TempDir()
	res := Import(contentDir, "f.md", []byte("# 独特标题二\n正文内容"), "", true, false,
		AICfg{APIKey: "k", BaseURL: srv.URL, Model: "m"}, ImageCfg{}, nil, "", fixedNow)

	_, fm := readArticle(t, contentDir, res.Path)
	if fm["description"] != "AI 摘要" {
		t.Fatalf("description = %#v, AI 应填充缺失字段", fm["description"])
	}
	tags := fmt.Sprintf("%v", fm["tags"])
	if !strings.Contains(tags, "ai-tag") {
		t.Fatalf("tags = %#v", fm["tags"])
	}

	// markdown 代码块包裹的响应同样可解析
	contentDir2 := t.TempDir()
	srv2 := aiChatServer(t, "```json\n{\"description\":\"代码块摘要\"}\n```", 0)
	res = Import(contentDir2, "f.md", []byte("# 独特标题三\n正文内容"), "", true, false,
		AICfg{APIKey: "k", BaseURL: srv2.URL}, ImageCfg{}, nil, "", fixedNow)
	_, fm = readArticle(t, contentDir2, res.Path)
	if fm["description"] != "代码块摘要" {
		t.Fatalf("description = %#v, 代码块包裹的 JSON 应可解析", fm["description"])
	}
}

func TestImportFrontmatterGenAIFailure(t *testing.T) {
	srv := aiChatServer(t, "", http.StatusInternalServerError)
	res := Import(t.TempDir(), "f.md", []byte("# 独特标题四\n正文内容"), "", true, false,
		AICfg{APIKey: "k", BaseURL: srv.URL}, ImageCfg{}, nil, "", fixedNow)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "frontmatter 生成失败") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Warnings = %#v, 应包含生成失败警告", res.Warnings)
	}
}

func TestImportCoverNoKey(t *testing.T) {
	res := Import(t.TempDir(), "f.md", []byte("# 独特标题五\n正文"), "", false, true,
		AICfg{}, ImageCfg{}, nil, "", fixedNow)
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "OPENROUTER_API_KEY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Warnings = %#v, 应包含未配置图片 Key 警告", res.Warnings)
	}
}

func TestImportCoverSync(t *testing.T) {
	overrideCoverEndpoint(t, coverServer(t).URL)
	contentDir := t.TempDir()
	res := Import(contentDir, "f.md", []byte("# 独特标题六\n正文"), "", false, true,
		AICfg{}, ImageCfg{APIKey: "k"}, nil, "", fixedNow)

	if res.CoverPending {
		t.Fatal("无 broker 时应同步生成封面")
	}
	wantCover := fmt.Sprintf("pics/cover_%d.png", fixedNow.Unix())
	picPath := filepath.Join(contentDir, filepath.Dir(res.Path), wantCover)
	if raw, err := os.ReadFile(picPath); err != nil || string(raw) != "fake-png-bytes" {
		t.Fatalf("封面文件 %s: %v", picPath, err)
	}
	_, fm := readArticle(t, contentDir, res.Path)
	if fm["cover"] != wantCover {
		t.Fatalf("cover = %#v, want %q", fm["cover"], wantCover)
	}
}

func TestImportCoverBackground(t *testing.T) {
	overrideCoverEndpoint(t, coverServer(t).URL)
	contentDir := t.TempDir()
	broker := realtime.NewBroker()
	ch := broker.Subscribe()
	defer broker.Unsubscribe(ch)

	res := Import(contentDir, "f.md", []byte("# 独特标题七\n正文"), "", false, true,
		AICfg{}, ImageCfg{APIKey: "k"}, broker, "test-scope", fixedNow)

	if !res.CoverPending {
		t.Fatal("有 broker + scope 时封面应转后台")
	}
	recv := func(want string) {
		t.Helper()
		select {
		case ev := <-ch:
			if ev.Name != want {
				t.Fatalf("事件 = %q, want %q（data=%s）", ev.Name, want, ev.Data)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("等待事件 %q 超时", want)
		}
	}
	recv("article_import.progress")
	recv("article_import.cover_done")

	_, fm := readArticle(t, contentDir, res.Path)
	if fm["cover"] == nil {
		t.Fatal("后台封面完成后 cover 字段应写回")
	}
}

func TestImportSlugAndHelpers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hello World", "Hello-World"},
		{"a/b\\c", "a-b-c"},
		{".trailing.", "trailing"},
		{"", "untitled"},
	}
	for _, tc := range cases {
		if got := importSlug(tc.in); got != tc.want {
			t.Fatalf("importSlug(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if !isEmpty(nil) || !isEmpty("") || !isEmpty([]any{}) || !isEmpty(map[string]any{}) {
		t.Fatal("nil/空串/空数组/空 map 应视为空")
	}
	if isEmpty("x") || isEmpty([]any{"x"}) || isEmpty(42) {
		t.Fatal("非空值不应视为空")
	}
}

// 回归：导入带 +++ frontmatter 的外部 Markdown 时应正确拆分元数据与正文。
func TestImportTOMLFrontmatter(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("+++\ntitle = \"外部文章\"\ndate = \"2019-03-05\"\ntags = [\"imported\"]\ncategories = [\"Test\"]\n+++\n\n外部正文\n")

	res := Import(dir, "外部文章.md", raw, "", false, false, AICfg{}, ImageCfg{}, nil, "", time.Now())
	if !strings.HasPrefix(res.Path, "post/") || res.Path == "" {
		t.Fatalf("path = %q warnings = %#v", res.Path, res.Warnings)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(res.Path)))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, data)
	}
	if doc.Metadata["title"] != "外部文章" {
		t.Fatalf("title = %#v", doc.Metadata["title"])
	}
	if doc.Metadata["date"] != "2019-03-05" {
		t.Fatalf("date = %#v（原 TOML 日期应保留）", doc.Metadata["date"])
	}
	if doc.Metadata["draft"] != true {
		t.Fatalf("导入应为草稿: %#v", doc.Metadata["draft"])
	}
	if doc.Content != "外部正文" { // Dump 会 strip 正文尾部空白
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestImportTOMLNativeDate(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("+++\ndate = 2018-06-01\n+++\n\n正文\n")
	res := Import(dir, "native.md", raw, "给定的标题", false, false, AICfg{}, ImageCfg{}, nil, "", time.Now())
	if res.Path == "" {
		t.Fatalf("path 为空 warnings = %#v", res.Warnings)
	}
	data, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(res.Path)))
	doc, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	// 原生 TOML 日期（time.Time）转为 RFC3339 字符串保留
	if ds, ok := doc.Metadata["date"].(string); !ok {
		t.Fatalf("date 类型 = %T", doc.Metadata["date"])
	} else if !strings.HasPrefix(ds, "2018-06-01") {
		t.Fatalf("date = %q", ds)
	}
}
