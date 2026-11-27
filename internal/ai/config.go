package ai

import (
	"log"
	"strings"
	"sync"

	"github.com/sun-praise/hugo-admin/v3/internal/settings"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	defaultModel   = "deepseek-flash"
)

// Config 是一次 AI 请求所需的全部配置。frontmatter 生成、选区辅助、
// AI 改写与对话共用这一份配置。
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
}

// ConfigStore 是所有 AI 功能共享的配置来源：
//   - base_url / model 来自 .admin/settings.json，启动时读取一次，
//     设置页保存后由 Refresh 更新；
//   - api key 优先取设置页保存的会话 key，其次回退到环境变量。
//
// 配置保存在内存中，AI 请求路径不会做磁盘 IO；设置页保存 key 后
// 无需重启服务，下一次请求即生效。
type ConfigStore struct {
	settingsSvc *settings.Service
	envKey      string

	mu         sync.RWMutex
	sessionKey string
	baseURL    string
	model      string
}

func NewConfigStore(settingsSvc *settings.Service, envKey string) *ConfigStore {
	s := &ConfigStore{
		settingsSvc: settingsSvc,
		envKey:      strings.TrimSpace(envKey),
	}
	s.Refresh()
	return s
}

// Refresh 重新读取设置文件中的 base_url/model。设置页保存成功后调用。
func (s *ConfigStore) Refresh() {
	if s == nil {
		return
	}
	baseURL, modelName := defaultBaseURL, defaultModel
	if s.settingsSvc != nil {
		st, err := s.settingsSvc.GetSettings()
		if err != nil {
			log.Printf("读取 AI 设置失败，使用默认端点: %v", err)
		} else {
			if v, ok := st.AI["base_url"].(string); ok {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					baseURL = trimmed
				}
			}
			if v, ok := st.AI["model"].(string); ok {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					modelName = trimmed
				}
			}
		}
	}
	s.mu.Lock()
	s.baseURL, s.model = baseURL, modelName
	s.mu.Unlock()
}

// Get 返回当前生效的 AI 配置。
func (s *ConfigStore) Get() Config {
	if s == nil {
		return Config{}
	}
	s.mu.RLock()
	cfg := Config{BaseURL: s.baseURL, Model: s.model, APIKey: s.sessionKey}
	s.mu.RUnlock()
	if cfg.APIKey == "" {
		cfg.APIKey = s.envKey
	}
	return cfg
}

// SetAPIKey 保存设置页提交的会话 key（不落盘）。
func (s *ConfigStore) SetAPIKey(key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.sessionKey = strings.TrimSpace(key)
	s.mu.Unlock()
}

// KeyInfo 返回 key 来源与脱敏提示（用于设置页展示）。
func (s *ConfigStore) KeyInfo() (source, hint string) {
	if s == nil {
		return "none", ""
	}
	s.mu.RLock()
	sessionKey := s.sessionKey
	s.mu.RUnlock()
	switch {
	case sessionKey != "":
		return "session", settings.MaskAPIKey(sessionKey)
	case s.envKey != "":
		return "env", ""
	default:
		return "none", ""
	}
}
