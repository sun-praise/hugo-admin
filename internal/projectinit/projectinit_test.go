package projectinit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newLocalGitRepo 建一个带单次提交的本地 git 仓库，替代真实 GitHub 克隆。
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

// overrideThemeRepo 将默认主题仓库指向本地仓库，测试后恢复。
func overrideThemeRepo(t *testing.T, repo string) {
	t.Helper()
	old := defaultThemeRepo
	defaultThemeRepo = repo
	t.Cleanup(func() { defaultThemeRepo = old })
}

func TestValidateTargetPath(t *testing.T) {
	adminRoot := t.TempDir()

	if _, err := ValidateTargetPath(adminRoot, "rel/path"); err == nil {
		t.Fatal("相对路径应被拒绝")
	}
	if _, err := ValidateTargetPath(adminRoot, filepath.Join(adminRoot, "sub")); err == nil {
		t.Fatal("位于 hugo-admin 目录内的路径应被拒绝")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hugo.toml"), []byte("title = 'x'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTargetPath(adminRoot, dir); err == nil {
		t.Fatal("已包含 Hugo 配置的目录应被拒绝")
	}

	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "config", "_default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "config", "_default", "config.yaml"), []byte("a: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateTargetPath(adminRoot, dir2); err == nil {
		t.Fatal("config/_default 形式的站点配置应被拒绝")
	}

	clean := filepath.Join(t.TempDir(), "site")
	p, err := ValidateTargetPath(adminRoot, clean+string(filepath.Separator))
	if err != nil {
		t.Fatalf("合法路径被拒绝: %v", err)
	}
	if p != filepath.Clean(clean) {
		t.Fatalf("p = %q, want %q", p, filepath.Clean(clean))
	}
}

func TestCreateSiteErrors(t *testing.T) {
	adminRoot := t.TempDir()
	if _, err := CreateSite(adminRoot, filepath.Join(t.TempDir(), "s"), "json"); err == nil {
		t.Fatal("非 toml/yaml 格式应报错")
	}
	if _, err := CreateSite(adminRoot, "rel", "toml"); err == nil {
		t.Fatal("相对路径应报错")
	}
	missing := filepath.Join(t.TempDir(), "no-parent", "site")
	if _, err := CreateSite(adminRoot, missing, "toml"); err == nil {
		t.Fatal("父目录不存在应报错")
	}
}

func TestCreateSiteToml(t *testing.T) {
	overrideThemeRepo(t, newLocalGitRepo(t))
	adminRoot := t.TempDir()
	target := filepath.Join(t.TempDir(), "site")

	res, err := CreateSite(adminRoot, target, "toml")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	if res.Path != target || res.ConfigFormat != "toml" {
		t.Fatalf("res = %+v", res)
	}
	if res.DefaultTheme["installed"] != true || res.DefaultTheme["activated"] != true {
		t.Fatalf("DefaultTheme = %#v", res.DefaultTheme)
	}

	raw, err := os.ReadFile(filepath.Join(target, "hugo.toml"))
	if err != nil {
		t.Fatalf("hugo.toml: %v", err)
	}
	if !strings.Contains(string(raw), `theme = "Fried-Rice"`) {
		t.Fatalf("hugo.toml = %q", raw)
	}
	// 主题成功安装后应删除占位布局
	if _, err := os.Stat(filepath.Join(target, "layouts")); !os.IsNotExist(err) {
		t.Fatal("安装主题后 layouts 应被移除")
	}
	// 主题已克隆且 .git 被移除
	if _, err := os.Stat(filepath.Join(target, "themes", "Fried-Rice", "theme.toml")); err != nil {
		t.Fatalf("主题未克隆成功: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "themes", "Fried-Rice", ".git")); !os.IsNotExist(err) {
		t.Fatal("克隆后 .git 应被移除")
	}
}

func TestCreateSiteYaml(t *testing.T) {
	overrideThemeRepo(t, newLocalGitRepo(t))
	target := filepath.Join(t.TempDir(), "site-yaml")

	if _, err := CreateSite(t.TempDir(), target, "yaml"); err != nil {
		t.Fatalf("CreateSite: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "hugo.yaml")); err != nil {
		t.Fatalf("hugo.yaml: %v", err)
	}
	// yaml 模式下 hugo new site 生成的 hugo.toml 应被删除
	if _, err := os.Stat(filepath.Join(target, "hugo.toml")); !os.IsNotExist(err) {
		t.Fatal("yaml 模式下应删除 hugo.toml")
	}
}

func TestInstallDefaultTheme(t *testing.T) {
	// 克隆失败：仓库地址不可达
	overrideThemeRepo(t, filepath.Join(t.TempDir(), "not-exist"))
	site := t.TempDir()
	result := installDefaultTheme(site)
	if result["installed"] != false || result["error"] == nil {
		t.Fatalf("克隆失败应记录 error: %#v", result)
	}

	// themes 目录已存在同名主题 → 跳过克隆，直接激活
	site2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(site2, "themes", "Fried-Rice"), 0o755); err != nil {
		t.Fatal(err)
	}
	result = installDefaultTheme(site2)
	if result["installed"] != false || result["activated"] != true {
		t.Fatalf("已存在主题应跳过安装直接激活: %#v", result)
	}
}

func TestWriteAndRemoveDefaultLayouts(t *testing.T) {
	site := t.TempDir()
	writeDefaultLayouts(site)
	for _, name := range []string{"baseof.html", "list.html", "single.html"} {
		if _, err := os.Stat(filepath.Join(site, "layouts", "_default", name)); err != nil {
			t.Fatalf("layouts/_default/%s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(site, "layouts", "index.html")); err != nil {
		t.Fatalf("layouts/index.html: %v", err)
	}

	RemoveDefaultLayouts(site)
	if _, err := os.Stat(filepath.Join(site, "layouts")); !os.IsNotExist(err) {
		t.Fatal("RemoveDefaultLayouts 后 layouts 应不存在")
	}
}
