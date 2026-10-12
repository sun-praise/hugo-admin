package importsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/svtter/hugo-admin/internal/frontmatter"
)

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
