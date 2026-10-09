// Package plugin 管理插件子进程（gRPC）：发现/生命周期/配置/能力代理。
// 行为对齐 services/plugin_manager.py 与 plugin_manifest.py。
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Manifest 对齐 PluginManifest。
type Manifest struct {
	Name            string
	Version         string
	Entry           string
	Author          string
	Description     string
	ProtocolVersion string
	Priority        int
	Platform        string
	Arch            string
	Capabilities    []string
	ConfigSchema    map[string]any
	PluginDir       string
}

// ParseManifest 对齐 parse_manifest：校验必填字段、entry 路径安全、
// capabilities 至少一个为 true、config_schema.schema 为合法 JSON。
func ParseManifest(pluginDir string) (*Manifest, error) {
	tomlPath := filepath.Join(pluginDir, "plugin.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return nil, fmt.Errorf("No plugin.toml found in %s", pluginDir)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("Malformed TOML in %s: %v", tomlPath, err)
	}

	pluginSection, _ := raw["plugin"].(map[string]any)
	if pluginSection == nil {
		return nil, fmt.Errorf("Missing [plugin] section in %s", tomlPath)
	}
	for _, key := range []string{"name", "version", "entry"} {
		if _, ok := pluginSection[key]; !ok {
			return nil, fmt.Errorf("Missing required fields in [plugin]: %s in %s", key, tomlPath)
		}
	}

	m := &Manifest{
		Name:            strField(pluginSection, "name"),
		Version:         strField(pluginSection, "version"),
		Entry:           strField(pluginSection, "entry"),
		Author:          strField(pluginSection, "author"),
		Description:     strField(pluginSection, "description"),
		ProtocolVersion: strOr(pluginSection, "protocol_version", "1"),
		Priority:        intField(pluginSection, "priority"),
		PluginDir:       pluginDir,
	}

	// entry 路径不得逃逸插件目录
	entryResolved := resolveWithin(pluginDir, m.Entry)
	if entryResolved == "" {
		return nil, fmt.Errorf("Entry path '%s' escapes plugin directory in %s", m.Entry, tomlPath)
	}

	capsSection, _ := raw["capabilities"].(map[string]any)
	for k, v := range capsSection {
		if b, ok := v.(bool); ok && b {
			m.Capabilities = append(m.Capabilities, k)
		}
	}
	if len(m.Capabilities) == 0 {
		return nil, fmt.Errorf("No capabilities declared in %s", tomlPath)
	}

	if build, ok := raw["build"].(map[string]any); ok {
		m.Platform = strField(build, "platform")
		m.Arch = strField(build, "arch")
	}

	if cs, ok := raw["config_schema"].(map[string]any); ok {
		if schemaStr, ok := cs["schema"].(string); ok && schemaStr != "" {
			if err := json.Unmarshal([]byte(schemaStr), &m.ConfigSchema); err != nil {
				return nil, fmt.Errorf("Invalid JSON in config_schema.schema in %s: %v", tomlPath, err)
			}
		}
	}
	if m.ConfigSchema == nil {
		m.ConfigSchema = map[string]any{}
	}
	return m, nil
}

// ResolveEntryPath 对齐 resolve_entry_path：realpath 解引用符号链接后
// 仍须在插件目录内，且文件存在、可执行。
func (m *Manifest) ResolveEntryPath() (string, error) {
	entryReal, err := filepath.EvalSymlinks(filepath.Join(m.PluginDir, m.Entry))
	if err != nil {
		return "", fmt.Errorf("Entry binary not found: %v", err)
	}
	dirReal, err := filepath.EvalSymlinks(m.PluginDir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(dirReal, entryReal)
	if err != nil || len(rel) >= 2 && rel[:2] == ".." {
		return "", fmt.Errorf("Entry '%s' resolves outside plugin directory after symlink dereference", m.Entry)
	}
	st, err := os.Stat(entryReal)
	if err != nil || st.IsDir() {
		return "", fmt.Errorf("Entry binary not found: %s", entryReal)
	}
	if st.Mode()&0o111 == 0 {
		return "", fmt.Errorf("Entry binary is not executable: %s", entryReal)
	}
	return entryReal, nil
}

// resolveWithin 返回 entry 相对 dir 解析后的路径；逃逸返回空串。
func resolveWithin(dir, entry string) string {
	abs, err := filepath.Abs(filepath.Join(dir, entry))
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil || (len(rel) >= 2 && rel[:2] == "..") {
		return ""
	}
	return abs
}

func strField(m map[string]any, key string) string { return strOr(m, key, "") }

func strOr(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok && v != "" {
		return v
	}
	return def
}

func intField(m map[string]any, key string) int {
	if v, ok := m[key].(int64); ok {
		return int(v)
	}
	return 0
}
