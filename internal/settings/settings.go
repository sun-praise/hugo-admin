// Package settings 对齐 services/settings_service.py：
// JSON 设置文件（.admin/settings.json，0600）、默认值合并、
// legacy 文件迁移、listmonk secret.yml 迁移、公共视图脱敏。
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type ValidationError struct{ Msg string }
type StorageError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }
func (e *StorageError) Error() string    { return e.Msg }

type Settings struct {
	AI       map[string]any `json:"ai"`
	Hugo     map[string]any `json:"hugo"`
	Listmonk map[string]any `json:"listmonk"`
	Theme    map[string]any `json:"theme"`
}

type Service struct {
	settingsFile       string
	legacySettingsFile string
	defaults           map[string]string

	mu sync.Mutex
}

func NewService(settingsFile string, defaults map[string]string, legacyFile string) *Service {
	_ = os.MkdirAll(filepath.Dir(settingsFile), 0o755)
	s := &Service{settingsFile: settingsFile, legacySettingsFile: legacyFile, defaults: defaults}
	s.migrateLegacyIfNeeded()
	return s
}

// GetSettings 获取当前设置（默认值 + 文件覆盖 + 规范化落盘）。
func (s *Service) GetSettings() (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadUnlocked()
}

// MaskAPIKey 对齐 _mask_api_key。
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	runes := []rune(key)
	if len(runes) <= 8 {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:4]) + "..." + string(runes[len(runes)-4:])
}

