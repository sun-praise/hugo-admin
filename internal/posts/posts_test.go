package posts

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 复刻 tests/test_posts_pagination_api.py 的 fixture 构造。
func seedPagination(t *testing.T, dir string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, "post", fmt.Sprintf("p%02d.md", i))
		os.MkdirAll(filepath.Dir(path), 0o755)
		content := fmt.Sprintf("---\ntitle: P%02d\ndate: 2025-01-01\ndraft: false\ncategories:\n  - demo\ntags:\n  - pagination\n---\n# P%02d\n", i, i)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPaginationDefaultWindow(t *testing.T) {
	dir := t.TempDir()
	seedPagination(t, dir, 25)
	data := GetPosts(dir, "", "", "", 1, 10)
	if data.Total != 25 || data.Page != 1 || data.PerPage != 10 || data.TotalPages != 3 {
		t.Fatalf("分页元信息: %+v", data)
	}
	if !data.HasNext || data.HasPrev {
		t.Fatalf("has_next/has_prev 错误")
	}
	if len(data.Posts) != 10 {
		t.Fatalf("首页应 10 篇，得到 %d", len(data.Posts))
	}
}

func TestPaginationPage2Disjoint(t *testing.T) {
	dir := t.TempDir()
	seedPagination(t, dir, 25)
	p1 := GetPosts(dir, "", "", "", 1, 10)
	p2 := GetPosts(dir, "", "", "", 2, 10)
	if p2.Page != 2 || !p2.HasNext || !p2.HasPrev {
		t.Fatalf("第 2 页元信息: %+v", p2)
	}
	seen := map[string]bool{}
	for _, p := range p1.Posts {
		seen[p.Path] = true
	}
	for _, p := range p2.Posts {
		if seen[p.Path] {
			t.Fatalf("页间重叠: %s", p.Path)
		}
	}
}

func TestPaginationLastPagePartial(t *testing.T) {
	dir := t.TempDir()
	seedPagination(t, dir, 25)
	data := GetPosts(dir, "", "", "", 3, 10)
	if data.HasNext || !data.HasPrev || len(data.Posts) != 5 {
		t.Fatalf("末页: %+v, posts=%d", data, len(data.Posts))
	}
}

func TestPaginationEmpty(t *testing.T) {
	data := GetPosts(t.TempDir(), "", "", "", 1, 20)
	if data.Total != 0 || data.Page != 1 || data.TotalPages != 0 || data.HasNext || data.HasPrev {
		t.Fatalf("空目录: %+v", data)
	}
	if data.Posts == nil || len(data.Posts) != 0 {
		t.Fatalf("posts 应为空数组（非 null）: %#v", data.Posts)
	}
}

