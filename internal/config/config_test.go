package config

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetEnvForTest 彻底移除环境变量并在测试结束后恢复原值
// （t.Setenv 无法表达“变量不存在”，而 .env 只在变量不存在时生效）。
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestLoadDefaults(t *testing.T) {
	for _, key := range []string{"PORT", "SECRET_KEY", "AUTH_STORE", "ADMIN_UI_DIR", "CONTENT_DIR", "HUGO_ROOT"} {
		unsetEnvForTest(t, key)
	}
	root := t.TempDir()
	c := Load(root)
	if c.Port != "5050" {
		t.Fatalf("Port = %q", c.Port)
	}
	if c.SecretKey != "dev-secret-key-change-in-production" {
		t.Fatalf("SecretKey = %q", c.SecretKey)
	}
	if c.AuthStorePath != filepath.Join(root, "data", "auth.json") {
		t.Fatalf("AuthStorePath = %q", c.AuthStorePath)
	}
	if c.AdminUIDir != filepath.Join(root, "admin-ui") {
		t.Fatalf("AdminUIDir = %q", c.AdminUIDir)
	}
	if c.ContentDir != filepath.Join(root, "content") {
		t.Fatalf("ContentDir = %q", c.ContentDir)
	}
	if c.HugoRoot != root {
		t.Fatalf("HugoRoot = %q", c.HugoRoot)
	}
	// __version__.py 不存在时回退
	if c.Version != "0.0.0-go" {
		t.Fatalf("Version = %q", c.Version)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("PORT", "8961")
	t.Setenv("SECRET_KEY", "sk-test")
	t.Setenv("AUTH_STORE", "/tmp/auth-test.json")
	t.Setenv("ADMIN_UI_DIR", "/tmp/ui-test")
	t.Setenv("CONTENT_DIR", "/tmp/content-test")
	t.Setenv("HUGO_ROOT", "/tmp/root-test")

	c := Load(t.TempDir())
	if c.Port != "8961" || c.SecretKey != "sk-test" ||
		c.AuthStorePath != "/tmp/auth-test.json" || c.AdminUIDir != "/tmp/ui-test" ||
		c.ContentDir != "/tmp/content-test" || c.HugoRoot != "/tmp/root-test" {
		t.Fatalf("config = %+v", c)
	}
}

func TestLoadDotEnv(t *testing.T) {
	root := t.TempDir()
	// 已存在的环境变量不覆盖（python-dotenv override=False 语义）
	t.Setenv("DOTENV_KEEP", "keep")
	for _, key := range []string{"DOTENV_PORT", "DOTENV_QUOTED", "DOTENV_SINGLE"} {
		unsetEnvForTest(t, key)
	}
	unsetEnvForTest(t, "PORT")

	envFile := "# 注释行\n" +
		"PORT=6000\n" +
		"DOTENV_QUOTED=\"quoted value\"\n" +
		"DOTENV_SINGLE='single'\n" +
		"DOTENV_KEEP=new\n" +
		"invalid-line-without-eq\n" +
		"\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(envFile), 0o644); err != nil {
		t.Fatal(err)
	}

	c := Load(root)
	if c.Port != "6000" {
		t.Fatalf("Port = %q, .env 中的 PORT 应生效", c.Port)
	}
	if v := os.Getenv("DOTENV_QUOTED"); v != "quoted value" {
		t.Fatalf("DOTENV_QUOTED = %q, 双引号应被剥离", v)
	}
	if v := os.Getenv("DOTENV_SINGLE"); v != "single" {
		t.Fatalf("DOTENV_SINGLE = %q, 单引号应被剥离", v)
	}
	if v := os.Getenv("DOTENV_KEEP"); v != "keep" {
		t.Fatalf("DOTENV_KEEP = %q, 已存在的环境变量不应被覆盖", v)
	}
}

func TestReadVersion(t *testing.T) {
	dir := t.TempDir()

	write := func(name, content string) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	if v := readVersion(write("v1.py", "__version__ = \"3.2.1\"\n")); v != "3.2.1" {
		t.Fatalf("version = %q", v)
	}
	if v := readVersion(write("v2.py", "__version__ = '2.0'\n")); v != "2.0" {
		t.Fatalf("version = %q", v)
	}
	if v := readVersion(write("v3.py", "__version__ = \"\"\n")); v != "0.0.0-go" {
		t.Fatalf("空版本应回退, got %q", v)
	}
	if v := readVersion(write("v4.py", "other = 1\n")); v != "0.0.0-go" {
		t.Fatalf("无版本行应回退, got %q", v)
	}
	if v := readVersion(filepath.Join(dir, "missing.py")); v != "0.0.0-go" {
		t.Fatalf("文件缺失应回退, got %q", v)
	}
}
