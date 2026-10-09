package httpapi

import (
	"net/http"

	"github.com/svtter/hugo-admin/internal/git"
)

// 对齐 routes/publish_routes.py 的 git 端点。
// /api/git/pushes 依赖推送历史数据库，待 sqlite 批次迁移。

// gitOrUnavailable 统一 nil 保护（仅测试环境会出现未注入）。
func (s *Server) gitOrUnavailable(w http.ResponseWriter) *git.Service {
	if s.gitSvc == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "Git 服务未初始化",
		})
		return nil
	}
	return s.gitSvc
}

// GET /api/git/pushes?page=&per_page=
// 对齐 publish_routes.git_pushes：分页查询推送历史。
func (s *Server) handleGitPushes(w http.ResponseWriter, r *http.Request) {
	if s.database == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "推送历史不可用：数据库未初始化",
		})
		return
	}
	page := intParam(r, "page", 1)
	perPage := intParam(r, "per_page", 20)
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	if perPage > 100 {
		perPage = 100
	}

	pushes, total, err := s.database.ListPushes(perPage, (page-1)*perPage)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "获取推送历史失败: " + err.Error(),
		})
		return
	}
	totalPages := 1
	if perPage > 0 {
		totalPages = (total + perPage - 1) / perPage
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"pushes":      pushes,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// GET /api/git/status
func (s *Server) handleGitStatus(w http.ResponseWriter, r *http.Request) {
	if svc := s.gitOrUnavailable(w); svc != nil {
		writeJSON(w, http.StatusOK, svc.GetStatus())
	}
}

// GET /api/git/commits?count=
func (s *Server) handleGitCommits(w http.ResponseWriter, r *http.Request) {
	if svc := s.gitOrUnavailable(w); svc != nil {
		writeJSON(w, http.StatusOK, svc.GetRecentCommits(intParam(r, "count", 10)))
	}
}

// POST /api/git/push {remote?, branch?, set_upstream?}
func (s *Server) handleGitPush(w http.ResponseWriter, r *http.Request) {
	svc := s.gitOrUnavailable(w)
	if svc == nil {
		return
	}
	data := jsonDict(r)
	remote, _ := data["remote"].(string)
	if remote == "" {
		remote = "origin"
	}
	branch, _ := data["branch"].(string)
	setUpstream, _ := data["set_upstream"].(bool)

	if !svc.IsGitRepo() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false,
			"message": "当前目录不是有效的 git 仓库",
			"remote":  remote,
			"branch":  branch,
		})
		return
	}
	ok, message := svc.Push(remote, branch, setUpstream)
	status := http.StatusOK
	if !ok {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]any{
		"success": ok,
		"message": message,
		"remote":  remote,
		"branch":  branch,
	})
}

// POST /api/publish/system {message?}
func (s *Server) handlePublishSystem(w http.ResponseWriter, r *http.Request) {
	svc := s.gitOrUnavailable(w)
	if svc == nil {
		return
	}
	data := jsonDict(r)
	message, _ := data["message"].(string)

	result := svc.PublishSystem(message)
	status := http.StatusOK
	if result["success"] != true {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}
