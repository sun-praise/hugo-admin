package frontmatter

import (
	"strings"
	"testing"
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
	doc2, err := Parse(dumped)
	if err != nil {
		t.Fatalf("re-parse: %v (%q)", err, dumped)
	}
	if doc2.Metadata["title"] != "Round" || doc2.Content != "内容\n" {
		t.Fatalf("round trip 失败: %#v / %q", doc2.Metadata, doc2.Content)
	}
}
