package httpapi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sun-praise/hugo-admin/v3/internal/aigen"
	"github.com/sun-praise/hugo-admin/v3/internal/importsvc"
	"github.com/sun-praise/hugo-admin/v3/internal/util"
)

// 对齐 ai_routes.fm_bp 的 /api/frontmatter/generate、image_routes 的
// /api/image/generate-cover、file_routes 的 /api/article/import。

// aiConfigFromSettings 从 settings + env 拼出 AI 配置（对齐各路由的
// current_app.config 读取语义：session key 优先于 env）。
func (s *Server) aiConfig() (apiKey, baseURL, model string) {
	apiKey = s.sessionAPIKey
	if apiKey == "" {
		apiKey = s.envAPIKey
	}
	baseURL = "https://api.deepseek.com"
	model = "deepseek-chat"
	if s.settingsSvc != nil {
		if st, err := s.settingsSvc.GetSettings(); err == nil {
			if v, ok := st.AI["base_url"].(string); ok && v != "" {
				baseURL = v
			}
			if v, ok := st.AI["model"].(string); ok && v != "" {
				model = v
			}
		}
	}
	return
}

// POST /api/frontmatter/generate {content}
func (s *Server) handleFMGenerate(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	content, _ := data["content"].(string)
	if strings.TrimSpace(content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "文章内容为空"})
		return
	}
	apiKey, baseURL, model := s.aiConfig()
	if apiKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "AI API Key 未配置"})
		return
	}
	ok, result := aigen.GenerateFrontmatter(content, apiKey, baseURL, model, time.Now())
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": fmt.Sprintf("%v", result)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "frontmatter": result,
	})
}

// POST /api/image/generate-cover {article_path, title, description, content}
func (s *Server) handleImageGenCover(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	articlePath, _ := data["article_path"].(string)
	if articlePath == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文章路径"})
		return
	}
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("IMAGE_GEN_MODEL")
	if apiKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "OPENROUTER_API_KEY 未配置，请设置环境变量"})
		return
	}

	title, _ := data["title"].(string)
	description, _ := data["description"].(string)
	content, _ := data["content"].(string)

	ok, result := aigen.GenerateCoverImage(title, description, content, apiKey, model)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": fmt.Sprintf("%v", result)})
		return
	}
	imageBytes, _ := result.([]byte)

	saveOK, saveResult := aigen.SaveGeneratedImage(articlePath, imageBytes, s.cfg.ContentDir, time.Now())
	if !saveOK {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": saveResult})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "url": saveResult, "message": "封面图片生成成功",
	})
}

// formBool 对齐 file_routes._form_bool。
func formBool(v string, def bool) bool {
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return false
	case "1", "true", "yes", "on":
		return true
	}
	return def
}

// POST /api/article/import（multipart: file + title + 开关）
// Content-Type: application/json 时走 JSON 模式（机器客户端友好）：
// {filename?, title?, content, generate_frontmatter?=false,
//
//	generate_cover?=false, draft?=true}——AI 开关默认关闭、可显式
//
// 指定草稿位，语义对 agent 客户端可预测。
func (s *Server) handleArticleImport(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		s.handleArticleImportJSON(w, r)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "缺少文件"})
		return
	}
	defer file.Close()
	filename := header.Filename
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "文件名为空"})
		return
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".md" && ext != ".markdown" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("不支持的文件类型: %s", strings.TrimPrefix(ext, "."))})
		return
	}

	raw, err := io.ReadAll(io.LimitReader(file, 16<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "读取文件失败"})
		return
	}

	apiKey, baseURL, model := s.aiConfig()
	imgCfg := importsvc.ImageCfg{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  os.Getenv("IMAGE_GEN_MODEL"),
	}
	eventScope := util.NewOperationID()

	result := importsvc.Import(s.cfg.ContentDir, filename, raw,
		r.FormValue("title"),
		formBool(r.FormValue("generate_frontmatter"), true),
		formBool(r.FormValue("generate_cover"), true),
		true,
		importsvc.AICfg{APIKey: apiKey, BaseURL: baseURL, Model: model},
		imgCfg, s.broker, eventScope, time.Now())

	if result.Path == "" {
		msg := strings.Join(result.Warnings, "; ")
		if msg == "" {
			msg = "导入失败"
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": msg})
		return
	}
	resp := map[string]any{
		"success":       true,
		"path":          result.Path,
		"title":         result.Title,
		"warnings":      result.Warnings,
		"cover_pending": result.CoverPending,
		"event_scope":   result.EventScope,
	}
	writeJSON(w, http.StatusOK, resp)
}

// jsonBool 取 JSON 字段布尔值，缺省/类型不符时用 def。
func jsonBool(data map[string]any, key string, def bool) bool {
	if v, ok := data[key].(bool); ok {
		return v
	}
	return def
}

// handleArticleImportJSON 是 /api/article/import 的 JSON 模式，供机器
// 客户端（agent 等）免 multipart、免文件系统直接投递 Markdown。
// 与 multipart UI 模式的差异：AI 开关默认关闭（可预测、无外部依赖），
// draft 可显式指定（默认 true 草稿）。路径派生与唯一目录语义与
// multipart 模式一致，永不覆盖既有文章；更新请走 /api/file/save。
func (s *Server) handleArticleImportJSON(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	content, _ := data["content"].(string)
	if strings.TrimSpace(content) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "content 为空"})
		return
	}
	filename, _ := data["filename"].(string)
	if strings.TrimSpace(filename) == "" {
		filename = "import.md"
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != ".md" && ext != ".markdown" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("不支持的文件类型: %s", strings.TrimPrefix(ext, "."))})
		return
	}
	title, _ := data["title"].(string)

	apiKey, baseURL, model := s.aiConfig()
	imgCfg := importsvc.ImageCfg{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  os.Getenv("IMAGE_GEN_MODEL"),
	}

	result := importsvc.Import(s.cfg.ContentDir, filename, []byte(content), title,
		jsonBool(data, "generate_frontmatter", false),
		jsonBool(data, "generate_cover", false),
		jsonBool(data, "draft", true),
		importsvc.AICfg{APIKey: apiKey, BaseURL: baseURL, Model: model},
		imgCfg, s.broker, util.NewOperationID(), time.Now())

	if result.Path == "" {
		msg := strings.Join(result.Warnings, "; ")
		if msg == "" {
			msg = "导入失败"
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"path":          result.Path,
		"title":         result.Title,
		"warnings":      result.Warnings,
		"cover_pending": result.CoverPending,
		"event_scope":   result.EventScope,
	})
}
