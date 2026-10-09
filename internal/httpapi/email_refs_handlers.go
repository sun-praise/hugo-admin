package httpapi

import (
	"net/http"
	"os"

	"github.com/svtter/hugo-admin/internal/email"
)

// 对齐 email_routes.py 与 references_routes.py。

// newEmailService 从 settings 构造（对齐 _create_email_service）。
func (s *Server) newEmailService(debugMode bool) *email.Service {
	listmonk := map[string]any{}
	if s.settingsSvc != nil {
		if st, err := s.settingsSvc.GetSettings(); err == nil {
			listmonk = st.Listmonk
		}
	}
	home, _ := os.UserHomeDir()
	return email.New(listmonk, debugMode, home)
}

// POST /api/email/push-latest
func (s *Server) handleEmailPushLatest(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	debugMode, _ := data["debug_mode"].(bool)
	force, _ := data["force"].(bool)
	result := s.newEmailService(debugMode).PushLatest(force)
	writeJSON(w, statusOf(result), result)
}

// POST /api/email/push-article
func (s *Server) handleEmailPushArticle(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	urlParam, _ := data["url"].(string)
	debugMode, _ := data["debug_mode"].(bool)
	force, _ := data["force"].(bool)
	if urlParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章 URL 参数"})
		return
	}
	result := s.newEmailService(debugMode).PushArticle(urlParam, force)
	writeJSON(w, statusOf(result), result)
}

// GET /api/email/preview-latest
func (s *Server) handleEmailPreviewLatest(w http.ResponseWriter, r *http.Request) {
	result := s.newEmailService(false).PreviewLatest()
	writeJSON(w, statusOf(result), result)
}

// GET /api/email/preview-article?url=
func (s *Server) handleEmailPreviewArticle(w http.ResponseWriter, r *http.Request) {
	urlParam := r.URL.Query().Get("url")
	if urlParam == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章 URL 参数"})
		return
	}
	result := s.newEmailService(false).PreviewArticle(urlParam)
	writeJSON(w, statusOf(result), result)
}

func statusOf(result map[string]any) int {
	if result["success"] == true {
		return http.StatusOK
	}
	return http.StatusBadRequest
}

// ============ references ============

// POST /api/references/scan
func (s *Server) handleRefsScan(w http.ResponseWriter, r *http.Request) {
	s.refsSvc.ScanAll()
	allRefs, err := s.database.GetAllReferences()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "references": allRefs,
	})
}

// GET /api/references/backlinks?path=
func (s *Server) handleRefsBacklinks(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少 path 参数"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "backlinks": s.refsSvc.GetBacklinks(path),
	})
}

// GET /api/posts/search?q=
func (s *Server) handlePostsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "posts": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "posts": s.refsSvc.SearchPosts(q),
	})
}
