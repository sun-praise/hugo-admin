package frontmatter

import (
	"strings"
	"testing"
	"time"
)

func TestParseWithFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: 标题\ntags:\n  - go\n  - hugo\ndraft: false\n---\n\n# 正文\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != "标题" {
		t.Fatalf("title = %v", doc.Metadata["title"])
	}
	if doc.Metadata["draft"] != false {
		t.Fatalf("draft = %v", doc.Metadata["draft"])
	}
	tags, _ := doc.Metadata["tags"].([]any)
	if len(tags) != 2 || tags[0] != "go" {
		t.Fatalf("tags = %#v", doc.Metadata["tags"])
	}
	// frontmatter.loads 语义：分隔线后的首个换行被剥离
	if doc.Content != "# 正文\n" && strings.TrimPrefix(doc.Content, "\n") != "# 正文\n" {
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestParseWithoutFrontmatter(t *testing.T) {
	doc, err := Parse([]byte("# 纯正文\n"))
	if err != nil || doc.Metadata["title"] != nil {
		t.Fatalf("doc = %#v, err = %v", doc, err)
	}
	if doc.Content != "# 纯正文\n" {
		t.Fatalf("content = %q", doc.Content)
	}
}

func TestParseUnclosedFrontmatter(t *testing.T) {
	// 无闭合分隔线：对齐 frontmatter.loads，整体视为正文
	doc, err := Parse([]byte("---\ntitle: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != nil && doc.Content == "" {
		t.Fatalf("未闭合应视为正文: %#v", doc)
	}
}

func TestDumpRoundTrip(t *testing.T) {
	doc, err := Parse([]byte("---\ntitle: Round\nnum: 42\n---\n内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dumped := doc.Dump()
	// python-frontmatter.dumps 的确切形态（实测字节）
	want := "---\nnum: 42\ntitle: Round\n---\n\n内容"
	if string(dumped) != want {
		t.Fatalf("dump = %q, want %q", dumped, want)
	}
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if doc2.Metadata["title"] != "Round" || doc2.Content != "内容" {
		t.Fatalf("round trip 失败: %#v / %q", doc2.Metadata, doc2.Content)
	}
}

func TestParseTOML(t *testing.T) {
	// Fried-Rice exampleSite 风格：+++ + 引号日期 + 多行数组
	doc, err := Parse([]byte("+++\nauthor = \"Hugo Authors\"\ntitle = \"Emoji Support\"\ndate = \"2019-03-05\"\ncategories = [\n    \"Test\"\n]\ntags = [\n    \"emoji\",\n]\nimage = \"cover.jpg\"\n+++\n\nEmoji 内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.Metadata["title"] != "Emoji Support" {
		t.Fatalf("title = %v", doc.Metadata["title"])
	}
	cats, _ := doc.Metadata["categories"].([]any)
	if len(cats) != 1 || cats[0] != "Test" {
		t.Fatalf("categories = %#v", doc.Metadata["categories"])
	}
	tags, _ := doc.Metadata["tags"].([]any)
	if len(tags) != 1 || tags[0] != "emoji" {
		t.Fatalf("tags = %#v", doc.Metadata["tags"])
	}
	if doc.Content != "Emoji 内容\n" {
		t.Fatalf("content = %q", doc.Content)
	}
	if !doc.TOML {
		t.Fatal("应标记为 TOML 文档")
	}
}

func TestParseTOMLNativeDate(t *testing.T) {
	// TOML 原生日期（无引号）解成 time.Time，与 YAML 原生日期同路
	doc, err := Parse([]byte("+++\ntitle = \"x\"\ndate = 2019-03-05\n+++\nbody\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ts, ok := doc.Metadata["date"].(time.Time)
	if !ok {
		t.Fatalf("date 类型 = %T", doc.Metadata["date"])
	}
	if ts.Year() != 2019 || ts.Month() != time.March || ts.Day() != 5 {
		t.Fatalf("date = %v", ts)
	}
}

func TestDumpTOMLRoundTrip(t *testing.T) {
	doc, err := Parse([]byte("+++\ntitle = \"Round\"\ndate = \"2019-03-05\"\n+++\n内容\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	dumped := doc.Dump()
	if !strings.HasPrefix(string(dumped), "+++\n") {
		t.Fatalf("dump 应保留 +++ 格式: %q", dumped)
	}
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if doc2.Metadata["title"] != "Round" || doc2.Content != "内容" {
		t.Fatalf("round trip 失败: %#v / %q", doc2.Metadata, doc2.Content)
	}
}
