package refs

import (
	"os"
	"path/filepath"
	"testing"
)

// 回归：TOML（+++）文章的 date 已归一化为 time.Time，
// parseForCache 需按 time.Time 分支取值，否则 DB 缓存 date 恒空。
func TestParseForCacheTOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post", "emoji.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path,
		[]byte("+++\ntitle = \"Emoji Support\"\ndate = \"2019-03-05\"\ntags = [\"emoji\"]\n+++\n\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := parseForCache(dir, path)
	if meta == nil {
		t.Fatal("meta 为 nil")
	}
	if meta.title != "Emoji Support" {
		t.Fatalf("title = %q", meta.title)
	}
	if meta.date != "2019-03-05" {
		t.Fatalf("date = %q", meta.date)
	}
	if len(meta.tags) != 1 || meta.tags[0] != "emoji" {
		t.Fatalf("tags = %#v", meta.tags)
	}
}

func TestParseForCacheTOMLNativeDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "post", "native.md")
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path,
		[]byte("+++\ntitle = \"Native\"\ndate = 2018-06-01\n+++\n正文\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := parseForCache(dir, path)
	if meta == nil {
		t.Fatal("meta 为 nil")
	}
	if meta.date != "2018-06-01" {
		t.Fatalf("date = %q", meta.date)
	}
}
