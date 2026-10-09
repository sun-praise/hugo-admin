package plugin

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// manifestFixture 写一个合法的插件目录。
func manifestFixture(t *testing.T, dir, toml string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestParseManifestValid(t *testing.T) {
	dir := manifestFixture(t, filepath.Join(t.TempDir(), "demo"), `
[plugin]
name = "demo"
version = "1.2.0"
entry = "bin/demo"
author = "作者"
description = "描述"

[capabilities]
image_upload = true
tts_generation = false

[build]
platform = "linux"

[config_schema]
schema = '{"type":"object","properties":{"api_key":{"type":"string"}}}'
`)
	m, err := ParseManifest(dir)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Name != "demo" || m.Version != "1.2.0" || m.Entry != "bin/demo" {
		t.Fatalf("manifest = %#v", m)
	}
	if len(m.Capabilities) != 1 || m.Capabilities[0] != "image_upload" {
		t.Fatalf("capabilities = %#v", m.Capabilities)
	}
	if m.Platform != "linux" {
		t.Fatalf("platform = %q", m.Platform)
	}
	if m.ConfigSchema["type"] != "object" {
		t.Fatalf("config_schema = %#v", m.ConfigSchema)
	}
}

func TestParseManifestInvalid(t *testing.T) {
	base := t.TempDir()
	cases := map[string]string{
		"缺 name":    "[plugin]\nversion = \"1\"\nentry = \"x\"\n[capabilities]\na = true\n",
		"缺能力":       "[plugin]\nname = \"x\"\nversion = \"1\"\nentry = \"e\"\n[capabilities]\na = false\n",
		"逃逸":        "[plugin]\nname = \"x\"\nversion = \"1\"\nentry = \"../../escape\"\n[capabilities]\na = true\n",
		"坏 schema":  "[plugin]\nname = \"x\"\nversion = \"1\"\nentry = \"e\"\n[capabilities]\na = true\n[config_schema]\nschema = '{broken'\n",
		"无 section": "name = \"x\"\n",
	}
	for name, toml := range cases {
		dir := manifestFixture(t, filepath.Join(base, name), toml)
		if _, err := ParseManifest(dir); err == nil {
			t.Errorf("%s: 应报错", name)
		}
	}
}

func TestResolveEntryPath(t *testing.T) {
	dir := manifestFixture(t, filepath.Join(t.TempDir(), "demo"), `
[plugin]
name = "demo"
version = "1"
entry = "bin/run"
[capabilities]
a = true
`)
	binDir := filepath.Join(dir, "bin")
	os.MkdirAll(binDir, 0o755)
	bin := filepath.Join(binDir, "run")
	os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o644)

	m, _ := ParseManifest(dir)
	if _, err := m.ResolveEntryPath(); err == nil {
		t.Fatal("不可执行应报错")
	}
	os.Chmod(bin, 0o755)
	path, err := m.ResolveEntryPath()
	if err != nil || path != bin {
		t.Fatalf("resolve = %q %v", path, err)
	}
}

// newTestPluginManager 构造基于 temp 目录的 manager 并注册 testplugin 二进制。
func newTestPluginManager(t *testing.T) *Manager {
	t.Helper()
	base := t.TempDir()
	pluginDir := filepath.Join(base, "plugins", "test-plugin")
	manifestFixture(t, pluginDir, `
[plugin]
name = "test-plugin"
version = "1.0.0"
entry = "testplugin"
description = "测试插件"
[capabilities]
image_upload = true
tts_generation = true
`)
	// 构建真插件二进制（cwd = internal/plugin）
	bin := filepath.Join(pluginDir, "testplugin")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/testplugin")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("构建测试插件失败: %v\n%s", err, out)
	}
	os.Chmod(bin, 0o755)
	return NewManager(base)
}

func TestManagerLifecycle(t *testing.T) {
	m := newTestPluginManager(t)
	defer m.StopAll()

	m.StartAll()
	plugins := m.ListPlugins()
	if len(plugins) != 1 {
		t.Fatalf("plugins = %#v", plugins)
	}
	p := plugins[0]
	if p["name"] != "test-plugin" || p["status"] != "running" || p["enabled"] != true {
		t.Fatalf("plugin = %#v", p)
	}
	caps := p["capabilities"].([]string)
	if len(caps) != 2 {
		t.Fatalf("capabilities = %#v", caps)
	}

	// 能力发现
	target := m.FindPluginWithCapability("image_upload")
	if target == nil || target["name"] != "test-plugin" {
		t.Fatalf("find capability = %#v", target)
	}
	if m.FindPluginWithCapability("nope") != nil {
		t.Fatal("不存在的能力应返回 nil")
	}

	// 配置：保存 + 解密往返 + has_config
	if !m.SetConfig("test-plugin", map[string]any{"api_key": "secret-值", "retries": 3}) {
		t.Fatal("SetConfig 失败")
	}
	got := m.GetConfig("test-plugin")
	if got["api_key"] != "secret-值" || got["retries"] != 3 {
		t.Fatalf("config 往返 = %#v", got)
	}

	// config-schema（gRPC 优先）
	schema := m.GetConfigSchema("test-plugin")
	if schema["type"] != "object" {
		t.Fatalf("schema = %#v", schema)
	}

	// disable → stopped；enable → running
	if !m.DisablePlugin("test-plugin") {
		t.Fatal("disable 失败")
	}
	if p := m.ListPlugins()[0]; p["status"] != "stopped" || p["enabled"] != false {
		t.Fatalf("disabled = %#v", p)
	}
	if !m.EnablePlugin("test-plugin") {
		t.Fatal("enable 失败")
	}
	if p := m.ListPlugins()[0]; p["status"] != "running" {
		t.Fatalf("enabled = %#v", p)
	}
}

func TestConfigStoreFernetRoundTrip(t *testing.T) {
	base := t.TempDir()
	store, err := NewConfigStore(filepath.Join(base, "config.json"), filepath.Join(base, ".secret_key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetConfig("p", map[string]any{"k": "v1"}); err != nil {
		t.Fatal(err)
	}
	// 重新加载（磁盘往返 + 密钥复用）
	store2, err := NewConfigStore(filepath.Join(base, "config.json"), filepath.Join(base, ".secret_key"))
	if err != nil {
		t.Fatal(err)
	}
	if got := store2.GetConfig("p")["k"]; got != "v1" {
		t.Fatalf("往返 = %#v", got)
	}
}
