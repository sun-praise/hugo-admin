package posts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seedPublishFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"post/draft.md":   "---\ntitle: 草稿\ndraft: true\ntags:\n  - go\n---\n\n草稿正文\n",
		"post/live.md":    "---\ntitle: 已发布\ndraft: false\n---\n\n正文\n",
		"post/nodraft.md": "---\ntitle: 无标记\n---\n\n无 draft 字段\n",
	}
	for rel, content := range files {
		path := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestGetPublishStatus(t *testing.T) {
	dir := seedPublishFixture(t)

	st := GetPublishStatus(dir, "post/draft.md")
	if st["is_draft"] != true || st["is_publishable"] != true {
		t.Fatalf("draft 状态 = %#v", st)
	}
	if errs := st["publish_errors"].([]string); len(errs) != 0 {
		t.Fatalf("publish_errors = %#v", errs)
	}

	st = GetPublishStatus(dir, "post/live.md")
	if st["is_draft"] != false || st["is_publishable"] != false {
		t.Fatalf("live 状态 = %#v", st)
	}
	if st["last_published"] != nil {
		t.Fatalf("无 publishDate 时 last_published 应为 nil: %#v", st["last_published"])
	}

	// draft 字段缺省 → 视为草稿
	st = GetPublishStatus(dir, "post/nodraft.md")
	if st["is_draft"] != true {
		t.Fatalf("缺省 draft 状态 = %#v", st)
	}

	// 不存在
	st = GetPublishStatus(dir, "post/none.md")
	if msg, _ := st["error"].(string); msg != "文件不存在" {
		t.Fatalf("不存在 = %#v", st)
	}
}

func TestPublishArticle(t *testing.T) {
	dir := seedPublishFixture(t)

	ok, msg, opID := PublishArticle(dir, "post/draft.md")
	if !ok || msg != "文章发布成功" || len(opID) != 36 {
		t.Fatalf("publish = %v %q %q", ok, msg, opID)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "post", "draft.md"))
	text := string(data)
	if !strings.Contains(text, "draft: false") {
		t.Fatalf("draft 未切换:\n%s", text)
	}
	// publishDate 补齐（yaml.v3 双引号 vs PyYAML 单引号，YAML 等价）
	if !strings.Contains(text, "publishDate: ") || !strings.Contains(text, "+08:00") {
		t.Fatalf("publishDate 未补齐:\n%s", text)
	}
	// 其他 metadata 保留
	if !strings.Contains(text, "title: 草稿") || !strings.Contains(text, "- go") {
		t.Fatalf("metadata 丢失:\n%s", text)
	}

	// 再次发布 → 已发布
	ok, msg, _ = PublishArticle(dir, "post/draft.md")
	if ok || msg != "文章已经发布" {
		t.Fatalf("重复发布 = %v %q", ok, msg)
	}

	// 非草稿 → 已发布
	ok, msg, _ = PublishArticle(dir, "post/live.md")
	if ok || msg != "文章已经发布" {
		t.Fatalf("非草稿发布 = %v %q", ok, msg)
	}

	// 不存在
	ok, msg, _ = PublishArticle(dir, "post/none.md")
	if ok || !strings.Contains(msg, "文件不存在") {
		t.Fatalf("不存在 = %v %q", ok, msg)
	}
}

func TestBulkPublishArticles(t *testing.T) {
	dir := seedPublishFixture(t)
	result := BulkPublishArticles(dir, []string{"post/draft.md", "post/none.md", "post/live.md"})

	if result["success"] != false || result["total_count"] != 3 ||
		result["published_count"] != 1 || result["failed_count"] != 2 {
		t.Fatalf("批量结果 = %#v", result)
	}
	if result["duration_ms"] != 0 {
		t.Fatalf("duration_ms = %#v", result["duration_ms"])
	}
	results := result["results"].([]map[string]any)
	if results[0]["success"] != true || results[0]["message"] != nil {
		t.Fatalf("results[0] = %#v", results[0])
	}
	publishedAt, _ := results[0]["published_at"].(string)
	if !strings.Contains(publishedAt, "+08:00") {
		t.Fatalf("published_at = %#v", results[0]["published_at"])
	}
	if results[1]["success"] != false || results[1]["message"] == nil || results[1]["published_at"] != nil {
		t.Fatalf("results[1] = %#v", results[1])
	}
}
