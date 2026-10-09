package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/svtter/hugo-admin/internal/settings"
	"github.com/svtter/hugo-admin/internal/theme"
)

// 对齐 settings_routes / config_routes / theme_routes / page_routes(content)。

// ensureServerURLHasPort 对齐 _ensure_server_url_has_port。
func ensureServerURLHasPort(rawURL string) string {
	if rawURL == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || u.Port() != "" {
		return rawURL
	}
	u.Host = fmt.Sprintf("%s:1313", u.Hostname())
	return u.String()
}

// toPublicSettings 对齐 _to_public_settings（含 api_key 来源）。
func (s *Server) toPublicSettings(st *settings.Settings) map[string]any {
	public := s.settingsSvc.ToPublicSettings(st)
	hugoPublic := public["hugo"].(map[string]any)
	if hugoPublic["base_dir"] == "" {
		hugoPublic["base_dir"] = s.cfg.HugoRoot
	}
	if hugoPublic["server_url"] == "" {
		hugoPublic["server_url"] = "http://0.0.0.0:1313"
	}
	hugoPublic["server_url"] = ensureServerURLHasPort(hugoPublic["server_url"].(string))

	aiPublic := public["ai"].(map[string]any)
	switch {
	case s.sessionAPIKey != "":
		aiPublic["api_key_source"] = "session"
		aiPublic["api_key_configured"] = true
		aiPublic["api_key_hint"] = settings.MaskAPIKey(s.sessionAPIKey)
	case s.envAPIKey != "":
		aiPublic["api_key_source"] = "env"
		aiPublic["api_key_configured"] = true
		aiPublic["api_key_hint"] = ""
	default:
		aiPublic["api_key_source"] = "none"
		aiPublic["api_key_configured"] = false
		aiPublic["api_key_hint"] = ""
	}
	return public
}

// GET /api/settings
func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	st, err := s.settingsSvc.GetSettings()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "settings": s.toPublicSettings(st),
	})
}

// PUT /api/settings —— 含 ai.api_key 剥离到 session、hugo 切换目录提示
func (s *Server) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := decodeJSONStrict(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": "请求体必须是 JSON 对象",
		})
		return
	}
	payload, _ := body["settings"].(map[string]any)
	if payload == nil {
		payload = body
	}

	// ai.api_key → session，不落盘
	settingsPayload := payload
	if ai, ok := payload["ai"].(map[string]any); ok {
		if apiKey, exists := ai["api_key"]; exists {
			key, _ := apiKey.(string)
			key = strings.TrimSpace(key)
			copied := copyMap(payload)
			aiCopy := copyMap(ai)
			delete(aiCopy, "api_key")
			copied["ai"] = aiCopy
			settingsPayload = copied
			if key != "" || exists {
				s.sessionAPIKey = key
			}
		}
	}

	updated, err := s.settingsSvc.UpdateSettings(settingsPayload)
	if err != nil {
		status := http.StatusInternalServerError
		if _, isValidation := err.(*settings.ValidationError); isValidation {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{
			"success": false, "message": err.Error(),
		})
		return
	}

	// hugo server_url 更新到 hugo manager
	if hugo, ok := updated.Hugo["server_url"].(string); ok {
		newURL := ensureServerURLHasPort(hugo)
		if newURL != s.hugo.ServerURL() {
			s.hugo.SetServerURL(newURL)
		}
	}
	// base_dir 切换：Go 版当前进程不热切换服务实例（需重启生效），
	// 提示信息由前端展示
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "设置已保存",
		"settings": s.toPublicSettings(updated),
	})
}

// ============ Hugo 配置文件读写（/api/config） ============

var rootConfigCandidates = []string{"hugo.toml", "hugo.yaml", "config.toml", "config.yaml", "config.json"}
var dirConfigCandidates = []string{"config/_default/config.toml", "config/_default/config.yaml", "config/_default/config.json"}

var extFormat = map[string]string{
	".toml": "toml", ".yaml": "yaml", ".yml": "yaml", ".json": "json",
}

func detectConfigMode(hugoRoot string) string {
	for _, name := range rootConfigCandidates {
		if isFile(filepath.Join(hugoRoot, name)) {
			return "root"
		}
	}
	configDir := filepath.Join(hugoRoot, "config", "_default")
	entries, err := os.ReadDir(configDir)
	if err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".toml") {
				return "dir"
			}
		}
	}
	return "none"
}

// GET /api/config
func (s *Server) handleConfigList(w http.ResponseWriter, r *http.Request) {
	hugoRoot := s.cfg.HugoRoot
	if !isDir(hugoRoot) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Hugo 项目路径无效"})
		return
	}
	mode := detectConfigMode(hugoRoot)
	files := []map[string]any{}
	if mode == "root" {
		for _, name := range rootConfigCandidates {
			p := filepath.Join(hugoRoot, name)
			if isFile(p) {
				ext := strings.ToLower(filepath.Ext(p))
				files = append(files, map[string]any{
					"name": name, "path": p, "format": extFormat[ext],
				})
				break
			}
		}
	} else if mode == "dir" {
		configDir := filepath.Join(hugoRoot, "config", "_default")
		entries, _ := os.ReadDir(configDir)
		for _, e := range entries {
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if _, ok := extFormat[ext]; ok && !e.IsDir() {
				files = append(files, map[string]any{
					"name": e.Name(), "path": filepath.Join(configDir, e.Name()), "format": extFormat[ext],
				})
			}
		}
	}
	if len(files) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"success": false, "message": "未找到 Hugo 配置文件"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "mode": mode, "files": files,
	})
}

