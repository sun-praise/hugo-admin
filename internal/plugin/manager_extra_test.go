package plugin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultBaseDir(t *testing.T) {
	dir := DefaultBaseDir()
	if dir == "" {
		t.Fatal("DefaultBaseDir 不应为空")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("DefaultBaseDir 应为绝对路径: %q", dir)
	}
}

func TestConfigStoreRoundtrip(t *testing.T) {
	base := t.TempDir()
	store, err := NewConfigStore(filepath.Join(base, "plugin-config.json"), filepath.Join(base, ".secret_key"))
	if err != nil {
		t.Fatalf("NewConfigStore: %v", err)
	}
	if err := store.SetConfig("alpha", map[string]any{"k": "v"}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	// 密钥文件应已生成且权限 0600
	info, err := os.Stat(filepath.Join(base, ".secret_key"))
	if err != nil {
		t.Fatalf("密钥文件: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("密钥权限 = %v", info.Mode().Perm())
	}
	// 重新加载（fernet 密钥复用）应能读回配置
	store2, err := NewConfigStore(filepath.Join(base, "plugin-config.json"), filepath.Join(base, ".secret_key"))
	if err != nil {
		t.Fatalf("重载: %v", err)
	}
	cfg := store2.GetConfig("alpha")
	if cfg["k"] != "v" {
		t.Fatalf("GetConfig = %#v", cfg)
	}
}

func TestNewConfigStoreBadPath(t *testing.T) {
	base := t.TempDir()
	// configPath 是目录 → ReadFile 报非 NotExist 错误
	if _, err := NewConfigStore(base, filepath.Join(base, ".secret")); err == nil {
		t.Fatal("非法 configPath 应报错")
	}
}

func TestManagerSetConfigWithoutRunningPlugin(t *testing.T) {
	m := NewManager(t.TempDir())
	// 插件未运行：配置持久化即返回 true
	if !m.SetConfig("ghost", map[string]any{"a": 1}) {
		t.Fatal("未运行插件的 SetConfig 应成功（仅落盘）")
	}
}

func TestFetchMarketOffline(t *testing.T) {
	oldURL := marketURL
	t.Cleanup(func() { marketURL = oldURL })

	// 成功路径：本地服务返回目录数据
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version": 1, "plugins": [{"name": "demo"}]}`)
	}))
	t.Cleanup(srv.Close)
	marketURL = srv.URL

	m := NewManager(t.TempDir())
	catalog := m.FetchMarket()
	plugins, _ := catalog["plugins"].([]any)
	if len(plugins) != 1 {
		t.Fatalf("成功路径 catalog = %#v", catalog)
	}
	// 二次调用命中 TTL 缓存
	m.mu.Lock()
	cached := m.marketCache
	m.mu.Unlock()
	if cached == nil {
		t.Fatal("成功后应有缓存")
	}

	// 失败路径：新 manager + 500 响应 → 回退空目录
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(bad.Close)
	marketURL = bad.URL
	fallback := NewManager(t.TempDir()).FetchMarket()
	if _, ok := fallback["plugins"]; !ok {
		t.Fatalf("回退 catalog 缺 plugins 键: %#v", fallback)
	}
	if plugins, _ := fallback["plugins"].([]any); len(plugins) != 0 {
		t.Fatalf("回退应为空目录: %#v", fallback)
	}
}

func TestManagerConnNilWithoutRunningPlugin(t *testing.T) {
	m := NewManager(t.TempDir())
	if m.Conn("ghost") != nil {
		t.Fatal("未运行插件的 Conn 应为 nil")
	}
	if m.EnablePlugin("ghost") {
		t.Fatal("启用不存在的插件应失败")
	}
}

func TestGetConfigSchemaFallbackToManifest(t *testing.T) {
	m := NewManager(t.TempDir())
	// 插件未运行且不存在 → nil
	if got := m.GetConfigSchema("ghost"); got != nil {
		t.Fatalf("ghost schema = %#v", got)
	}
}