// UpdateSettings 对齐 update_settings：合并更新 + 校验 + 原子写。
func (s *Service) UpdateSettings(updates map[string]any) (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.loadUnlocked()
	if err != nil {
		return nil, err
	}

	// AI
	if aiUpdates, ok := updates["ai"].(map[string]any); ok {
		if v, ok := aiUpdates["base_url"]; ok {
			current.AI["base_url"] = v
		}
		if v, ok := aiUpdates["model"]; ok {
			current.AI["model"] = v
		}
	}

	// Hugo
	if hugoUpdates, ok := updates["hugo"].(map[string]any); ok {
		if v, ok := hugoUpdates["base_dir"]; ok {
			baseDir, _ := v.(string)
			if trimmed := strings.TrimSpace(baseDir); trimmed != "" {
				if !filepath.IsAbs(trimmed) {
					return nil, &ValidationError{"Hugo 根目录必须是绝对路径"}
				}
				if st, err := os.Stat(trimmed); err != nil || !st.IsDir() {
					return nil, &ValidationError{fmt.Sprintf("Hugo 根目录不存在: %s", trimmed)}
				}
				if !hasHugoConfig(trimmed) {
					return nil, &ValidationError{fmt.Sprintf("目录中未找到 Hugo 配置文件: %s", trimmed)}
				}
			}
			current.Hugo["base_dir"] = strings.TrimSpace(baseDir)
		}
		if v, ok := hugoUpdates["server_url"]; ok {
			serverURL, _ := v.(string)
			serverURL = strings.TrimSpace(serverURL)
			if serverURL != "" && !strings.HasPrefix(serverURL, "http://") && !strings.HasPrefix(serverURL, "https://") {
				return nil, &ValidationError{"Hugo 服务器 URL 必须以 http:// 或 https:// 开头"}
			}
			current.Hugo["server_url"] = serverURL
		}
	}

	// Listmonk
	if lmUpdates, ok := updates["listmonk"].(map[string]any); ok {
		if v, ok := lmUpdates["api_url"]; ok {
			current.Listmonk["api_url"] = v
		}
		if v, ok := lmUpdates["api_user"]; ok {
			current.Listmonk["api_user"] = v
		}
		if v, ok := lmUpdates["api_key"]; ok {
			current.Listmonk["api_key"] = v
		}
		if v, ok := lmUpdates["blog_list_id"]; ok {
			current.Listmonk["blog_list_id"] = toInt(v, 1)
		}
	}

	// Theme
	if themeUpdates, ok := updates["theme"].(map[string]any); ok {
		if v, ok := themeUpdates["name"]; ok {
			name, _ := v.(string)
			name = strings.TrimSpace(name)
			if name != "" && (strings.HasPrefix(name, ".") || strings.Contains(name, "/") || strings.Contains(name, `\`)) {
				return nil, &ValidationError{"主题名称不能包含路径分隔符或特殊字符"}
			}
			current.Theme["name"] = name
		}
	}

	normalized, err := normalizeAndValidate(current)
	if err != nil {
		return nil, err
	}
	if err := s.write(normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// ToPublicSettings 对齐 to_public_settings：隐藏敏感值。
func (s *Service) ToPublicSettings(st *Settings) map[string]any {
	apiKey, _ := st.Listmonk["api_key"].(string)
	return map[string]any{
		"ai": map[string]any{
			"base_url": strOrDef(st.AI, "base_url", "https://api.deepseek.com"),
			"model":    strOrDef(st.AI, "model", "deepseek-chat"),
		},
		"hugo": map[string]any{
			"base_dir":   strOrDef(st.Hugo, "base_dir", ""),
			"server_url": strOrDef(st.Hugo, "server_url", ""),
		},
		"listmonk": map[string]any{
			"api_url":      strOrDef(st.Listmonk, "api_url", ""),
			"api_user":     strOrDef(st.Listmonk, "api_user", ""),
			"api_key":      MaskAPIKey(apiKey),
			"blog_list_id": toInt(st.Listmonk["blog_list_id"], 1),
		},
		"theme": map[string]any{
			"name": strOrDef(st.Theme, "name", ""),
		},
	}
}

func (s *Service) loadUnlocked() (*Settings, error) {
	fileExists := fileExists(s.settingsFile)
	st := s.defaults_()

	var fileSettings map[string]any
	raw, err := os.ReadFile(s.settingsFile)
	if err == nil {
		if err := json.Unmarshal(raw, &fileSettings); err != nil {
			return nil, &StorageError{fmt.Sprintf("设置文件 JSON 格式错误: %v", err)}
		}
	} else if !os.IsNotExist(err) {
		return nil, &StorageError{fmt.Sprintf("读取设置文件失败: %v", err)}
	}
	if fileSettings == nil {
		fileSettings = map[string]any{}
	}

	needsSanitize := false
	if ai, ok := fileSettings["ai"].(map[string]any); ok {
		for _, k := range []string{"base_url", "model"} {
			if v, ok := ai[k]; ok {
				st.AI[k] = v
			}
		}
		if _, ok := ai["api_key"]; ok {
			needsSanitize = true
		}
	}
	if hugo, ok := fileSettings["hugo"].(map[string]any); ok {
		for _, k := range []string{"base_dir", "server_url"} {
			if v, ok := hugo[k]; ok {
				st.Hugo[k] = v
			}
		}
	}
	lmFromFile, hasLmFile := fileSettings["listmonk"].(map[string]any)
	if hasLmFile {
		for _, k := range []string{"api_url", "api_user", "api_key", "blog_list_id"} {
			if v, ok := lmFromFile[k]; ok {
				st.Listmonk[k] = v
			}
		}
		if _, ok := lmFromFile["api_key"]; ok {
			needsSanitize = true
		}
	}
	if theme, ok := fileSettings["theme"].(map[string]any); ok {
		if v, ok := theme["name"]; ok {
			st.Theme["name"] = v
		}
	}

	// listmonk 未配置时从 ~/.config/secret.yml 迁移
	migrated := false
	lmURL, _ := st.Listmonk["api_url"].(string)
	if !hasLmFile && lmURL == "" {
		migrated = s.tryMigrateListmonkFromSecretYml(st)
	}

	normalized, err := normalizeAndValidate(st)
	if err != nil {
		return nil, err
	}
	if !fileExists || needsSanitize || migrated {
		_ = s.write(normalized)
	}
	return normalized, nil
}

func (s *Service) defaults_() *Settings {
	return &Settings{
		AI: map[string]any{
			"base_url": orDefault(s.defaults["AI_BASE_URL"], "https://api.deepseek.com"),
			"model":    orDefault(s.defaults["AI_MODEL"], "deepseek-chat"),
		},
		Hugo: map[string]any{
			"base_dir":   orDefault(s.defaults["HUGO_BASE_DIR"], ""),
			"server_url": orDefault(s.defaults["HUGO_SERVER_URL"], ""),
		},
		Listmonk: map[string]any{
			"api_url":      orDefault(s.defaults["LISTMONK_API_URL"], ""),
			"api_user":     orDefault(s.defaults["LISTMONK_API_USER"], ""),
			"api_key":      "",
			"blog_list_id": 1,
		},
		Theme: map[string]any{"name": ""},
	}
}

func (s *Service) write(st *Settings) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return &StorageError{err.Error()}
	}
	tmp := s.settingsFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return &StorageError{fmt.Sprintf("保存设置失败: %v", err)}
	}
	if err := os.Rename(tmp, s.settingsFile); err != nil {
		return &StorageError{fmt.Sprintf("保存设置失败: %v", err)}
	}
	_ = os.Chmod(s.settingsFile, 0o600)
	return nil
}

// migrateLegacyIfNeeded 对齐 _migrate_legacy_file_if_needed。
func (s *Service) migrateLegacyIfNeeded() {
	if s.legacySettingsFile == "" {
		return
	}
	if fileExists(s.settingsFile) || !fileExists(s.legacySettingsFile) {
		return
	}
	raw, err := os.ReadFile(s.legacySettingsFile)
	if err != nil {
		return
	}
	var legacy map[string]any
	if json.Unmarshal(raw, &legacy) != nil || legacy == nil {
		return
	}
	st := s.defaults_()
	if ai, ok := legacy["ai"].(map[string]any); ok {
		if v, ok := ai["base_url"]; ok {
			st.AI["base_url"] = v
		}
		if v, ok := ai["model"]; ok {
			st.AI["model"] = v
		}
	}
	normalized, err := normalizeAndValidate(st)
	if err != nil {
		return
	}
	if s.write(normalized) == nil {
		_ = os.Remove(s.legacySettingsFile)
	}
}

func (s *Service) tryMigrateListmonkFromSecretYml(st *Settings) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(filepath.Join(home, ".config", "secret.yml"))
	if err != nil {
		return false
	}
	var data map[string]any
	if yaml.Unmarshal(raw, &data) != nil || data == nil {
		return false
	}
	lm, ok := data["listmonk"].(map[string]any)
	if !ok {
		return false
	}
	if v, ok := lm["api_url"]; ok {
		st.Listmonk["api_url"] = fmt.Sprintf("%v", v)
	}
	if v, ok := lm["api_user"]; ok {
		st.Listmonk["api_user"] = fmt.Sprintf("%v", v)
	}
	if v, ok := lm["api_key"]; ok {
		st.Listmonk["api_key"] = fmt.Sprintf("%v", v)
	}
	if v, ok := lm["blog_list_id"]; ok {
		st.Listmonk["blog_list_id"] = toInt(v, 1)
	}
	url, _ := st.Listmonk["api_url"].(string)
	return url != ""
}

// normalizeAndValidate 对齐 _normalize_and_validate。
func normalizeAndValidate(st *Settings) (*Settings, error) {
	baseURL := strings.TrimSpace(fmt.Sprintf("%v", orNil(st.AI["base_url"], "")))
	if baseURL == "" {
		return nil, &ValidationError{"AI Base URL 不能为空"}
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		return nil, &ValidationError{"AI Base URL 必须以 http:// 或 https:// 开头"}
	}
	model := strings.TrimSpace(fmt.Sprintf("%v", orNil(st.AI["model"], "")))
	if model == "" {
		return nil, &ValidationError{"AI 模型不能为空"}
	}
	return &Settings{
		AI: map[string]any{"base_url": baseURL, "model": model},
		Hugo: map[string]any{
			"base_dir":   strings.TrimSpace(fmt.Sprintf("%v", orNil(st.Hugo["base_dir"], ""))),
			"server_url": strings.TrimSpace(fmt.Sprintf("%v", orNil(st.Hugo["server_url"], ""))),
		},
		Listmonk: map[string]any{
			"api_url":      strings.TrimSpace(fmt.Sprintf("%v", orNil(st.Listmonk["api_url"], ""))),
			"api_user":     strings.TrimSpace(fmt.Sprintf("%v", orNil(st.Listmonk["api_user"], ""))),
			"api_key":      fmt.Sprintf("%v", orNil(st.Listmonk["api_key"], "")),
			"blog_list_id": toInt(st.Listmonk["blog_list_id"], 1),
		},
		Theme: map[string]any{
			"name": strings.TrimSpace(fmt.Sprintf("%v", orNil(st.Theme["name"], ""))),
		},
	}, nil
}

func hasHugoConfig(dir string) bool {
	candidates := []string{
		"config.toml", "config.yaml", "hugo.toml", "hugo.yaml", "config.json",
	}
	for _, f := range candidates {
		if fileExists(filepath.Join(dir, f)) {
			return true
		}
	}
	return fileExists(filepath.Join(dir, "config", "_default", "config.toml")) ||
		fileExists(filepath.Join(dir, "config", "_default", "config.yaml"))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}

func orNil(v any, def any) any {
	if v == nil {
		return def
	}
	return v
}

func strOrDef(m map[string]any, key, def string) any {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	if _, exists := m[key]; !exists {
		return def
	}
	return fmt.Sprintf("%v", m[key])
}

func toInt(v any, def int) int {
	switch t := v.(type) {
	case int:
		return t
	case float64:
		return int(t)
	case string:
		var n int
		if _, err := fmt.Sscanf(t, "%d", &n); err == nil {
			return n
		}
	}
	return def
}
