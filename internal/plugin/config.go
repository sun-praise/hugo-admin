package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/fernet/fernet-go"
)

// ConfigStore 对齐 PluginConfigStore：插件配置持久化，
// 字符串值全量 Fernet 加密（_enc: 前缀）。密钥文件与 Python 共享
// （~/.hugo-admin/.secret_key），两套实现可互解对方存储的配置。
type ConfigStore struct {
	configPath string
	secretFile string

	mu   sync.Mutex
	data map[string]map[string]any
}

func NewConfigStore(configPath, secretFile string) (*ConfigStore, error) {
	s := &ConfigStore{configPath: configPath, secretFile: secretFile, data: map[string]map[string]any{}}
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(raw, &s.data); err != nil {
			fmt.Printf("Failed to load plugin config: %v\n", err)
			s.data = map[string]map[string]any{}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if _, err := s.fernetKey(); err != nil {
		return nil, err
	}
	return s, nil
}

// fernetKey 加载或生成机器密钥（对齐 _get_fernet，0600 权限）。
func (s *ConfigStore) fernetKey() (*fernet.Key, error) {
	if err := os.MkdirAll(filepath.Dir(s.secretFile), 0o755); err != nil {
		return nil, err
	}
	// 密钥文件是 Fernet 文本形式（44 字符 urlsafe base64），
	// 与 Python Fernet.generate_key() 的落盘格式一致（共享互认）
	keyText, err := os.ReadFile(s.secretFile)
	if err == nil && len(trimSpace(keyText)) > 0 {
		keyText = trimSpace(keyText)
	} else {
		var k fernet.Key
		if err := k.Generate(); err != nil {
			return nil, err
		}
		keyText = []byte(k.Encode())
		if err := os.WriteFile(s.secretFile, keyText, 0o600); err != nil {
			return nil, err
		}
	}
	_ = os.Chmod(s.secretFile, 0o600)
	return fernet.DecodeKey(string(keyText))
}

func trimSpace(b []byte) []byte {
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\n' || b[start] == '\r' || b[start] == '\t') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == '\t') {
		end--
	}
	return b[start:end]
}

// GetConfig 返回解密后的配置。
func (s *ConfigStore) GetConfig(pluginName string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw := s.data[pluginName]
	if raw == nil {
		return map[string]any{}
	}
	return s.decryptValues(raw)
}

// SetConfig 加密并保存。
func (s *ConfigStore) SetConfig(pluginName string, config map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enc, err := s.encryptValues(config)
	if err != nil {
		return err
	}
	s.data[pluginName] = enc
	return s.save()
}

func (s *ConfigStore) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.configPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.configPath)
}

func (s *ConfigStore) encryptValues(config map[string]any) (map[string]any, error) {
	key, err := s.fernetKey()
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range config {
		if str, ok := v.(string); ok && str != "" {
			token, err := fernet.EncryptAndSign([]byte(str), key)
			if err != nil {
				return nil, err
			}
			out[k] = "_enc:" + string(token)
		} else {
			out[k] = v
		}
	}
	return out, nil
}

func (s *ConfigStore) decryptValues(config map[string]any) map[string]any {
	key, err := s.fernetKey()
	out := map[string]any{}
	for k, v := range config {
		str, ok := v.(string)
		if !ok || len(str) < 5 || str[:5] != "_enc:" {
			out[k] = v
			continue
		}
		if err != nil {
			fmt.Printf("Failed to decrypt config value for key '%s'\n", k)
			out[k] = ""
			continue
		}
		// Python Fernet.decrypt 默认不校验 TTL，ttl<=0 等价
		plain := fernet.VerifyAndDecrypt([]byte(str[5:]), 0, []*fernet.Key{key})
		if plain == nil {
			fmt.Printf("Failed to decrypt config value for key '%s'\n", k)
			out[k] = ""
			continue
		}
		out[k] = string(plain)
	}
	return out
}
