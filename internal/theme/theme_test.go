package theme

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newLocalGitRepo 建一个带提交的本地 git 仓库，供克隆/子模块测试使用。
func newLocalGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "theme.toml"), []byte("name = \"local\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

func TestListThemes(t *testing.T) {
	root := t.TempDir()
	s := NewService(root, nil, nil)

	// themes 目录不存在 → 空列表
	themes, err := s.ListThemes()
	if err != nil || len(themes) != 0 {
		t.Fatalf("themes = %#v, err = %v", themes, err)
	}

	// alpha 带 JSON 可解析的 hugo.toml（带元数据）；beta 的 theme.toml 是 TOML，解析失败跳过元数据
	themesDir := filepath.Join(root, "themes")
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(themesDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(themesDir, "alpha", "hugo.toml"),
		[]byte(`{"version":"1.2","description":"Alpha theme"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themesDir, "beta", "theme.toml"),
		[]byte("name = 'beta'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	themes, err = s.ListThemes()
	if err != nil {
		t.Fatalf("ListThemes: %v", err)
	}
	if len(themes) != 2 || themes[0]["name"] != "alpha" || themes[1]["name"] != "beta" {
		t.Fatalf("themes = %#v（应按字母序）", themes)
	}
	if themes[0]["version"] != "1.2" || themes[0]["description"] != "Alpha theme" {
		t.Fatalf("alpha 元数据 = %#v", themes[0])
	}
	if _, ok := themes[1]["version"]; ok {
		t.Fatalf("beta 的 TOML 元数据应被跳过: %#v", themes[1])
	}
}

func TestActiveTheme(t *testing.T) {
	// 注入的 getActive 优先
	s := NewService(t.TempDir(), nil, func() string { return "injected" })
	if got := s.ActiveTheme(); got != "injected" {
		t.Fatalf("ActiveTheme = %q", got)
	}

	cases := []struct {
		file    string
		content string
		want    string
	}{
		{"hugo.toml", "theme = \"toml-theme\"\n", "toml-theme"},
		{"hugo.yaml", "theme: 'yaml-theme'\n", "yaml-theme"},
		{"config.toml", "theme = \"cfg-theme\"\n", "cfg-theme"},
		{"config.yaml", "theme: cfg-yaml\n", "cfg-yaml"},
	}
	for _, tc := range cases {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, tc.file), []byte(tc.content), 0o644); err != nil {
			t.Fatal(err)
		}
		got := NewService(root, nil, nil).ActiveTheme()
		if got != tc.want {
			t.Fatalf("%s: ActiveTheme = %q, want %q", tc.file, got, tc.want)
		}
	}
	// 无任何配置 → 空串
	if got := NewService(t.TempDir(), nil, nil).ActiveTheme(); got != "" {
		t.Fatalf("ActiveTheme = %q, want 空", got)
	}
}

func TestThemeExists(t *testing.T) {
	root := t.TempDir()
	themesDir := filepath.Join(root, "themes", "real")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s := NewService(root, nil, nil)

	if !s.ThemeExists("real") {
		t.Fatal("real 主题应存在")
	}
	if s.ThemeExists("missing") {
		t.Fatal("missing 主题不应存在")
	}
	// 非法名称（路径穿越/特殊字符）直接拒绝
	for _, name := range []string{"../escape", ".hidden", "with space", ""} {
		if s.ThemeExists(name) {
			t.Fatalf("非法名称 %q 不应通过", name)
		}
	}
}

func TestInstallThemeValidation(t *testing.T) {
	s := NewService(t.TempDir(), nil, nil)
	if _, err := s.InstallTheme("", "n", "clone"); err == nil {
		t.Fatal("空 repo_url 应报错")
	}
	if _, err := s.InstallTheme("u", "", "clone"); err == nil {
		t.Fatal("空 name 应报错")
	}
	if _, err := s.InstallTheme("u", "bad/name", "clone"); err == nil {
		t.Fatal("非法名称应报错")
	}
	// 已存在
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "themes", "dup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewService(root, nil, nil).InstallTheme("u", "dup", "clone"); err == nil {
		t.Fatal("已存在主题应报错")
	}
}

func TestInstallThemeClone(t *testing.T) {
	repo := newLocalGitRepo(t)
	root := t.TempDir()
	s := NewService(root, nil, nil)

	res, err := s.InstallTheme(repo, "cloned", "clone")
	if err != nil {
		t.Fatalf("InstallTheme: %v", err)
	}
	if res["name"] != "cloned" || res["repo_url"] != repo {
		t.Fatalf("res = %#v", res)
	}
	// 克隆产物存在且 .git 已移除
	if _, err := os.Stat(filepath.Join(root, "themes", "cloned", "theme.toml")); err != nil {
		t.Fatalf("克隆结果: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "themes", "cloned", ".git")); !os.IsNotExist(err) {
		t.Fatal("clone 模式应移除 .git")
	}
}

func TestInstallThemeSubmodule(t *testing.T) {
	repo := newLocalGitRepo(t)
	root := t.TempDir()
	// hugoRoot 必须是 git 仓库才能 submodule add。
	// 本地路径需放行 file 协议（git CVE-2022-39253 后默认禁止）；
	// 通过环境变量传递，确保生产代码 fork 的 git 子进程同样生效
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "protocol.file.allow")
	t.Setenv("GIT_CONFIG_VALUE_0", "always")
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	s := NewService(root, nil, nil)
	res, err := s.InstallTheme(repo, "submod", "submodule")
	if err != nil {
		t.Fatalf("InstallTheme(submodule): %v", err)
	}
	if res["name"] != "submod" {
		t.Fatalf("res = %#v", res)
	}

	// 非 git 仓库上 submodule 应失败
	if _, err := NewService(t.TempDir(), nil, nil).InstallTheme(repo, "sub2", "submodule"); err == nil {
		t.Fatal("非 git 仓库 submodule add 应报错")
	}
}

func TestActivateTheme(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "themes", "act"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 主题不存在
	if _, err := NewService(root, nil, nil).ActivateTheme("nope"); err == nil {
		t.Fatal("主题不存在应报错")
	}
	// setActive 未注入
	if _, err := NewService(root, nil, nil).ActivateTheme("act"); err == nil {
		t.Fatal("设置服务不可用应报错")
	}
	// setActive 报错
	fail := func(string) error { return os.ErrPermission }
	if _, err := NewService(root, fail, nil).ActivateTheme("act"); err == nil {
		t.Fatal("保存失败应报错")
	}
	// 成功
	ok := func(string) error { return nil }
	res, err := NewService(root, ok, nil).ActivateTheme("act")
	if err != nil || res["name"] != "act" {
		t.Fatalf("res = %#v, err = %v", res, err)
	}
}

func TestDetectSubmodules(t *testing.T) {
	// 非 git 目录 → 空
	if got := DetectSubmodules(t.TempDir()); len(got) != 0 {
		t.Fatalf("非 git 目录应返回空: %#v", got)
	}

	// 真实 submodule：root 仓库 + submodule add 本地仓库
	repo := newLocalGitRepo(t)
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "init")
	// 本地路径 submodule 需显式放行 file 协议（git CVE-2022-39253 后默认禁止）
	run("-c", "protocol.file.allow=always", "submodule", "add", "-f", repo, "themes/gamma")

	got := DetectSubmodules(root)
	if !got["gamma"] {
		t.Fatalf("应检测到 themes/gamma: %#v", got)
	}
}