func TestSortByDateDescAndUndatedLast(t *testing.T) {
	dir := t.TempDir()
	write := func(name, date, title string) {
		path := filepath.Join(dir, "post", name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		fm := fmt.Sprintf("---\ntitle: %s\n", title)
		if date != "" {
			fm += fmt.Sprintf("date: %s\n", date)
		}
		fm += "---\n内容\n"
		os.WriteFile(path, []byte(fm), 0o644)
	}
	write("newest.md", "2026-09-01", "Newest")
	write("oldest.md", "2020-01-01", "Oldest")
	write("mid.md", "2023-06-15T10:00:00Z", "Mid")
	write("nodate.md", "", "NoDate")

	got := GetBlogPosts(dir)
	titles := make([]string, len(got))
	for i, p := range got {
		titles[i] = p.Title
	}
	want := []string{"Newest", "Mid", "Oldest", "NoDate"}
	for i := range want {
		if titles[i] != want[i] {
			t.Fatalf("排序 = %v, want %v", titles, want)
		}
	}
}

func TestFilterSearchCategoryTag(t *testing.T) {
	dir := t.TempDir()
	write := func(name, title, body string, cats, tags []string) {
		path := filepath.Join(dir, "post", name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		catsY, tagsY := "", ""
		for _, c := range cats {
			catsY += fmt.Sprintf("  - %s\n", c)
		}
		for _, g := range tags {
			tagsY += fmt.Sprintf("  - %s\n", g)
		}
		os.WriteFile(path, []byte(fmt.Sprintf("---\ntitle: %s\ndate: 2025-05-01\ncategories:\n%stags:\n%s---\n%s\n", title, catsY, tagsY, body)), 0o644)
	}
	write("a.md", "Alpha 文章", "包含关键词 搜索命中", []string{"go"}, []string{"重构"})
	write("b.md", "Beta 文章", "无关内容", []string{"python"}, []string{"重构"})
	write("c.md", "Gamma", "另一个关键词", []string{"go"}, []string{"杂项"})

	if got := GetPosts(dir, "搜索命中", "", "", 1, 20).Total; got != 1 {
		t.Fatalf("正文搜索 total = %d", got)
	}
	if got := GetPosts(dir, "alpha 文章", "", "", 1, 20).Total; got != 1 {
		t.Fatalf("标题搜索（小写化）total = %d", got)
	}
	if got := GetPosts(dir, "", "go", "", 1, 20).Total; got != 2 {
		t.Fatalf("分类过滤 total = %d", got)
	}
	if got := GetPosts(dir, "", "", "重构", 1, 20).Total; got != 2 {
		t.Fatalf("标签过滤 total = %d", got)
	}
}

func TestTagsCategoriesAggregate(t *testing.T) {
	dir := t.TempDir()
	write := func(name, title string, cats, tags []string) {
		path := filepath.Join(dir, "post", name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		var catsY, tagsY string
		for _, c := range cats {
			catsY += fmt.Sprintf("  - %s\n", c)
		}
		for _, g := range tags {
			tagsY += fmt.Sprintf("  - %s\n", g)
		}
		os.WriteFile(path, []byte(fmt.Sprintf("---\ntitle: %s\ndate: 2025-05-0%d\ncategories:\n%stags:\n%s---\nx\n", title, len(name), catsY, tagsY)), 0o644)
	}
	write("a.md", "A", []string{"go"}, []string{"x"})
	write("b.md", "B", []string{"go"}, []string{"x", "y"})
	write("c.md", "C", []string{"py"}, []string{"y"})

	tags := GetAllTags(dir)
	// x=2, y=2（同数稳定序：x 首见在前），首见序按日期降序扫描
	if len(tags) != 2 || tags[0].Name != "x" || tags[0].Count != 2 || tags[1].Name != "y" || tags[1].Count != 2 {
		t.Fatalf("tags = %#v", tags)
	}
	cats := GetAllCategories(dir)
	if cats[0].Name != "go" || cats[0].Count != 2 || cats[1].Name != "py" || cats[1].Count != 1 {
		t.Fatalf("categories = %#v", cats)
	}
}

func TestExcerpt(t *testing.T) {
	long := make([]rune, 300)
	for i := range long {
		long[i] = '字'
	}
	text := "# 标题 [链接](http://x) *强调* `代码` " + string(long)
	got := generateExcerpt(text)
	if len([]rune(got)) != 203 { // 200 + "..."
		t.Fatalf("excerpt 长度 = %d", len([]rune(got)))
	}
	if containsAny(got, "#", "[", "`", "*") {
		t.Fatalf("excerpt 未剥离 markdown: %q", got[:40])
	}
	if !endsWith(got, "...") {
		t.Fatal("excerpt 应以 ... 结尾")
	}
}

func containsAny(s string, chars ...string) bool {
	for _, c := range chars {
		if len(c) > 0 && indexOf(s, c) >= 0 {
			return true
		}
	}
	return false
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func TestResolveCoverURL(t *testing.T) {
	cases := []struct{ rel, cover, want string }{
		{"post/a.md", "", ""},
		{"post/a.md", "https://cdn.example.com/x.jpg", "https://cdn.example.com/x.jpg"},
		{"post/a.md", "/static/x.jpg", "/static/x.jpg"},
		{"post/a.md", "cover.jpg", "/content/post/cover.jpg"},
		{"post/sub/b.md", "../shared/x.jpg", "/content/post/shared/x.jpg"},
		{"post/sub/b.md", "../../escape.jpg", "/content/escape.jpg"},
	}
	for _, c := range cases {
		if got := ResolveCoverURL(c.rel, c.cover); got != c.want {
			t.Errorf("ResolveCoverURL(%q, %q) = %q, want %q", c.rel, c.cover, got, c.want)
		}
	}
}

func TestCoverFallbackChain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post", "c.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("---\ntitle: C\ndate: 2025-01-01\nimages:\n  - img1.jpg\n  - img2.jpg\n---\n内容\n"), 0o644)
	p := parsePost(path, dir)
	if p.Cover != "img1.jpg" {
		t.Fatalf("cover 兜底链失败: %q", p.Cover)
	}
}

func TestModTimeAndDateFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post", "t.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("---\ntitle: T\ndate: 2026-03-05\ntags: go\n---\n内容\n"), 0o644)

	result := GetPosts(dir, "", "", "", 1, 20)
	if len(result.Posts) != 1 {
		t.Fatalf("posts = %d", len(result.Posts))
	}
	p := result.Posts[0]
	if p.Date != "2026-03-05" {
		t.Fatalf("date = %q", p.Date)
	}
	// tags 单字符串应包成列表
	if len(p.Tags) != 1 || p.Tags[0] != "go" {
		t.Fatalf("tags = %#v", p.Tags)
	}
	if _, err := time.Parse("2006-01-02 15:04", p.ModTime); err != nil {
		t.Fatalf("mod_time 格式错误: %q", p.ModTime)
	}
	if p.Path != "post/t.md" {
		t.Fatalf("relative path = %q", p.Path)
	}
}

