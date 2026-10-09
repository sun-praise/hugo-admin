package httpapi

import (
	"net/http"
	"strconv"

	"github.com/svtter/hugo-admin/internal/posts"
)

// intParam 解析整型查询参数；缺失或非法时用默认值。
func intParam(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// GET /api/posts?q=&category=&tag=&page=&per_page=
func (s *Server) handlePosts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	result := posts.GetPosts(s.cfg.ContentDir,
		q.Get("q"), q.Get("category"), q.Get("tag"),
		intParam(r, "page", 1), intParam(r, "per_page", 20))
	writeJSON(w, http.StatusOK, result)
}

// GET /api/posts/tags
func (s *Server) handlePostTags(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"tags": posts.GetAllTags(s.cfg.ContentDir)})
}

// GET /api/posts/categories
func (s *Server) handlePostCategories(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"categories": posts.GetAllCategories(s.cfg.ContentDir)})
}
