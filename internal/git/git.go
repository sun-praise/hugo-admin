// Package git 封装 Hugo 仓库的 git 操作：status/commits/push/publish_system。
// 行为逐条对齐 services/git_service.py（shell out 到 git 命令）。
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PushRecorder 记录推送历史（database 批次接入 sqlite 后实现）。
type PushRecorder interface {
	RecordPush(remote, branch, fromSHA, toSHA string, commitCount int, commitMessage, message string, success bool)
}

type Service struct {
	repoPath string
	db       PushRecorder // 可为 nil（no-op，对齐 Python database=None）
}

func New(repoPath string, db PushRecorder) (*Service, error) {
	st, err := os.Stat(repoPath)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("仓库路径不存在: %s", repoPath)
	}
	return &Service{repoPath: repoPath, db: db}, nil
}

// runGit 对齐 _run_git_command：check=false 时 CalledProcessError 不抛出。
func (s *Service) runGit(check bool, args ...string) (bool, string, string) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.repoPath
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	ok := err == nil
	if !ok && check {
		if _, isExit := err.(*exec.ExitError); !isExit {
			return false, stdout.String(), err.Error()
		}
	}
	return ok, stdout.String(), stderr.String()
}

// IsGitRepo 对齐 Python：仅认 .git 目录（worktree/submodule 的 .git 文件不算）。
func (s *Service) IsGitRepo() bool {
	st, err := os.Stat(filepath.Join(s.repoPath, ".git"))
	return err == nil && st.IsDir()
}

// GetStatus 对齐 get_status：porcelain 输出解析为 staged/unstaged/untracked。
func (s *Service) GetStatus() map[string]any {
	if !s.IsGitRepo() {
		return map[string]any{
			"success":     false,
			"has_changes": false,
			"message":     "当前目录不是有效的 git 仓库",
		}
	}
	ok, stdout, stderr := s.runGit(true, "status", "--porcelain")
	if !ok {
		return map[string]any{
			"success":     false,
			"has_changes": false,
			"message":     fmt.Sprintf("获取 git 状态失败: %s", stderr),
		}
	}

	staged, unstaged, untracked := []string{}, []string{}, []string{}
	// 不对整体输出 TrimSpace：porcelain 首行的 X 位可能是空格
	// （" M file"），整体 strip 会剥掉它导致首条目丢字符/误分类
	for _, line := range strings.Split(stdout, "\n") {
		if line == "" {
			continue
		}
		status := line[:2]
		filePath := strings.TrimSpace(line[3:])
		if status[0] != ' ' && status[0] != '?' {
			staged = append(staged, filePath)
		}
		if status[1] != ' ' {
			unstaged = append(unstaged, filePath)
		}
		if status == "??" {
			untracked = append(untracked, filePath)
		}
	}
	return map[string]any{
		"success":     true,
		"has_changes": len(staged) > 0 || len(unstaged) > 0 || len(untracked) > 0,
		"staged":      staged,
		"unstaged":    unstaged,
		"untracked":   untracked,
		"message":     "获取状态成功",
	}
}

// AddAll 对齐 add_all。
func (s *Service) AddAll() (bool, string) {
	ok, _, stderr := s.runGit(true, "add", "-A")
	if ok {
		return true, "已添加所有改动到暂存区"
	}
	return false, fmt.Sprintf("添加文件失败: %s", stderr)
}

// Commit 对齐 commit：nil 消息用时间戳默认值，"nothing to commit" 特判。
func (s *Service) Commit(message string) (bool, string) {
	if message == "" {
		message = "Update blog content - " + time.Now().Format("2006-01-02 15:04:05")
	}
	ok, stdout, stderr := s.runGit(true, "commit", "-m", message)
	if ok {
		return true, fmt.Sprintf("提交成功: %s", message)
	}
	if strings.Contains(stderr, "nothing to commit") || strings.Contains(stdout, "nothing to commit") {
		return false, "没有需要提交的改动"
	}
	return false, fmt.Sprintf("提交失败: %s", stderr)
}

func (s *Service) remoteHead(remote, branch string) string {
	ok, stdout, _ := s.runGit(false, "rev-parse", remote+"/"+branch)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stdout)
}

func (s *Service) headSubject() string {
	ok, stdout, _ := s.runGit(false, "log", "-1", "--pretty=format:%s")
	if !ok {
		return ""
	}
	return strings.TrimSpace(stdout)
}

func (s *Service) countCommits(fromSHA, toSHA string) int {
	if toSHA != "" && fromSHA != "" {
		ok, stdout, _ := s.runGit(false, "rev-list", "--count", fromSHA+".."+toSHA)
		if n, err := strconv.Atoi(strings.TrimSpace(stdout)); ok && err == nil {
			return n
		}
	}
	return 0
}

func (s *Service) recordPush(remote, branch, fromSHA, toSHA string, commitCount int, commitMessage, message string, success bool) {
	if s.db == nil {
		return
	}
	s.db.RecordPush(remote, branch, fromSHA, toSHA, commitCount, commitMessage, message, success)
}