func TestManifestHelpers(t *testing.T) {
	// intField：int64 命中 / 其他类型回退 0
	if got := intField(map[string]any{"k": int64(7)}, "k"); got != 7 {
		t.Fatalf("intField = %d", got)
	}
	if got := intField(map[string]any{"k": "7"}, "k"); got != 0 {
		t.Fatalf("非 int64 应回退 0, got %d", got)
	}

	// ResolveEntryPath：不存在 / 不可执行 / 越界 / 正常
	dir := t.TempDir()
	mf := &Manifest{PluginDir: dir, Entry: "missing"}
	if _, err := mf.ResolveEntryPath(); err == nil {
		t.Fatal("缺失 Entry 应报错")
	}
	bin := filepath.Join(dir, "tool")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mf.Entry = "tool"
	if _, err := mf.ResolveEntryPath(); err == nil {
		t.Fatal("不可执行 Entry 应报错")
	}
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if p, err := mf.ResolveEntryPath(); err != nil || p != bin {
		t.Fatalf("合法 Entry = %q, err = %v", p, err)
	}
	// symlink 越界：目标必须是存在且可执行的文件，才能真正命中目录边界检查
	outside := t.TempDir()
	outsideTool := filepath.Join(outside, "tool")
	if err := os.WriteFile(outsideTool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "evil")
	if err := os.Symlink(outsideTool, link); err != nil {
		t.Fatal(err)
	}
	mf.Entry = "evil"
	if _, err := mf.ResolveEntryPath(); err == nil {
		t.Fatal("symlink 越界应报错")
	}
}

func TestTrimSpaceBytes(t *testing.T) {
	if got := string(trimSpace([]byte(" \n\r\t x \t"))); got != "x" {
		t.Fatalf("trimSpace = %q", got)
	}
	if got := string(trimSpace([]byte(""))); got != "" {
		t.Fatalf("空串 = %q", got)
	}
}

func TestStartAllWithBrokenPlugin(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "plugins", "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// manifest 合法但 Entry 二进制缺失 → 启动失败分支
	if err := os.WriteFile(filepath.Join(dir, "plugin.toml"),
		[]byte("[plugin]\nname = \"broken\"\nversion = \"0.1.0\"\nentry = \"missing-bin\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(base)
	m.StartAll()
	if m.EnablePlugin("broken") {
		t.Fatal("Entry 缺失的插件 enable 应失败")
	}
	// getPlugin 未注册场景（StartAll 内跳过失败项则 plugins 里可能无此名）
	_ = m.GetConfigSchema("broken")
}

func TestConfigStoreSaveFailure(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "plugin-config.json")
	store, err := NewConfigStore(configPath, filepath.Join(base, ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	// 先写一次成功，再把 configPath 换成目录使 save 失败
	if err := store.SetConfig("p", map[string]any{"a": 1}); err != nil {
		t.Fatalf("首次 SetConfig: %v", err)
	}
	if err := os.Remove(configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.SetConfig("p", map[string]any{"a": 2}); err == nil {
		t.Fatal("save 失败应返回错误")
	}
}

func TestSetConfigWithRunningPlugin(t *testing.T) {
	m := newTestPluginManager(t)
	defer m.StopAll()
	m.StartAll()

	// 插件运行中：配置落盘 + gRPC 同步到插件
	if !m.SetConfig("test-plugin", map[string]any{"greeting": "hi"}) {
		t.Fatal("运行中插件的 SetConfig 应成功")
	}
	if cfg := m.GetConfig("test-plugin"); cfg["greeting"] != "hi" {
		t.Fatalf("GetConfig = %#v", cfg)
	}
}

func TestDefaultBaseDirNoHome(t *testing.T) {
	// HOME 缺失时回退相对路径 ".hugo-admin"
	t.Setenv("HOME", "")
	if got := DefaultBaseDir(); got != ".hugo-admin" {
		t.Fatalf("DefaultBaseDir = %q", got)
	}
}

func TestNewManagerWithUnwritableBase(t *testing.T) {
	// baseDir 是普通文件 → plugins 目录创建失败分支（不 panic，继续工作）
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewManager(blocker)
	if m == nil || len(m.DiscoverPlugins()) != 0 {
		t.Fatal("异常 base 下 manager 仍应可用且发现为空")
	}
}
