package posts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	// TOML 原生日期/整数：time.Time → RFC3339（parseDate 可再解析），
	// int64（go-toml 整数）保留数值
	writeFile(t, filepath.Join(dir, "post", "e.md"),
		"+++\ntitle = \"E\"\ndate = 2019-03-05\nweight = 10\n+++\n\n正文\n")
	_, _, fm, _ = ReadFileWithFrontmatter(dir, "post/e.md")
	if ds, ok := fm["date"].(string); !ok {
		t.Fatalf("date 应转字符串: %#v", fm["date"])
	} else if _, err := time.Parse(time.RFC3339, ds); err != nil {
		t.Fatalf("date 应为 RFC3339，得到 %q: %v", ds, err)
	}
	if w, ok := fm["weight"].(int64); !ok || w != 10 {
		t.Fatalf("weight 应保留 int64: %#v", fm["weight"])
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

func TestSaveFileTOMLEdgeCases(t *testing.T) {
	// BOM 前缀的 +++ 文件也应被识别为 TOML
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "bom.md"), "\uFEFF+++\ntitle = \"旧\"\n+++\n\n旧\n")
	ok, _, _ := SaveFile(dir, "post/bom.md", "+++\ntitle = \"旧\"\n+++\n\n新\n",
		map[string]any{"title": "新"}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "bom.md"))
	if !strings.HasPrefix(string(data), "+++\n") {
		t.Fatalf("BOM 文件应保留 +++: %q", data)
	}

	// 新文件：内容以 +++ 开头时按 TOML 写，而非默认 YAML
	ok, _, _ = SaveFile(dir, "post/new.md", "+++\ntitle = \"x\"\n+++\n\n新文件正文\n",
		map[string]any{"title": "x", "weight": 10}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ = os.ReadFile(filepath.Join(dir, "post", "new.md"))
	if !strings.HasPrefix(string(data), "+++\n") {
		t.Fatalf("新文件应按内容识别为 +++: %q", data)
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
		// +++ TOML 同款语义
		{"+++\na = 1\n+++\n\n正文", "正文"},
		{"+++\na = 1\n+++\n+++\nb = 2\n+++\n\n双层后的正文", "双层后的正文"},
		{"+++\n未闭合", "+++\n未闭合"},
		{"---\na: 1\n---\n+++\nb = 2\n+++\n\nYAML 后跟 TOML 块", "YAML 后跟 TOML 块"},
		{"\n\n---\na: 1\n---\n\n前导空行正文", "前导空行正文"},
		{"\n\n+++\na = 1\n+++\n\n前导空行正文", "前导空行正文"},
	}
	for _, c := range cases {
		if got := StripLeadingFrontmatter(c.in); got != c.want {
			t.Errorf("strip(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// 回归：形似 +++ 但非法的文档读回时不得剥块——否则编辑保存一次
// 就静默丢掉开头的 +++ 块（评审指出的行为回退）。
func TestReadSaveRoundTripDegradedTOML(t *testing.T) {
	dir := t.TempDir()
	original := "+++\n这是正文开头的一行\n+++\n继续正文\n"
	writeFile(t, filepath.Join(dir, "post", "deg.md"), original)

	ok, body, fm, _ := ReadFileWithFrontmatter(dir, "post/deg.md")
	if !ok {
		t.Fatal("read 失败")
	}
	if len(fm) != 0 {
		t.Fatalf("降级文档 fm 应为空: %#v", fm)
	}
	if body != original {
		t.Fatalf("降级文档正文被剥块:\n got %q\nwant %q", body, original)
	}

	// fmData 为空时 SaveFile 原样写回，不丢块
	ok, _, _ = SaveFile(dir, "post/deg.md", body, nil, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "deg.md"))
	if string(data) != original {
		t.Fatalf("round trip 丢内容:\n got %q\nwant %q", data, original)
	}
}

func TestParseSupportsBOM(t *testing.T) {
	// BOM + +++：Parse 应识别 frontmatter（对齐 Hugo）
	doc, err := frontmatter.Parse([]byte("\uFEFF+++\ntitle = \"B\"\n+++\n\n正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != "B" || !doc.TOML {
		t.Fatalf("doc = %#v", doc.Metadata)
	}
	if doc.Content != "正文\n" {
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestSaveFileTOMLLeadingBlankLines(t *testing.T) {
	// 分隔线前有空行的 +++ 文件：按 TOML 保留（避免改写成 ---）
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "blank.md"), "\n\n+++\ntitle = \"旧\"\n+++\n\n旧\n")
	ok, _, _ := SaveFile(dir, "post/blank.md", "\n\n+++\ntitle = \"旧\"\n+++\n\n新\n",
		map[string]any{"title": "新"}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "blank.md"))
	if !strings.Contains(string(data), "+++") || strings.Contains(string(data), "---") {
		t.Fatalf("应保留 +++ 格式: %q", data)
	}
}

func TestReadFileWithFrontmatterNestedNormalize(t *testing.T) {
	// 嵌套表/数组内的 TOML 日期 → RFC3339 字符串（normalizeEditorValue 递归）
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "post", "nested.md"),
		"+++\ntitle = \"N\"\ndates = [2019-03-05, 2020-01-02]\n[extra]\nwhen = 2021-06-30\n+++\n\n正文\n")
	_, _, fm, _ := ReadFileWithFrontmatter(dir, "post/nested.md")
	list, ok := fm["dates"].([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("dates = %#v", fm["dates"])
	}
	for i, want := range []string{"2019-03-05", "2020-01-02"} {
		s, ok := list[i].(string)
		if !ok {
			t.Fatalf("dates[%d] 类型 = %T", i, list[i])
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			t.Fatalf("dates[%d] = %q 非 RFC3339: %v", i, s, err)
		}
		if !strings.HasPrefix(s, want) {
			t.Fatalf("dates[%d] = %q, want %q*", i, s, want)
		}
	}
	extra, ok := fm["extra"].(map[string]any)
	if !ok {
		t.Fatalf("extra = %#v", fm["extra"])
	}
	when, ok := extra["when"].(string)
	if !ok || !strings.HasPrefix(when, "2021-06-30") {
		t.Fatalf("extra.when = %#v", extra["when"])
	}
}

// 回归：降级文档 + 非空 fmData 保存时同样不得丢开头的 +++ 块
func TestSaveFileDegradedTOMLWithFM(t *testing.T) {
	dir := t.TempDir()
	original := "+++\n这是正文开头的一行\n+++\n继续正文\n"
	writeFile(t, filepath.Join(dir, "post", "deg.md"), original)

	// 编辑器读回
	ok, _, _, _ := ReadFileWithFrontmatter(dir, "post/deg.md")
	if !ok {
		t.Fatal("read 失败")
	}
	// 用户在元数据面板加了 title 后保存
	ok, _, _ = SaveFile(dir, "post/deg.md", original, map[string]any{"title": "加了字段"}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "deg.md"))
	text := string(data)
	if !strings.Contains(text, "这是正文开头的一行") || !strings.Contains(text, "继续正文") {
		t.Fatalf("原正文丢失: %q", text)
	}
	doc, err := frontmatter.Parse(data)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, text)
	}
	if doc.Metadata["title"] != "加了字段" {
		t.Fatalf("title = %#v", doc.Metadata["title"])
	}
}

// 回归：分隔线前有空行的 +++ 文件读侧也应识别（与写侧口径一致），
// read→save 往返不丢块
func TestReadSaveRoundTripBlankLineTOML(t *testing.T) {
	dir := t.TempDir()
	original := "\n\n+++\ntitle = \"B\"\n+++\n\n正文\n"
	writeFile(t, filepath.Join(dir, "post", "blank.md"), original)

	_, body, fm, _ := ReadFileWithFrontmatter(dir, "post/blank.md")
	if fm["title"] != "B" {
		t.Fatalf("fm = %#v（空行前缀的 +++ 应被识别）", fm)
	}
	if body != "正文\n" {
		t.Fatalf("body = %q", body)
	}

	ok, _, _ := SaveFile(dir, "post/blank.md", "\n\n+++\ntitle = \"B\"\n+++\n\n正文\n",
		map[string]any{"title": "B"}, nil)
	if !ok {
		t.Fatal("save 失败")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "blank.md"))
	doc, err := frontmatter.Parse(data)
	if err != nil || doc.Metadata["title"] != "B" || !doc.TOML {
		t.Fatalf("round trip: %v %#v (%q)", err, doc.Metadata, data)
	}
}
