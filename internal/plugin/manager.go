package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/svtter/hugo-admin/proto"
)

const (
	handshakeTimeout = 10 * time.Second
	shutdownTimeout  = 5 * time.Second
	marketURL        = "https://plugins.hugo-admin.dev/manifest.json"
	marketCacheTTL   = 5 * time.Minute
)

// pluginState 对齐 PluginState。
type pluginState struct {
	manifest *Manifest
	process  *exec.Cmd
	port     int
	conn     *grpc.ClientConn
	stub     pb.PluginServiceClient
	enabled  bool
	status   string // stopped | running | error
}

// Manager 管理插件生命周期。baseDir 对齐 ~/.hugo-admin（可注入便于测试）。
type Manager struct {
	baseDir string

	mu      sync.Mutex
	plugins map[string]*pluginState
	config  *ConfigStore

	marketCache     map[string]any
	marketCacheTime time.Time
}

func NewManager(baseDir string) *Manager {
	pluginDir := filepath.Join(baseDir, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		fmt.Printf("创建插件目录失败: %v\n", err)
	}
	store, err := NewConfigStore(filepath.Join(baseDir, "plugin-config.json"), filepath.Join(baseDir, ".secret_key"))
	if err != nil {
		fmt.Printf("插件配置存储初始化失败: %v\n", err)
	}
	return &Manager{baseDir: baseDir, plugins: map[string]*pluginState{}, config: store}
}

// DefaultBaseDir 是生产路径 ~/.hugo-admin。
func DefaultBaseDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".hugo-admin"
	}
	return filepath.Join(home, ".hugo-admin")
}

// DiscoverPlugins 对齐 discover_plugins：扫描 <base>/plugins/*/plugin.toml。
func (m *Manager) DiscoverPlugins() []*Manifest {
	pluginDir := filepath.Join(m.baseDir, "plugins")
	_ = os.MkdirAll(pluginDir, 0o755)
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var manifests []*Manifest
	for _, name := range names {
		dir := filepath.Join(pluginDir, name)
		manifest, err := ParseManifest(dir)
		if err != nil {
			fmt.Printf("Skipping invalid plugin in %s: %v\n", dir, err)
			continue
		}
		manifests = append(manifests, manifest)
	}
	return manifests
}

// StartAll 发现并启动全部插件。
func (m *Manager) StartAll() {
	for _, manifest := range m.DiscoverPlugins() {
		if err := m.startPlugin(manifest); err != nil {
			fmt.Printf("Failed to start plugin %s: %v\n", manifest.Name, err)
		}
	}
}

// StopAll 优雅停止全部插件。
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, st := range m.plugins {
		if st.enabled && st.process != nil {
			m.stopPlugin(name, st)
		}
	}
}

func (m *Manager) startPlugin(manifest *Manifest) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if st, ok := m.plugins[manifest.Name]; ok && st.enabled {
		return fmt.Errorf("plugin %s is already running", manifest.Name)
	}

	entryPath, err := manifest.ResolveEntryPath()
	if err != nil {
		return err
	}

	// 平台兼容
	if manifest.Platform != "" {
		expected := runtime.GOOS
		if expected == "windows" {
			expected = "windows"
		}
		if manifest.Platform != expected {
			return fmt.Errorf("plugin %s built for '%s', running on '%s' — skipping",
				manifest.Name, manifest.Platform, expected)
		}
	}

	// 动态端口
	sock, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port := sock.Addr().(*net.TCPAddr).Port
	sock.Close()

	cmd := exec.Command(entryPath, "--port", fmt.Sprintf("%d", port))
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return err
	}

	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	stub := pb.NewPluginServiceClient(conn)

	// 健康检查握手（对齐：10s 超时，0.2s 重试）
	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()
	healthy := false
	for {
		checkCtx, checkCancel := context.WithTimeout(ctx, time.Second)
		resp, err := stub.HealthCheck(checkCtx, &pb.Empty{})
		checkCancel()
		if err == nil && resp.Healthy {
			healthy = true
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(200 * time.Millisecond):
		}
		if ctx.Err() != nil {
			break
		}
	}
	if !healthy {
		_ = cmd.Process.Kill()
		conn.Close()
		return fmt.Errorf("plugin %s failed health check within %vs", manifest.Name, handshakeTimeout.Seconds())
	}

	if _, err := stub.Info(context.Background(), &pb.Empty{}); err != nil {
		_ = cmd.Process.Kill()
		conn.Close()
		return fmt.Errorf("plugin %s Info failed: %v", manifest.Name, err)
	}

	// 恢复已存配置
	if saved := m.config.GetConfig(manifest.Name); len(saved) > 0 {
		configJSON, _ := json.Marshal(saved)
		if _, err := stub.SetConfig(context.Background(), &pb.SetConfigRequest{ConfigJson: string(configJSON)}); err != nil {
			fmt.Printf("Failed to push config to %s: %v\n", manifest.Name, err)
		}
	}

	m.plugins[manifest.Name] = &pluginState{
		manifest: manifest, process: cmd, port: port,
		conn: conn, stub: stub, enabled: true, status: "running",
	}
	fmt.Printf("Plugin %s v%s started on port %d (capabilities: %v)\n",
		manifest.Name, manifest.Version, port, manifest.Capabilities)
	return nil
}

