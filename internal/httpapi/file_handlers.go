package httpapi

import (
	"net/http"

	"github.com/svtter/hugo-admin/internal/posts"
)

// 对齐 routes/file_routes.py 的文件读写端点。
// /api/article/import 依赖 AI 服务，待 AI 批次迁移。

// POST /api/file/read {path}
func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	filePath, _ := data["path"].(string)
	if filePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文件路径",
		})
		return
	}
	ok, content, mtime := posts.ReadFile(s.cfg.ContentDir, filePath)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": content,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "content": content, "path": filePath, "mtime": mtime,
	})
}

// POST /api/file/read-with-frontmatter {path}
func (s *Server) handleFileReadWithFM(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	filePath, _ := data["path"].(string)
	if filePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文件路径",
		})
		return
	}
	ok, content, fm, mtime := posts.ReadFileWithFrontmatter(s.cfg.ContentDir, filePath)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": content,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "content": content, "frontmatter": fm,
		"path": filePath, "mtime": mtime,
	})
}

// POST /api/file/save {path, content, frontmatter?, expected_mtime?, force?}
func (s *Server) handleFileSave(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	filePath, _ := data["path"].(string)
	content, hasContent := data["content"].(string)
	fmData, _ := data["frontmatter"].(map[string]any)

	if filePath == "" || !hasContent {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少必要参数",
		})
		return
	}

	force, _ := data["force"].(bool)
	var expected *float64
	if !force {
		if v, ok := data["expected_mtime"].(float64); ok {
			expected = &v
		}
	}

	ok, message, newMtime := posts.SaveFile(s.cfg.ContentDir, filePath, content, fmData, expected)
	if !ok {
		if conflict, isConflict := message.(posts.ConflictInfo); isConflict {
			writeJSON(w, http.StatusConflict, map[string]any{
				"success":         false,
				"conflict":        conflict.Conflict,
				"current_content": conflict.CurrentContent,
				"current_mtime":   conflict.CurrentMTime,
				"message":         conflict.Message,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": message,
		})
		return
	}
	resp := map[string]any{
		"success": true, "message": message,
	}
	if ok && newMtime != 0 {
		resp["mtime"] = newMtime
	}
	writeJSON(w, http.StatusOK, resp)
}

// POST /api/post/create {title}
func (s *Server) handlePostCreate(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	title, _ := data["title"].(string)
	if title == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章标题",
		})
		return
	}
	ok, result := posts.CreatePost(s.cfg.ContentDir, title)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "path": result, "message": "文章创建成功",
	})
}
