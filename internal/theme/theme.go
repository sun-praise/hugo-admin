// Package theme 对齐 services/theme_service.py：
// 已安装主题发现（themes/ 目录 + submodule 检测）、
// 安装（submodule/clone）、激活（改 Hugo 配置）与默认主题目录。
package theme

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DefaultThemes 对齐 ThemeService.list_default_themes。
var DefaultThemes = []map[string]any{
	{
		"name":        "Fried-Rice",
		"repo_url":    "https://github.com/Svtter/Fried-Rice",
		"description": "svtter 的默认博客主题",
	},
}

type Service struct {
	hugoRoot     string
	themesDir    string
	setActive    func(name string) error
	getActive    func() string
	submoduleSet func() map[string]bool
}

func NewService(hugoRoot string, setActive func(string) error, getActive func() string) *Service {
	return &Service{
		hugoRoot:     hugoRoot,
		themesDir:    filepath.Join(hugoRoot, "themes"),
		setActive:    setActive,
		getActive:    getActive,
		submoduleSet: func() map[string]bool { return DetectSubmodules(hugoRoot) },
	}
}

type ThemeInfo struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
	IsSubmodule bool   `json:"is_submodule"`
	IsActive    bool   `json:"is_active"`
}

// ListThemes 对齐 list_themes：themes/ 下的目录（含 theme.toml/hugo.toml 的版本描述）。
func (s *Service) ListThemes() ([]map[string]any, error) {
	entries, err := os.ReadDir(s.themesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []map[string]any{}, nil
		}
		return nil, fmt.Errorf("读取主题目录失败: %v", err)
	}
	submodules := s.submoduleSet()
	active := s.ActiveTheme()

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	themes := []map[string]any{}
	for _, name := range names {
		info := map[string]any{
			"name":         name,
			"is_submodule": submodules[name],
			"is_active":    name == active,
		}
		// 版本/描述来自主题的 theme.toml / hugo.toml（宽松解析）
		for _, meta := range []string{"theme.toml", "hugo.toml"} {
			if raw, err := os.ReadFile(filepath.Join(s.themesDir, name, meta)); err == nil {
				var m map[string]any
				if json.Unmarshal(raw, &m) != nil {
					continue // TOML 文件不按 JSON 解析，跳过元数据
				}
				if v, ok := m["version"]; ok {
					info["version"] = fmt.Sprintf("%v", v)
				}
				if v, ok := m["description"]; ok {
					info["description"] = fmt.Sprintf("%v", v)
				}
				break
			}
		}
		themes = append(themes, info)
	}
	return themes, nil
}

// ActiveTheme 对齐 get_active_theme：解析 hugo 配置的 theme 字段。
func (s *Service) ActiveTheme() string {
	if s.getActive != nil {
		return s.getActive()
	}
	for _, name := range []string{"hugo.toml", "hugo.yaml", "config.toml", "config.yaml"} {
		raw, err := os.ReadFile(filepath.Join(s.hugoRoot, name))
		if err != nil {
			continue
		}
		re := regexp.MustCompile(`(?m)^\s*theme\s*=\s*"([^"]+)"`)
		if m := re.FindSubmatch(raw); m != nil {
			return string(m[1])
		}
		reYaml := regexp.MustCompile(`(?m)^\s*theme:\s*(\S+)`)
		if m := reYaml.FindSubmatch(raw); m != nil {
			return strings.Trim(string(m[1]), `"'`)
		}
	}
	return ""
}

// ThemeExists 对齐 theme_exists。
func (s *Service) ThemeExists(name string) bool {
	if !validThemeName(name) {
		return false
	}
	st, err := os.Stat(filepath.Join(s.themesDir, name))
	return err == nil && st.IsDir()
}

var reSafeTheme = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func validThemeName(name string) bool {
	return reSafeTheme.MatchString(name)
}

// InstallTheme 对齐 install_theme：submodule 或 clone 模式。
func (s *Service) InstallTheme(repoURL, name, mode string) (map[string]any, error) {
	if repoURL == "" || name == "" {
		return nil, fmt.Errorf("repo_url 与 name 不能为空")
	}
	if !validThemeName(name) {
		return nil, fmt.Errorf("主题名称不合法: %s", name)
	}
	if s.ThemeExists(name) {
		return nil, fmt.Errorf("主题已存在: %s", name)
	}
	if mode == "" {
		mode = "submodule"
	}
	_ = os.MkdirAll(s.themesDir, 0o755)

	target := filepath.Join(s.themesDir, name)
	if mode == "submodule" {
		cmd := exec.Command("git", "-C", s.hugoRoot, "submodule", "add", "-f", repoURL, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("submodule 添加失败: %v\n%s", err, out)
		}
	} else {
		cmd := exec.Command("git", "clone", "--depth", "1", repoURL, target)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("克隆失败: %v\n%s", err, out)
		}
		os.RemoveAll(filepath.Join(target, ".git"))
	}
	return map[string]any{"name": name, "repo_url": repoURL}, nil
}

// ActivateTheme 对齐 activate_theme：写 settings 的 theme.name。
func (s *Service) ActivateTheme(name string) (map[string]any, error) {
	if !s.ThemeExists(name) {
		return nil, fmt.Errorf("主题不存在: %s", name)
	}
	if s.setActive == nil {
		return nil, fmt.Errorf("设置服务不可用")
	}
	if err := s.setActive(name); err != nil {
		return nil, fmt.Errorf("保存活跃主题失败: %v", err)
	}
	return map[string]any{"name": name}, nil
}

// DetectSubmodules 是可用的 submodule 检测（按 hugoRoot）。
func DetectSubmodules(hugoRoot string) map[string]bool {
	result := map[string]bool{}
	out, err := exec.Command("git", "-C", hugoRoot, "submodule", "status", "--recursive").Output()
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		path := fields[1]
		if strings.HasPrefix(path, "themes/") {
			result[strings.TrimPrefix(path, "themes/")] = true
		}
	}
	return result
}