func TestParseDateFormats(t *testing.T) {
	cases := map[string]string{
		"2026-01-02T15:04:05+08:00": "2026-01-02", // RFC3339
		"2026-01-02 15:04:05+08:00": "2026-01-02", // 空格分隔（旧文章带引号保存）
		"2026-01-02 15:04:05":       "2026-01-02",
		"2026-01-02":                "2026-01-02",
		"2026-01-02T15:04:05Z":      "2026-01-02",
	}
	for in, wantDate := range cases {
		ts := parseDate(in)
		if ts == nil {
			t.Errorf("parseDate(%q) = nil", in)
			continue
		}
		if got := ts.Format("2006-01-02"); got != wantDate {
			t.Errorf("parseDate(%q) = %s, want %s", in, got, wantDate)
		}
	}
	for _, bad := range []any{"not-a-date", "", 42} {
		if ts := parseDate(bad); ts != nil {
			t.Errorf("parseDate(%v) 应为 nil，得到 %v", bad, ts)
		}
	}
}

func TestGetPostsTOMLFrontmatter(t *testing.T) {
	// 回归：+++ TOML frontmatter 此前被当正文，列表里 title/date/tags 全空
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "post"), 0o755); err != nil {
		t.Fatal(err)
	}
	tomlPost := "+++\nauthor = \"Hugo Authors\"\ntitle = \"Emoji Support\"\ndate = \"2019-03-05\"\ncategories = [\n    \"Test\"\n]\ntags = [\n    \"emoji\",\n]\n+++\n\nEmoji 内容\n"
	if err := os.WriteFile(filepath.Join(dir, "post", "emoji-support.md"), []byte(tomlPost), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "post", "yaml.md"),
		[]byte("---\ntitle: Yaml\ndate: 2026-01-01\n---\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	data := GetPosts(dir, "", "", "", 1, 20)
	if data.Total != 2 {
		t.Fatalf("total = %d", data.Total)
	}
	var emojiIdx = -1
	for i := range data.Posts {
		if data.Posts[i].Title == "Emoji Support" {
			emojiIdx = i
		}
	}
	if emojiIdx < 0 {
		t.Fatalf("TOML 文章缺失: %+v", data.Posts)
	}
	emoji := data.Posts[emojiIdx]
	if emoji.Date != "2019-03-05" {
		t.Fatalf("date = %v", emoji.Date)
	}
	if len(emoji.Tags) != 1 || emoji.Tags[0] != "emoji" {
		t.Fatalf("tags = %#v", emoji.Tags)
	}
	if len(emoji.Categories) != 1 || emoji.Categories[0] != "Test" {
		t.Fatalf("categories = %#v", emoji.Categories)
	}
	if emoji.Excerpt == "" {
		t.Fatalf("excerpt 为空: %+v", emoji)
	}
	// TOML 原生日期（无引号）同样解出日期
	if err := os.WriteFile(filepath.Join(dir, "post", "native.md"),
		[]byte("+++\ntitle = \"Native\"\ndate = 2018-06-01\n+++\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	data = GetPosts(dir, "", "", "", 1, 20)
	for _, p := range data.Posts {
		if p.Title == "Native" && p.Date != "2018-06-01" {
			t.Fatalf("原生 date = %v", p.Date)
		}
	}
}