// Push 对齐 push：branch 为空用当前分支，成败都记录推送历史（db 注入时）。
func (s *Service) Push(remote, branch string, setUpstream bool) (bool, string) {
	if branch == "" {
		ok, stdout, stderr := s.runGit(true, "branch", "--show-current")
		if !ok {
			return false, fmt.Sprintf("获取当前分支失败: %s", stderr)
		}
		branch = strings.TrimSpace(stdout)
	}
	fromSHA := s.remoteHead(remote, branch)
	commitMessage := s.headSubject()

	args := []string{"push"}
	if setUpstream {
		args = append(args, "-u", remote, branch)
	} else {
		args = append(args, remote, branch)
	}
	ok, _, stderr := s.runGit(true, args...)

	toSHA, commitCount := "", 0
	if ok {
		toSHA = s.remoteHead(remote, branch)
		commitCount = s.countCommits(fromSHA, toSHA)
		message := fmt.Sprintf("推送成功到 %s/%s", remote, branch)
		s.recordPush(remote, branch, fromSHA, toSHA, commitCount, commitMessage, message, true)
		return true, message
	}
	message := fmt.Sprintf("推送失败: %s", stderr)
	s.recordPush(remote, branch, fromSHA, toSHA, 0, commitMessage, message, false)
	return false, message
}

// PublishSystem 对齐 publish_system：check_status → add → commit → push。
func (s *Service) PublishSystem(commitMessage string) map[string]any {
	if !s.IsGitRepo() {
		return map[string]any{
			"success": false,
			"steps":   map[string]any{},
			"message": "当前目录不是有效的 git 仓库",
		}
	}
	steps := map[string]any{}

	status := s.GetStatus()
	steps["check_status"] = status
	if status["success"] != true {
		return map[string]any{"success": false, "steps": steps, "message": status["message"]}
	}
	if status["has_changes"] != true {
		return map[string]any{"success": false, "steps": steps, "message": "没有需要发布的改动"}
	}

	addOK, addMsg := s.AddAll()
	steps["add"] = map[string]any{"success": addOK, "message": addMsg}
	if !addOK {
		return map[string]any{"success": false, "steps": steps, "message": addMsg}
	}

	commitOK, commitMsg := s.Commit(commitMessage)
	steps["commit"] = map[string]any{"success": commitOK, "message": commitMsg}
	if !commitOK {
		return map[string]any{"success": false, "steps": steps, "message": commitMsg}
	}

	pushOK, pushMsg := s.Push("origin", "", false)
	steps["push"] = map[string]any{"success": pushOK, "message": pushMsg}
	if !pushOK {
		return map[string]any{
			"success": false, "steps": steps,
			"message": fmt.Sprintf("推送失败: %s", pushMsg),
		}
	}
	return map[string]any{
		"success": true, "steps": steps,
		"message": "系统发布成功，GitHub Actions 将自动构建站点",
	}
}

// CommitItem 是 get_recent_commits 的单条记录。
type CommitItem struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Email   string `json:"email"`
	Date    string `json:"date"`
	Refs    string `json:"refs"`
	Message string `json:"message"`
	Stats   struct {
		Files      int `json:"files"`
		Insertions int `json:"insertions"`
		Deletions  int `json:"deletions"`
	} `json:"stats"`
}

// GetRecentCommits 对齐 get_recent_commits：单次 git log（%H|%an|%ae|%ad|%d|%s
// + --numstat）解析，count 钳制 [1,50]。
func (s *Service) GetRecentCommits(count int) map[string]any {
	if count < 1 {
		count = 10
	}
	if count > 50 {
		count = 50
	}
	if !s.IsGitRepo() {
		return map[string]any{
			"success": false,
			"commits": []CommitItem{},
			"message": "当前目录不是有效的 git 仓库",
		}
	}
	ok, stdout, stderr := s.runGit(true, "log",
		fmt.Sprintf("-%d", count),
		"--pretty=format:%H|%an|%ae|%ad|%d|%s",
		"--date=iso",
		"--numstat",
	)
	if !ok {
		return map[string]any{
			"success": false,
			"commits": []CommitItem{},
			"message": fmt.Sprintf("获取提交记录失败: %s", stderr),
		}
	}

	commits := []CommitItem{}
	var current *CommitItem
	for _, line := range strings.Split(stdout, "\n") {
		stripped := strings.TrimSpace(line)

		// numstat 行: <ins>\t<del>\t<path>
		if current != nil && strings.Contains(stripped, "\t") && !strings.HasSuffix(stripped, "|") {
			parts := strings.Split(stripped, "\t")
			if len(parts) >= 3 {
				current.Stats.Files++
				if n, err := strconv.Atoi(strings.TrimLeft(parts[0], "-")); err == nil {
					current.Stats.Insertions += n
				}
				if n, err := strconv.Atoi(strings.TrimLeft(parts[1], "-")); err == nil {
					current.Stats.Deletions += n
				}
				continue
			}
		}

		// 提交行（message 可能含 |，只切前 5 个）
		if strings.Contains(stripped, "|") && !strings.HasPrefix(stripped, "\t") {
			fields := strings.SplitN(stripped, "|", 6)
			if len(fields) >= 6 {
				if current != nil {
					commits = append(commits, *current)
				}
				refs := strings.Trim(strings.TrimSpace(fields[4]), "()")
				current = &CommitItem{
					Hash:    fields[0],
					Author:  fields[1],
					Email:   fields[2],
					Date:    fields[3],
					Refs:    refs,
					Message: fields[5],
				}
				continue
			}
		}
	}
	if current != nil {
		commits = append(commits, *current)
	}
	return map[string]any{
		"success": true,
		"commits": commits,
		"message": fmt.Sprintf("成功获取 %d 条提交记录", len(commits)),
	}
}
