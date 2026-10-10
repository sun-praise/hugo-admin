// Package projectinit 对齐 services/project_init_service.py：
// hugo new site + 默认配置/布局 + 默认主题 + 活跃项目切换。
package projectinit

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var hugoConfigFiles = []string{"config.toml", "config.yaml", "config.json", "hugo.toml", "hugo.yaml"}

const (
	defaultThemeRepo = "https://github.com/Svtter/Fried-Rice.git"
	defaultThemeName = "Fried-Rice"
)

// ValidateTargetPath 对齐 validate_target_path。
func ValidateTargetPath(adminRoot, targetPath string) (string, error) {
	if !filepath.IsAbs(targetPath) {
		return "", fmt.Errorf("目标路径必须是绝对路径")
	}
	path := filepath.Clean(targetPath)

	// 防止写入 hugo-admin 自身
	if rel, err := filepath.Rel(adminRoot, path); err == nil && !strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("目标路径不能位于 hugo-admin 安装目录内")
	}

	// 已有 Hugo 配置 → 拒绝
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		for _, name := range hugoConfigFiles {
			if _, err := os.Stat(filepath.Join(path, name)); err == nil {
				return "", fmt.Errorf("目标路径已包含 Hugo 配置文件: %s", name)
			}
		}
		for _, name := range []string{"config.toml", "config.yaml"} {
			if _, err := os.Stat(filepath.Join(path, "config", "_default", name)); err == nil {
				return "", fmt.Errorf("目标路径已包含 Hugo 站点配置")
			}
		}
	}
	return path, nil
}

// CreateSiteResult 对齐 create_site 返回。
type CreateSiteResult struct {
	Path         string
	ConfigFormat string
	DefaultTheme map[string]any
}

// CreateSite 对齐 create_site：hugo new site + 默认配置 + 布局 + 主题。
func CreateSite(adminRoot, targetPath, configFormat string) (*CreateSiteResult, error) {
	if configFormat != "toml" && configFormat != "yaml" {
		return nil, fmt.Errorf("配置文件格式仅支持 toml 或 yaml")
	}
	path, err := ValidateTargetPath(adminRoot, targetPath)
	if err != nil {
		return nil, err
	}
	parent := filepath.Dir(path)
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("目标路径的父目录不存在: %s", parent)
	}

	cmd := exec.Command("hugo", "new", "site", path)
	cmd.Dir = parent
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, lookErr := exec.LookPath("hugo"); lookErr != nil {
			return nil, fmt.Errorf("未找到 hugo 命令，请确保 Hugo 已安装")
		}
		return nil, fmt.Errorf("创建 Hugo 站点失败: %s", strings.TrimSpace(string(out)))
	}

	writeDefaultConfig(path, configFormat)
	writeDefaultLayouts(path)

	themeResult := installDefaultTheme(path)
	installed, _ := themeResult["installed"].(bool)
	activated, _ := themeResult["activated"].(bool)
	if installed || activated {
		RemoveDefaultLayouts(path)
	}

	return &CreateSiteResult{
		Path: path, ConfigFormat: configFormat, DefaultTheme: themeResult,
	}, nil
}

func writeDefaultConfig(siteRoot, configFormat string) {
	var configName, content string
	if configFormat == "toml" {
		configName = "hugo.toml"
		content = fmt.Sprintf("baseURL = \"https://example.org/\"\nlanguageCode = \"zh-CN\"\ntitle = \"My New Hugo Site\"\ntheme = \"%s\"\n", defaultThemeName)
	} else {
		configName = "hugo.yaml"
		content = fmt.Sprintf("baseURL: https://example.org/\nlanguageCode: zh-CN\ntitle: My New Hugo Site\ntheme: %s\n", defaultThemeName)
	}
	configFile := filepath.Join(siteRoot, configName)
	if configFormat == "yaml" {
		// hugo new site 默认生成 hugo.toml；yaml 时删除
		_ = os.Remove(filepath.Join(siteRoot, "hugo.toml"))
	}
	_ = os.WriteFile(configFile, []byte(content), 0o644)
}

// installDefaultTheme 对齐 _install_default_theme：失败不阻塞。
func installDefaultTheme(siteRoot string) map[string]any {
	result := map[string]any{
		"name": defaultThemeName, "repo": defaultThemeRepo,
		"installed": false, "activated": false,
		"skipped_reason": nil, "error": nil,
	}
	themesDir := filepath.Join(siteRoot, "themes", defaultThemeName)
	if _, err := os.Stat(themesDir); os.IsNotExist(err) {
		// copy 模式安装（新站点无 git 仓库）
		if out, err := exec.Command("git", "clone", "--depth", "1", defaultThemeRepo, themesDir).CombinedOutput(); err != nil {
			result["error"] = fmt.Sprintf("install_failed: %v\n%s", err, out)
			return result
		}
		_ = os.RemoveAll(filepath.Join(themesDir, ".git"))
		result["installed"] = true
	}
	result["activated"] = true
	return result
}

// writeDefaultLayouts 对齐 _write_default_layouts：占位布局。
func writeDefaultLayouts(siteRoot string) {
	defaultDir := filepath.Join(siteRoot, "layouts", "_default")
	_ = os.MkdirAll(defaultDir, 0o755)

	baseof := "<!DOCTYPE html>\n<html lang=\"zh-CN\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n  <title>{{ block \"title\" . }}{{ .Site.Title }}{{ end }}</title>\n</head>\n<body>\n  <main>{{ block \"main\" . }}{{ end }}</main>\n</body>\n</html>\n"
	_ = os.WriteFile(filepath.Join(defaultDir, "baseof.html"), []byte(baseof), 0o644)

	list := "{{ define \"title\" }}{{ .Title }} - {{ .Site.Title }}{{ end }}\n{{ define \"main\" }}\n  <h1>{{ .Title }}</h1>\n  <ul>\n  {{ range .Pages }}\n    <li><a href=\"{{ .Permalink }}\">{{ .Title }}</a></li>\n  {{ end }}\n  </ul>\n{{ end }}\n"
	_ = os.WriteFile(filepath.Join(defaultDir, "list.html"), []byte(list), 0o644)

	single := "{{ define \"title\" }}{{ .Title }} - {{ .Site.Title }}{{ end }}\n{{ define \"main\" }}\n  <article>\n    <h1>{{ .Title }}</h1>\n    {{ .Content }}\n  </article>\n{{ end }}\n"
	_ = os.WriteFile(filepath.Join(defaultDir, "single.html"), []byte(single), 0o644)

	index := "{{ define \"title\" }}{{ .Site.Title }}{{ end }}\n{{ define \"main\" }}\n  <h1>{{ .Site.Title }}</h1>\n  <ul>\n  {{ range .RegularPages }}\n    <li><a href=\"{{ .Permalink }}\">{{ .Title }}</a></li>\n  {{ end }}\n  </ul>\n{{ end }}\n"
	_ = os.WriteFile(filepath.Join(siteRoot, "layouts", "index.html"), []byte(index), 0o644)
}

// RemoveDefaultLayouts 对齐 _remove_default_layouts。
func RemoveDefaultLayouts(siteRoot string) {
	_ = os.RemoveAll(filepath.Join(siteRoot, "layouts"))
}
