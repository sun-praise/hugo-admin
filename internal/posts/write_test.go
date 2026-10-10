package posts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/svtter/hugo-admin/internal/frontmatter"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "a.md"), "---\ntitle: A\n---\n正文\n")

	ok, content, mtime := ReadFile(dir, "post/a.md")
	if !ok || content != "---\ntitle: A\n---\n正文\n" || mtime <= 0 {
		t.Fatalf("read = %v %q %v", ok, content, mtime)
	}

	// 绝对路径同样可读
	ok, _, _ = ReadFile(dir, filepath.Join(dir, "post", "a.md"))
	if !ok {
		t.Fatal("绝对路径读取失败")
	}

	// 不存在
	ok, msg, _ := ReadFile(dir, "post/none.md")
	if ok || !strings.Contains(msg, "文件不存在") {
		t.Fatalf("不存在: %v %q", ok, msg)
	}

	// 越界路径
	ok, msg, _ = ReadFile(dir, "../outside.md")
	if ok || !strings.Contains(msg, "访问被拒绝") {
		t.Fatalf("越界: %v %q", ok, msg)
	}
}

func TestReadFileWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "a.md"),
		"---\ntitle: A\ndate: 2026-01-02\ntags:\n  - go\n---\n\n正文开始\n")

	ok, body, fm, mtime := ReadFileWithFrontmatter(dir, "post/a.md")
	if !ok || mtime <= 0 {
		t.Fatalf("ok=%v mtime=%v", ok, mtime)
	}
	if fm["title"] != "A" || body != "正文开始\n" {
		t.Fatalf("fm=%#v body=%q", fm, body)
	}

	// 正文里又出现 --- 分隔线：只切文件头首对
	writeFile(t, filepath.Join(dir, "post", "b.md"),
		"---\ntitle: B\n---\n---\ninner\n---\nreal body\n")
	_, body, fm, _ = ReadFileWithFrontmatter(dir, "post/b.md")
	if fm["title"] != "B" {
		t.Fatalf("fm=%#v", fm)
	}
	// 双重 frontmatter 被剥离
	if body != "real body\n" {
		t.Fatalf("双重剥离 body=%q", body)
	}

	// 非 dict 的 frontmatter → 空 map
	writeFile(t, filepath.Join(dir, "post", "c.md"), "---\n- a\n- b\n---\n内容\n")
	_, body, fm, _ = ReadFileWithFrontmatter(dir, "post/c.md")
	if len(fm) != 0 || body != "内容\n" {
		t.Fatalf("非 dict fm=%#v body=%q", fm, body)
	}

	// TOML frontmatter（+++）
	writeFile(t, filepath.Join(dir, "post", "d.md"),
		"+++\ntitle = \"D\"\ndate = \"2019-03-05\"\ntags = [\"emoji\"]\n+++\n\nTOML 正文\n")
	_, body, fm, _ = ReadFileWithFrontmatter(dir, "post/d.md")
	if fm["title"] != "D" || body != "TOML 正文\n" {
		t.Fatalf("toml fm=%#v body=%q", fm, body)
	}
	tags, _ := fm["tags"].([]any)
	if len(tags) != 1 || tags[0] != "emoji" {
		t.Fatalf("toml tags=%#v", fm["tags"])
	}
}

func TestSaveFilePlain(t *testing.T) {
	dir := t.TempDir()
	ok, msg, mtime := SaveFile(dir, "post/new.md", "# 新内容\n", nil, nil)
	if !ok || msg != "文件保存成功" || mtime <= 0 {
		t.Fatalf("save = %v %q %v", ok, msg, mtime)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "new.md"))
	if string(data) != "# 新内容\n" {
		t.Fatalf("内容 = %q", data)
	}
}

func TestSaveFileWithFrontmatter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "a.md"), "旧内容\n")

	ok, _, mtime := SaveFile(dir, "post/a.md", "---\ntitle: old\n---\n\n新正文\n",
		map[string]any{"title": "新标题", "tags": []any{"go"}, "draft": true}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "a.md"))
	text := string(data)
	// 头部 frontmatter + 剥离正文中重复的 frontmatter 块；
	// dumps 会 strip 正文首尾空白（对齐 python-frontmatter）
	if !strings.HasPrefix(text, "---\n") {
		t.Fatalf("无 frontmatter: %q", text)
	}
	if strings.Count(text, "title:") != 1 {
		t.Fatalf("双重 frontmatter 未剥离: %q", text)
	}
	if !strings.HasSuffix(text, "\n---\n\n新正文") {
		t.Fatalf("正文错误: %q", text)
	}
	_ = mtime
}

