package httpapi

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/svtter/hugo-admin/internal/projectinit"
)

// 对齐 project_init_routes.py 与 post_routes.py 的缓存端点。

// POST /api/project/init {path, config_format}
func (s *Server) handleProjectInit(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	path, _ := data["path"].(string)
	configFormat, _ := data["config_format"].(string)
	if configFormat == "" {
		configFormat = "toml"
	}

	if strings.TrimSpace(path) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "目标路径不能为空"})
		return
	}
	if configFormat != "toml" && configFormat != "yaml" {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "配置文件格式仅支持 toml 或 yaml"})
		return
	}

	result, err := projectinit.CreateSite(s.cfg.HugoRoot, path, configFormat)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": err.Error()})
		return
	}

	// 记录活跃项目（重启后恢复；Go 版当前进程需重启生效——与 settings
	// 的 base_dir 切换语义一致）
	activeFile := filepath.Join(filepath.Dir(s.cfg.AuthStorePath), "active_project.txt")
	if dir := filepath.Dir(activeFile); dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	_ = os.WriteFile(activeFile, []byte(result.Path+"\n"), 0o644)

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"message":       "Hugo 站点已创建并设为活跃项目（重启服务后完全生效）",
		"path":          result.Path,
		"config_format": result.ConfigFormat,
		"default_theme": result.DefaultTheme,
	})
}

// GET /api/project/active
func (s *Server) handleProjectActive(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "path": s.cfg.HugoRoot,
	})
}

// POST /api/project/active/reset
func (s *Server) handleProjectActiveReset(w http.ResponseWriter, r *http.Request) {
	activeFile := filepath.Join(filepath.Dir(s.cfg.AuthStorePath), "active_project.txt")
	_ = os.Remove(activeFile)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "已清除持久化记录"})
}

// POST /api/project/clean-layouts
func (s *Server) handleProjectCleanLayouts(w http.ResponseWriter, r *http.Request) {
	hugoRoot := s.cfg.HugoRoot
	if st, err := os.Stat(hugoRoot); err != nil || !st.IsDir() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "活跃项目路径无效"})
		return
	}
	themesDir := filepath.Join(hugoRoot, "themes")
	entries, err := os.ReadDir(themesDir)
	if err != nil || len(entries) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "未检测到任何已安装主题，保留占位 layouts"})
		return
	}
	projectinit.RemoveDefaultLayouts(hugoRoot)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("已清理占位 layouts，主题接管渲染: %s", hugoRoot),
	})
}

// ============ 缓存端点（Go 直扫语义） ============

// cacheStats 返回直扫统计（对齐 get_stats 的字段形状；Go 无缓存层，
// total 为实时扫描数）。
func (s *Server) cacheStats() map[string]any {
	posts := countMarkdown(s.cfg.ContentDir)
	return map[string]any{
		"total_posts": posts,
		"cache_type":  "direct-scan",
	}
}

func countMarkdown(dir string) int {
	count := 0
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ".md") {
			count++
		}
		return nil
	})
	return count
}

// POST /api/cache/refresh —— Go 直扫无缓存，刷新即重建引用索引
func (s *Server) handleCacheRefresh(w http.ResponseWriter, r *http.Request) {
	if s.refsSvc != nil {
		s.refsSvc.ScanAll()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "缓存刷新成功", "stats": s.cacheStats(),
	})
}

// GET /api/cache/stats
func (s *Server) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "stats": s.cacheStats(),
	})
}
