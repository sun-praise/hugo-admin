package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/svtter/hugo-admin/internal/posts"
)

// 对齐 publish_routes.py 的 article 端点与 server_routes.py 的
// Hugo 服务器管理端点。

// GET /api/article/status?file_path=
func (s *Server) handleArticleStatus(w http.ResponseWriter, r *http.Request) {
	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "缺少 file_path 参数", "error_code": "MISSING_PARAMETER",
		})
		return
	}
	status := posts.GetPublishStatus(s.cfg.ContentDir, filePath)
	if errMsg, hasErr := status["error"]; hasErr {
		code := http.StatusBadRequest
		if msg, _ := errMsg.(string); strings.Contains(msg, "不存在") {
			code = http.StatusNotFound
		}
		writeJSON(w, code, map[string]any{
			"success": false, "error": errMsg, "error_code": "STATUS_CHECK_FAILED",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
}

// POST /api/article/status/bulk {file_paths}
func (s *Server) handleArticleStatusBulk(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	paths, _ := data["file_paths"].([]any)
	if _, has := data["file_paths"]; !has {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "缺少 file_paths 参数", "error_code": "MISSING_PARAMETER",
		})
		return
	}
	results := make([]map[string]any, 0, len(paths))
	for _, p := range paths {
		filePath, _ := p.(string)
		results = append(results, map[string]any{
			"file_path": filePath,
			"status":    posts.GetPublishStatus(s.cfg.ContentDir, filePath),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "results": results, "count": len(results),
	})
}

// POST /api/article/publish {file_path}
func (s *Server) handleArticlePublish(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	filePath, has := data["file_path"].(string)
	if !has {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "缺少 file_path 参数", "error_code": "MISSING_PARAMETER",
		})
		return
	}
	ok, message, operationID := posts.PublishArticle(s.cfg.ContentDir, filePath)
	if ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":              true,
			"message":              message,
			"operation_id":         operationID,
			"article_path":         filePath,
			"draft_status_changed": true,
			"published_at":         time.Now().Format("2006-01-02T15:04:05.999999") + "Z",
		})
		return
	}
	code := http.StatusBadRequest
	switch {
	case strings.Contains(message, "不存在"):
		code = http.StatusNotFound
	case strings.Contains(message, "已经发布") || strings.Contains(message, "访问被拒绝"):
		code = http.StatusConflict
	}
	writeJSON(w, code, map[string]any{
		"success": false, "error": message, "error_code": "PUBLISH_FAILED",
	})
}

// POST /api/article/publish/bulk {file_paths}
func (s *Server) handleArticlePublishBulk(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	pathsAny, has := data["file_paths"].([]any)
	if !has {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "error": "缺少 file_paths 参数", "error_code": "MISSING_PARAMETER",
		})
		return
	}
	paths := make([]string, 0, len(pathsAny))
	for _, p := range pathsAny {
		if str, ok := p.(string); ok {
			paths = append(paths, str)
		}
	}
	result := posts.BulkPublishArticles(s.cfg.ContentDir, paths)

	failed := result["failed_count"].(int)
	published := result["published_count"].(int)
	status := http.StatusOK
	switch {
	case result["success"] == true || failed == 0:
	case failed > 0 && published > 0:
		status = http.StatusMultiStatus
	default:
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}

// GET /api/server/status
func (s *Server) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.hugo.Status())
}

// POST /api/server/start {debug?}
func (s *Server) handleServerStart(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	debug, _ := data["debug"].(bool)
	ok, message := s.hugo.Start(debug, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"success": ok, "message": message, "status": s.hugo.Status(),
	})
}

// POST /api/server/stop
func (s *Server) handleServerStop(w http.ResponseWriter, r *http.Request) {
	ok, message := s.hugo.Stop()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": ok, "message": message, "status": s.hugo.Status(),
	})
}

// GET /api/server/logs?count=
// Python 侧经 socket request_logs 拉历史日志；Go 侧提供等价 REST，
// 前端 SSE 改造批次统一切换。
func (s *Server) handleServerLogs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"logs": s.hugo.RecentLogs(intParam(r, "count", 100))})
}