// resolveConfigPath 对齐 _resolve_config_path（防穿越）。
func resolveConfigPath(hugoRoot, filename string) string {
	rootFile := filepath.Join(hugoRoot, filename)
	if isFile(rootFile) {
		return rootFile
	}
	configDir := filepath.Join(hugoRoot, "config", "_default")
	dirFile := filepath.Join(configDir, filename)
	if isSafeWithin(dirFile, configDir) && isFile(dirFile) {
		return dirFile
	}
	return ""
}

func isSafeWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func validateConfigContent(content, format string) string {
	switch format {
	case "toml":
		var v map[string]any
		if err := toml.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Sprintf("TOML 语法错误: %v", err)
		}
	case "yaml", "yml":
		var v any
		if err := yaml.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Sprintf("YAML 语法错误: %v", err)
		}
	case "json":
		var v any
		if err := decodeStrict(content, &v); err != nil {
			return fmt.Sprintf("JSON 语法错误: %v", err)
		}
	default:
		return fmt.Sprintf("不支持的配置格式: %s", format)
	}
	return ""
}

// GET /api/config/{filename}
func (s *Server) handleConfigGet(w http.ResponseWriter, r *http.Request) {
	hugoRoot := s.cfg.HugoRoot
	if !isDir(hugoRoot) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Hugo 项目路径无效"})
		return
	}
	filename := r.PathValue("filename")
	configPath := resolveConfigPath(hugoRoot, filename)
	if configPath == "" {
		writeJSON(w, http.StatusNotFound, map[string]any{
			"success": false, "message": fmt.Sprintf("配置文件不存在: %s", filename)})
		return
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "filename": filepath.Base(configPath),
		"format":  extFormat[strings.ToLower(filepath.Ext(configPath))],
		"content": string(content), "path": configPath,
	})
}

// PUT /api/config/{filename}
func (s *Server) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	hugoRoot := s.cfg.HugoRoot
	if !isDir(hugoRoot) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "Hugo 项目路径无效"})
		return
	}
	data := jsonDict(r)
	content, _ := data["content"].(string)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "配置内容不能为空"})
		return
	}
	filename := r.PathValue("filename")
	configPath := resolveConfigPath(hugoRoot, filename)
	if configPath == "" {
		if detectConfigMode(hugoRoot) == "dir" {
			configPath = filepath.Join(hugoRoot, "config", "_default", filename)
		} else {
			configPath = filepath.Join(hugoRoot, filename)
		}
	}
	format := extFormat[strings.ToLower(filepath.Ext(configPath))]
	if errMsg := validateConfigContent(content, format); errMsg != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": errMsg})
		return
	}
	_ = os.MkdirAll(filepath.Dir(configPath), 0o755)
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": fmt.Sprintf("配置已保存: %s", filepath.Base(configPath)),
		"path":    configPath,
	})
}

// ============ 主题（/api/themes） ============

func (s *Server) themeService() *theme.Service {
	return theme.NewService(s.cfg.HugoRoot, s.setActiveTheme, nil)
}

func (s *Server) setActiveTheme(name string) error {
	_, err := s.settingsSvc.UpdateSettings(map[string]any{
		"theme": map[string]any{"name": name},
	})
	return err
}

// GET /api/themes
func (s *Server) handleThemeList(w http.ResponseWriter, r *http.Request) {
	svc := s.themeService()
	themes, err := svc.ListThemes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"success": false, "message": fmt.Sprintf("获取主题失败: %v", err)})
		return
	}
	active := theme.NewService(s.cfg.HugoRoot, nil, nil).ActiveTheme()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "themes": themes, "active_theme": active,
	})
}

// GET /api/themes/available
func (s *Server) handleThemeAvailable(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "available_themes": theme.DefaultThemes,
	})
}

// POST /api/themes/install
func (s *Server) handleThemeInstall(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	repoURL, _ := data["repo_url"].(string)
	name, _ := data["name"].(string)
	mode, _ := data["mode"].(string)
	result, err := s.themeService().InstallTheme(repoURL, name, mode)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "主题安装成功", "theme": result,
	})
}

// POST /api/themes/activate
func (s *Server) handleThemeActivate(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	name, _ := data["name"].(string)
	result, err := s.themeService().ActivateTheme(name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "主题已激活", "theme": result,
	})
}

// POST /api/themes/preview —— 主题预览重启 Hugo server
func (s *Server) handleThemePreview(w http.ResponseWriter, r *http.Request) {
	data := jsonDict(r)
	name, _ := data["name"].(string)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "主题名称不能为空"})
		return
	}
	svc := s.themeService()
	if !svc.ThemeExists(name) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success": false, "message": fmt.Sprintf("主题不存在: %s", name)})
		return
	}
	// 停止后带主题重启（对齐 preview_theme；不持久化活跃主题）
	s.hugo.Stop()
	ok, message := s.hugo.Start(false, name)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": message})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true, "message": "预览服务器已启动",
		"preview_theme": name, "server_url": s.hugo.ServerURL(),
	})
}

// ============ 内容静态文件（/api/content 与 /content） ============

// GET /api/content/{path...} —— 对齐 page_routes.serve_content_files
func (s *Server) handleContentFile(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	for _, part := range strings.Split(filepath.ToSlash(filename), "/") {
		if strings.HasPrefix(part, ".") {
			writeJSON(w, http.StatusForbidden, map[string]any{
				"success": false, "message": "访问被拒绝"})
			return
		}
	}
	full := filepath.Join(s.cfg.ContentDir, filepath.FromSlash(filename))
	if !isSafeWithin(full, s.cfg.ContentDir) || !isFile(full) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
}

// ============ 工具 ============

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// decodeStrict / decodeJSONStrict：非法 JSON 返回错误（对齐 is_json 检查）。
func decodeStrict(content string, v any) error {
	return json.NewDecoder(strings.NewReader(content)).Decode(v)
}

func decodeJSONStrict(r *http.Request, v *map[string]any) error {
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}