func (m *Manager) stopPlugin(name string, st *pluginState) {
	if st.process == nil || st.process.Process == nil {
		return
	}
	if st.process.ProcessState != nil { // 已退出
	} else {
		_ = st.process.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = st.process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(shutdownTimeout):
			_ = st.process.Process.Kill()
			<-done
		}
	}
	if st.conn != nil {
		st.conn.Close()
	}
	st.enabled = false
	st.status = "stopped"
	st.process = nil
	st.conn = nil
	st.stub = nil
}

// EnablePlugin 启动已停止的插件。
func (m *Manager) EnablePlugin(name string) bool {
	st := m.getPlugin(name)
	if st == nil {
		return false
	}
	if st.enabled {
		return true
	}
	if err := m.startPlugin(st.manifest); err != nil {
		fmt.Printf("Failed to enable plugin %s: %v\n", name, err)
		return false
	}
	return true
}

// DisablePlugin 停止插件（保留配置）。
func (m *Manager) DisablePlugin(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.plugins[name]
	if st == nil {
		return false
	}
	if !st.enabled {
		return true
	}
	m.stopPlugin(name, st)
	return true
}

func (m *Manager) getPlugin(name string) *pluginState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plugins[name]
}

// Conn 返回运行中插件的 gRPC 连接（能力 stub 的基础）。
func (m *Manager) Conn(name string) *grpc.ClientConn {
	st := m.getPlugin(name)
	if st != nil && st.enabled && st.conn != nil {
		return st.conn
	}
	return nil
}

// Stub 返回运行中插件的 PluginService 客户端。
func (m *Manager) Stub(name string) pb.PluginServiceClient {
	st := m.getPlugin(name)
	if st != nil && st.enabled && st.stub != nil {
		return st.stub
	}
	return nil
}

// FindPluginWithCapability 对齐 find_plugin_with_capability。
func (m *Manager) FindPluginWithCapability(capability string) map[string]any {
	for _, info := range m.ListPlugins() {
		caps, _ := info["capabilities"].([]string)
		for _, c := range caps {
			if c == capability && info["status"] == "running" && info["enabled"] == true {
				return info
			}
		}
	}
	return nil
}

// GetConfig / SetConfigSchema 转发配置存储。
func (m *Manager) GetConfig(name string) map[string]any { return m.config.GetConfig(name) }

// SetConfig 保存配置并推送到运行中的插件。
func (m *Manager) SetConfig(name string, config map[string]any) bool {
	if err := m.config.SetConfig(name, config); err != nil {
		fmt.Printf("保存插件配置失败: %v\n", err)
		return false
	}
	stub := m.Stub(name)
	if stub == nil {
		return true // 已保存，插件未运行
	}
	configJSON, _ := json.Marshal(config)
	resp, err := stub.SetConfig(context.Background(), &pb.SetConfigRequest{ConfigJson: string(configJSON)})
	if err != nil || !resp.Success {
		return false
	}
	return true
}

// GetConfigSchema 优先 gRPC，回退 manifest。
func (m *Manager) GetConfigSchema(name string) map[string]any {
	if stub := m.Stub(name); stub != nil {
		resp, err := stub.GetConfigSchema(context.Background(), &pb.Empty{})
		if err == nil && resp.SchemaJson != "" {
			var schema map[string]any
			if json.Unmarshal([]byte(resp.SchemaJson), &schema) == nil && schema != nil {
				return schema
			}
		}
	}
	if st := m.getPlugin(name); st != nil {
		return st.manifest.ConfigSchema
	}
	return nil
}

// ListPlugins 对齐 list_plugins。
func (m *Manager) ListPlugins() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []map[string]any{}
	for name, st := range m.plugins {
		result = append(result, map[string]any{
			"name":         name,
			"version":      st.manifest.Version,
			"description":  st.manifest.Description,
			"author":       st.manifest.Author,
			"capabilities": st.manifest.Capabilities,
			"status":       st.status,
			"enabled":      st.enabled,
			"has_config":   len(m.config.GetConfig(name)) > 0,
		})
	}
	// Go map 遍历无序：按名字排序保证稳定输出（Python dict 插入序）
	sort.Slice(result, func(i, j int) bool {
		return result[i]["name"].(string) < result[j]["name"].(string)
	})
	return result
}

// FetchMarket 对齐 fetch_market：TTL 缓存 + 失败回退 stale + 空目录。
func (m *Manager) FetchMarket() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	if m.marketCache != nil && now.Sub(m.marketCacheTime) < marketCacheTTL {
		return m.marketCache
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(marketURL)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var data map[string]any
			if json.NewDecoder(resp.Body).Decode(&data) == nil {
				m.marketCache = data
				m.marketCacheTime = now
				return data
			}
		}
	}
	fmt.Printf("Failed to fetch plugin market: %v\n", err)
	if m.marketCache != nil {
		return m.marketCache
	}
	return map[string]any{"version": float64(1), "plugins": []any{}}
}
