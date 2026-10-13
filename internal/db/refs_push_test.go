package db

import (
	"path/filepath"
	"testing"
)

func TestUpsertAndBatchReferences(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	// UpsertReferences：写入 → 同 source 再写为替换语义
	refs := []RefEntry{
		{TargetPath: "post/b/index.md", Context: "参见"},
		{TargetPath: "post/c/index.md", Context: "ref"},
	}
	if err := d.UpsertReferences("post/a/index.md", refs); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := d.UpsertReferences("post/a/index.md", refs[:1]); err != nil {
		t.Fatalf("二次 upsert: %v", err)
	}

	// BatchUpsertReferences：事务批量
	batch := map[string][]RefEntry{
		"post/x/index.md": {{TargetPath: "post/y/index.md", Context: "批量"}},
		"post/z/index.md": {},
	}
	if err := d.BatchUpsertReferences(batch); err != nil {
		t.Fatalf("batch: %v", err)
	}
}

func TestRecordPush(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	d.RecordPush("origin", "main", "a", "b", 3, "feat: x", "推送成功", true)
	d.RecordPush("origin", "main", "b", "c", 1, "fix: y", "推送失败", false)

	pushes, total, err := d.ListPushes(10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 2 || len(pushes) != 2 {
		t.Fatalf("total = %d, len = %d", total, len(pushes))
	}
	if pushes[1].Remote != "origin" || pushes[1].CommitCount != 3 {
		t.Fatalf("pushes[1] = %+v", pushes[1])
	}
	if pushes[0].Success {
		t.Fatalf("第二条应为失败记录: %+v", pushes[0])
	}
}

func TestOpenInvalidPath(t *testing.T) {
	// 数据库路径是一个目录 → 打开失败
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("目录路径应打开失败")
	}
}

func TestClosedDBErrorPaths(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	// 关闭后操作应走错误分支而非 panic
	if err := d.UpsertReferences("a", []RefEntry{{TargetPath: "b"}}); err == nil {
		t.Fatal("关闭后的 UpsertReferences 应报错")
	}
	if err := d.BatchUpsertReferences(map[string][]RefEntry{"a": {}}); err == nil {
		t.Fatal("关闭后的 BatchUpsertReferences 应报错")
	}
}
