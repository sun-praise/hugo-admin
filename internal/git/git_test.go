package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo 构造测试仓库：2 个固定日期提交 + staged/unstaged/untracked
// 各一个文件 + bare 远程。commit hash 由固定 author/committer 日期决定，
// Python 契约测试与 Go 回放构造完全相同的序列。
func setupRepo(t *testing.T) (repoDir, bareDir string) {
	t.Helper()
	base := t.TempDir()
	repoDir = filepath.Join(base, "repo")
	bareDir = filepath.Join(base, "remote.git")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	run := func(dir string, env map[string]string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		for k, v := range env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustWrite := func(rel, content string) {
		t.Helper()
		path := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run(repoDir, nil, "init", "-b", "main")
	run(repoDir, nil, "config", "user.name", "契约测试")
	run(repoDir, nil, "config", "user.email", "contract@test.local")

	d1 := map[string]string{
		"GIT_AUTHOR_DATE":    "2026-01-01T12:00:00+08:00",
		"GIT_COMMITTER_DATE": "2026-01-01T12:00:00+08:00",
	}
	mustWrite("content/post/a.md", "a\n")
	run(repoDir, d1, "add", "-A")
	run(repoDir, d1, "commit", "-m", "init: 第一提交")

	d2 := map[string]string{
		"GIT_AUTHOR_DATE":    "2026-01-02T08:30:00+08:00",
		"GIT_COMMITTER_DATE": "2026-01-02T08:30:00+08:00",
	}
	mustWrite("content/post/b.md", "b\n")
	run(repoDir, d2, "add", "-A")
	run(repoDir, d2, "commit", "-m", "feat: 第二提交（含|竖线）")

	// 工作区状态：d.md staged、b.md 修改、c.md 未跟踪
	mustWrite("content/post/d.md", "d\n")
	run(repoDir, nil, "add", "content/post/d.md")
	mustWrite("content/post/b.md", "b changed\n")
	mustWrite("content/post/c.md", "c\n")

	run(base, nil, "init", "-b", "main", "--bare", bareDir)
	run(repoDir, nil, "remote", "add", "origin", bareDir)
	return repoDir, bareDir
}

func TestGetStatus(t *testing.T) {
	repoDir, _ := setupRepo(t)
	svc, err := New(repoDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetStatus()
	if st["success"] != true || st["has_changes"] != true {
		t.Fatalf("status = %#v", st)
	}
	if fmt.Sprint(st["staged"]) != "[content/post/d.md]" {
		t.Fatalf("staged = %#v", st["staged"])
	}
	if fmt.Sprint(st["unstaged"]) != "[content/post/b.md content/post/c.md]" {
		// Python 语义：untracked（"?? "）的 Y 位是 '?'，同样计入 unstaged
		t.Fatalf("unstaged = %#v", st["unstaged"])
	}
	if fmt.Sprint(st["untracked"]) != "[content/post/c.md]" {
		t.Fatalf("untracked = %#v", st["untracked"])
	}
}

func TestGetStatusNotRepo(t *testing.T) {
	svc, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	st := svc.GetStatus()
	if st["success"] != false || st["message"] != "当前目录不是有效的 git 仓库" {
		t.Fatalf("status = %#v", st)
	}
	// 非 repo 响应只有三个字段（对齐 Python）
	if len(st) != 3 {
		t.Fatalf("字段数 = %d: %#v", len(st), st)
	}
}

func TestGetRecentCommits(t *testing.T) {
	repoDir, _ := setupRepo(t)
	svc, _ := New(repoDir, nil)
	result := svc.GetRecentCommits(10)
	if result["success"] != true {
		t.Fatalf("result = %#v", result)
	}
	commits := result["commits"].([]CommitItem)
	if len(commits) != 2 {
		t.Fatalf("commits = %d", len(commits))
	}
	first := commits[0]
	if first.Author != "契约测试" || first.Email != "contract@test.local" {
		t.Fatalf("author = %#v", first)
	}
	if first.Message != "feat: 第二提交（含|竖线）" {
		t.Fatalf("message = %q", first.Message)
	}
	if !strings.Contains(first.Date, "2026-01-02") {
		t.Fatalf("date = %q", first.Date)
	}
	if first.Refs != "HEAD -> main" {
		t.Fatalf("refs = %q", first.Refs)
	}
	if first.Stats.Files != 1 || first.Stats.Insertions != 1 || first.Stats.Deletions != 0 {
		t.Fatalf("stats = %#v", first.Stats)
	}
}

func TestPush(t *testing.T) {
	repoDir, bareDir := setupRepo(t)
	svc, _ := New(repoDir, nil)

	ok, msg := svc.Push("origin", "", false)
	if !ok || msg != "推送成功到 origin/main" {
		t.Fatalf("push = %v %q", ok, msg)
	}
	// bare 已有 main
	out, err := exec.Command("git", "-C", bareDir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("bare HEAD: %v", err)
	}
	if len(strings.TrimSpace(string(out))) != 40 {
		t.Fatalf("bare HEAD = %q", out)
	}
}

func TestPublishSystem(t *testing.T) {
	repoDir, _ := setupRepo(t)
	svc, _ := New(repoDir, nil)

	result := svc.PublishSystem("release: 测试发布")
	if result["success"] != true {
		t.Fatalf("result = %#v", result)
	}
	if result["message"] != "系统发布成功，GitHub Actions 将自动构建站点" {
		t.Fatalf("message = %#v", result["message"])
	}
	// 无改动时拒绝
	result = svc.PublishSystem("again")
	if result["success"] != false || result["message"] != "没有需要发布的改动" {
		t.Fatalf("再次发布 = %#v", result)
	}
}

func TestCommitNothing(t *testing.T) {
	repoDir, _ := setupRepo(t)
	svc, _ := New(repoDir, nil)
	// 先推掉所有改动？直接在干净仓库上 commit
	exec.Command("git", "-C", repoDir, "add", "-A").Run()
	exec.Command("git", "-C", repoDir, "-c", "user.name=x", "-c", "user.email=x@x", "commit", "-m", "clean").Run()
	ok, msg := svc.Commit("empty")
	if ok || msg != "没有需要提交的改动" {
		t.Fatalf("commit = %v %q", ok, msg)
	}
}
