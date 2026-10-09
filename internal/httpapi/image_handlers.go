package httpapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/svtter/hugo-admin/internal/posts"
)

// 对齐 routes/image_routes.py 的上传与列表。
// /api/image/generate-cover 依赖 OpenRouter 图像生成，待后续批次。
// 插件上传（gRPC image_upload 能力）待插件系统批次；当前直接本地保存。

// route 层扩展名白名单（与服务层不同：这里含 svg）
var routeImageExtensions = map[string]bool{
	"png": true, "jpg": true, "jpeg": true, "gif": true, "svg": true, "webp": true,
}

// POST /api/image/upload（multipart: file + article_path）
func (s *Server) handleImageUpload(w http.ResponseWriter, r *http.Request) {
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "没有文件",
		})
		return
	}
	defer file.Close()
	articlePath := r.FormValue("article_path")
	if articlePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章路径",
		})
		return
	}
	filename := ""
	if header != nil {
		filename = header.Filename
	}
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "文件名为空",
		})
		return
	}

	ext := ""
	if idx := strings.LastIndex(filename, "."); idx >= 0 {
		ext = strings.ToLower(filename[idx+1:])
	}
	if !routeImageExtensions[ext] {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "不支持的文件类型: " + ext,
		})
		return
	}

	// 插件上传优先（对齐 _try_plugin_upload：R2/CDN 等能力插件）
	if s.pluginMgr != nil {
		if target := s.pluginMgr.FindPluginWithCapability("image_upload"); target != nil {
			name, _ := target["name"].(string)
			if _, seekErr := file.Seek(0, 0); seekErr == nil {
				if uploaded := s.uploadViaPlugin(w, r, name, file, header, articlePath); uploaded {
					return
				}
			}
			// 插件上传失败 → 回退本地保存（对齐 Python fallback 语义）
		}
	}

	data, err := io.ReadAll(io.LimitReader(file, posts.MaxImageSize+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": "保存图片失败: " + err.Error(),
		})
		return
	}

	ok, result := posts.SaveImage(s.cfg.ContentDir, articlePath, filename, data)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "url": result, "message": "图片上传成功",
	})
}

// POST /api/image/list {article_path}
func (s *Server) handleImageList(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	articlePath, _ := data["article_path"].(string)
	if articlePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章路径",
		})
		return
	}
	ok, result := posts.ListImages(s.cfg.ContentDir, articlePath)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": result,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "images": result,
	})
}