func TestSaveFilePreservesTOML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "toml.md"),
		"+++\ntitle = \"旧\"\ntags = [\"a\"]\n+++\n\n旧正文\n")

	ok, _, _ := SaveFile(dir, "post/toml.md", "+++\ntitle = \"旧\"\n+++\n\n新正文\n",
		map[string]any{"title": "新", "tags": []any{"a", "b"}}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "toml.md"))
	text := string(data)
	if !strings.HasPrefix(text, "+++\n") {
		t.Fatalf("应保留 +++ 格式: %q", text)
	}
	// 按 Parse 验证内容（不依赖 go-toml 的引号风格）
	doc, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, text)
	}
	if doc.Metadata["title"] != "新" || !doc.TOML {
		t.Fatalf("metadata = %#v", doc.Metadata)
	}
	if doc.Content != "新正文" {
		t.Fatalf("正文 = %q（前导 +++ 块应被剥离）", doc.Content)
	}
}

func TestSaveFileOptimisticLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post", "a.md")
	writeFile(t, path, "原始内容\n")
	st, _ := os.Stat(path)
	realMtime := float64(st.ModTime().UnixNano()) / 1e9

	// 错误的期望 mtime → 冲突
	stale := realMtime - 10
	ok, msg, mtime := SaveFile(dir, "post/a.md", "覆盖\n", nil, &stale)
	if ok || mtime != 0 {
		t.Fatalf("应冲突: %v %v", ok, mtime)
	}
	conflict, isConflict := msg.(ConflictInfo)
	if !isConflict || !conflict.Conflict || conflict.CurrentContent != "原始内容\n" ||
		conflict.Message != "文件已被其他人修改" {
		t.Fatalf("conflict = %#v", msg)
	}

	// 正确的期望 mtime → 成功
	ok, _, _ = SaveFile(dir, "post/a.md", "覆盖\n", nil, &realMtime)
	if !ok {
		t.Fatal("正确 mtime 保存失败")
	}
	// 文件不存在时 expected_mtime 不拦截
	missing := realMtime
	ok, _, _ = SaveFile(dir, "post/brand-new.md", "新文件\n", nil, &missing)
	if !ok {
		t.Fatal("新文件不应触发乐观锁")
	}
}

func TestCreatePost(t *testing.T) {
	dir := t.TempDir()
	ok, rel := CreatePost(dir, "我的 Go 文章!")
	if !ok {
		t.Fatalf("create 失败: %s", rel)
	}
	if !strings.HasPrefix(rel, "post/") || !strings.Contains(rel, "-我的-Go-文章") || !strings.HasSuffix(rel, "/index.md") {
		t.Fatalf("rel = %q", rel)
	}
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"title: 我的 Go 文章!", "draft: true", "categories: []", "tags: []", "在这里编写你的文章内容..."} {
		if !strings.Contains(text, want) {
			t.Fatalf("缺少 %q:\n%s", want, text)
		}
	}
	if !strings.Contains(text, "date: 20") || !strings.HasSuffix(strings.SplitN(strings.Split(text, "date: ")[1], "\n", 2)[0], "+08:00") {
		t.Fatalf("date 格式错误:\n%s", text)
	}
}

func TestSlugifyTitle(t *testing.T) {
	cases := map[string]string{
		"Hello World":      "Hello-World",
		"我的文章":             "我的文章",
		"  spaced  out  ":  "spaced-out",
		"a///b???c":        "a-b-c",
		"!!!":              "untitled",
		"":                 "untitled",
		"保持-under_score-和": "保持-under_score-和",
	}
	for in, want := range cases {
		if got := SlugifyTitle(in); got != want {
			t.Errorf("SlugifyTitle(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripLeadingFrontmatter(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"---\na: 1\n---\n\n正文", "正文"},
		{"---\na: 1\n---\n---\nb: 2\n---\n\n双层后的正文", "双层后的正文"},
		{"---\n未闭合", "---\n未闭合"},
		{"直接正文", "直接正文"},
	}
	for _, c := range cases {
		if got := StripLeadingFrontmatter(c.in); got != c.want {
			t.Errorf("strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
