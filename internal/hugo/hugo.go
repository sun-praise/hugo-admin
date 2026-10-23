// Package hugo 管理 Hugo 开发服务器进程：启动/停止/状态/日志。
// 行为对齐 services/hugo_service.py；日志推送用 SSE broker 替代
// socketio.emit("server_log")。进程指标用 /proc 替代 psutil（Linux）。
package hugo

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sun-praise/hugo-admin/v3/internal/realtime"
)

const (
	defaultServerURL = "http://0.0.0.0:1313"
	maxLogs          = 1000
)

// previewServer 是主题预览用的临时 Hugo 实例：独立端口、独立生命周期，
// 与主服务器（m.cmd）互不影响，用于切换主题前并排预览效果。
type previewServer struct {
	cmd    *exec.Cmd
	exited chan struct{}
	port   int
	url    string
	theme  string
	logs   []map[string]any
}

type Manager struct {
	hugoRoot  string
	serverURL string
	broker    *realtime.Broker
	// command 供测试注入假 hugo 可执行文件
	command string

	mu        sync.Mutex
	cmd       *exec.Cmd
	pid       int
	isRunning bool
	startedAt time.Time
	logs      []map[string]any
	// exited 在 monitorLogs 完成收尸（cmd.Wait 返回）后关闭，
	// 供 Stop 等待进程退出、processAlive 判断进程已死。访问需持有 m.mu。
	exited chan struct{}

	// preview 主题预览实例（nil = 未运行）。访问需持有 m.mu；
	// previewMu 串行化 StartPreview/StopPreview，避免并发起停互相踩踏。
	preview   *previewServer
	previewMu sync.Mutex
}

func NewManager(hugoRoot, serverURL string, broker *realtime.Broker) *Manager {
	if serverURL == "" {
		serverURL = defaultServerURL
	}
	return &Manager{hugoRoot: hugoRoot, serverURL: serverURL, broker: broker, command: "hugo"}
}

// Start 对齐 start()：已在运行则拒绝；命令不存在给出安装提示；
// --disableFastRender -D（默认渲染草稿）与主题（override > HUGO_THEME）。
// 注：持久化主题设置（settings_service）待 settings 批次接入。
func (m *Manager) Start(debug bool, themeOverride string) (bool, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning && m.processAlive() {
		return false, "Hugo 服务器已经在运行中"
	}

	bin := m.command
	if _, err := exec.LookPath(bin); err != nil {
		return false, "未找到 hugo 命令，请确保 Hugo 已安装"
	}

	args := []string{"server", "--bind=0.0.0.0", "-b", m.serverURL, "--disableFastRender", "-D"}
	theme := themeOverride
	if theme == "" {
		theme = os.Getenv("HUGO_THEME")
	}
	if theme != "" {
		args = append(args, "--theme", theme)
	}

	cmd := exec.Command(bin, args...)
	cmd.Dir = m.hugoRoot
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false, fmt.Sprintf("启动失败: %v", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return false, fmt.Sprintf("启动失败: %v", err)
	}

	m.cmd = cmd
	m.pid = cmd.Process.Pid
	m.isRunning = true
	m.startedAt = time.Now()
	m.logs = nil
	m.addLog(fmt.Sprintf("Hugo 服务器已启动 (PID: %d)", m.pid), "SUCCESS")

	m.exited = make(chan struct{})
	// cmd/exited 以参数传入（Start 持锁期间创建），monitorLogs 不跨 goroutine 读 m.cmd
	go m.monitorLogs(stdout, cmd, m.exited)
	return true, fmt.Sprintf("Hugo 服务器已启动 (PID: %d)", m.pid)
}

// Stop 对齐 stop()：SIGTERM 优雅停止，5 秒后 SIGKILL。
func (m *Manager) Stop() (bool, string) {
	m.mu.Lock()
	if !m.isRunning || m.cmd == nil {
		m.mu.Unlock()
		return false, "Hugo 服务器未运行"
	}
	cmd := m.cmd
	exited := m.exited
	m.mu.Unlock()

	// monitorLogs 负责唯一的 cmd.Wait()（stdout EOF 后收尸），
	// 这里经 exited 等待退出：SIGTERM 优雅停止，5 秒后 SIGKILL。
	// 等待期间不得持有 m.mu：monitorLogs 要先拿锁写日志、收尸后才能关 exited，
	// 持锁等待会与之互相等待而死锁。
	if cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// 等待期间可能已被并发处理（自动退出纠正、重复 Stop、重启了新实例），
	// 只清理仍属于本次 Stop 的那台服务器
	if m.cmd != cmd {
		return true, "Hugo 服务器已停止"
	}
	m.addLog("Hugo 服务器已停止", "INFO")
	m.isRunning = false
	m.cmd = nil
	m.pid = 0
	return true, "Hugo 服务器已停止"
}

// Status 对齐 get_status()：进程已死时自动纠正为未运行。
func (m *Manager) Status() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.isRunning && !m.processAlive() {
		m.isRunning = false
		m.cmd = nil
		m.pid = 0
	}

	status := map[string]any{
		"running":     m.isRunning,
		"pid":         nil,
		"uptime":      nil,
		"cpu_percent": nil,
		"memory_mb":   nil,
	}
	if m.isRunning && m.pid != 0 {
		status["pid"] = m.pid
		status["uptime"] = formatUptime(time.Since(m.startedAt))
		if mb, ok := procMemoryMB(m.pid); ok {
			status["memory_mb"] = mb
		}
	}
	return status
}

// RecentLogs 对齐 get_recent_logs()。
func (m *Manager) RecentLogs(count int) []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if count <= 0 || count > len(m.logs) {
		count = len(m.logs)
	}
	out := make([]map[string]any, count)
	copy(out, m.logs[len(m.logs)-count:])
	return out
}

func (m *Manager) processAlive() bool {
	if m.cmd == nil || m.cmd.Process == nil || m.pid == 0 {
		return false
	}
	// exited 已关闭 → monitorLogs 已 Wait 收尸，进程必定退出
	// （不再裸读 cmd.ProcessState：它与 Wait 的写入无同步）
	select {
	case <-m.exited:
		return false
	default:
	}
	return syscall.Kill(m.pid, 0) == nil
}

// monitorLogs 消费 hugo stdout 追加日志；stdout EOF 即进程退出：
// Wait 回收僵尸并关闭 exited，下一次 Status/Start 据此自动纠正为未运行。
func (m *Manager) monitorLogs(stdout io.Reader, cmd *exec.Cmd, exited chan struct{}) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			m.mu.Lock()
			m.addLog(line, "INFO")
			m.mu.Unlock()
		}
	}
	_ = cmd.Wait()
	close(exited)
	m.mu.Lock()
	defer m.mu.Unlock()
	// 仅当仍是本次启动的进程时才翻转状态：Stop→Start 期间旧 goroutine
	// 迟到的收尸不得把新实例的 isRunning 误清（Status 误报/重复拉起）。
	if m.cmd == cmd && m.isRunning {
		m.isRunning = false
	}
}

// addLog 追加日志（内存上限 1000 条）并经 SSE 推送 server_log 事件。
// 调用方需持有 m.mu。
func (m *Manager) addLog(message, level string) {
	entry := map[string]any{
		"timestamp": time.Now().Format("15:04:05"),
		"level":     level,
		"message":   message,
	}
	m.logs = append(m.logs, entry)
	if len(m.logs) > maxLogs {
		m.logs = m.logs[len(m.logs)-maxLogs:]
	}
	if m.broker != nil {
		m.broker.Broadcast("server_log", entry)
	}
}

func formatUptime(d time.Duration) string {
	total := int(d.Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// procMemoryMB 从 /proc/<pid>/status 读 VmRSS（psutil 的 Linux 替代）。
func procMemoryMB(pid int) (float64, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
			fields := strings.Fields(v)
			if len(fields) >= 1 {
				if kb, err := strconv.ParseFloat(fields[0], 64); err == nil {
					round := float64(int(kb/1024*100)) / 100
					return round, true
				}
			}
		}
	}
	return 0, false
}

// ServerURL 返回当前 Hugo 预览服务器基础 URL。
func (m *Manager) ServerURL() string { return m.serverURL }

// SetServerURL 更新预览服务器基础 URL（设置页保存后生效）。

// ============ 主题预览实例（独立端口，不影响主服务器） ============

// StartPreview 启动主题预览服务器：自动挑选空闲端口，绑定 --theme 启动
// 临时实例；已有预览实例会先停止（切换预览主题）。返回可访问的 URL。
// 主服务器与预览实例互不影响。
func (m *Manager) StartPreview(theme string) (string, bool, string) {
	m.previewMu.Lock()
	defer m.previewMu.Unlock()

	if theme == "" {
		return "", false, "主题名称不能为空"
	}
	bin := m.command
	if _, err := exec.LookPath(bin); err != nil {
		return "", false, "未找到 hugo 命令，请确保 Hugo 已安装"
	}

	// 先摘下旧实例再等待退出：等待不能持有 m.mu（monitorPreviewLogs 要拿锁写日志）
	m.mu.Lock()
	old := m.preview
	m.preview = nil
	m.mu.Unlock()
	if old != nil {
		m.terminatePreview(old)
	}

	port, err := pickFreePort()
	if err != nil {
		return "", false, fmt.Sprintf("分配预览端口失败: %v", err)
	}
	baseURL := previewBaseURL(m.serverURL, port)

	args := []string{
		"server", "--bind=0.0.0.0", "-b", baseURL,
		"-p", strconv.Itoa(port), "--disableFastRender", "-D",
		"--theme", theme,
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = m.hugoRoot
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", false, fmt.Sprintf("启动失败: %v", err)
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		return "", false, fmt.Sprintf("启动失败: %v", err)
	}

	p := &previewServer{
		cmd:    cmd,
		exited: make(chan struct{}),
		port:   port,
		url:    baseURL,
		theme:  theme,
	}
	m.mu.Lock()
	m.preview = p
	m.mu.Unlock()
	go m.monitorPreviewLogs(stdout, p)
	return baseURL, true, "预览服务器已启动"
}

// StopPreview 停止主题预览服务器（主服务器不受影响）。
func (m *Manager) StopPreview() (bool, string) {
	m.previewMu.Lock()
	defer m.previewMu.Unlock()

	m.mu.Lock()
	p := m.preview
	m.preview = nil
	m.mu.Unlock()
	if p == nil {
		return false, "预览服务器未运行"
	}
	m.terminatePreview(p)
	return true, "预览服务器已停止"
}

// PreviewStatus 返回预览实例状态；进程已死时自动纠正为未运行。
func (m *Manager) PreviewStatus() map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()

	p := m.preview
	if p != nil {
		select {
		case <-p.exited:
			m.preview = nil
			p = nil
		default:
		}
	}
	status := map[string]any{
		"running": false, "theme": nil, "url": nil, "port": nil, "pid": nil,
	}
	if p != nil {
		status["running"] = true
		status["theme"] = p.theme
		status["url"] = p.url
		status["port"] = p.port
		status["pid"] = p.cmd.Process.Pid
	}
	return status
}

// terminatePreview 对预览进程 SIGTERM 优雅停止，5 秒后 SIGKILL。
// 不得持有 m.mu 调用：monitorPreviewLogs 需要拿锁写日志、收尸后关闭 exited。
func (m *Manager) terminatePreview(p *previewServer) {
	if p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.exited:
	case <-time.After(5 * time.Second):
		_ = p.cmd.Process.Kill()
		<-p.exited
	}
}

// monitorPreviewLogs 消费预览实例 stdout（仅内存缓冲，不进主日志 SSE 流）；
// stdout EOF 即进程退出：Wait 收尸并关闭 exited，PreviewStatus 据此自动纠正。
func (m *Manager) monitorPreviewLogs(stdout io.Reader, p *previewServer) {
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		m.mu.Lock()
		p.logs = append(p.logs, map[string]any{
			"timestamp": time.Now().Format("15:04:05"),
			"level":     "INFO",
			"message":   line,
		})
		if len(p.logs) > maxLogs {
			p.logs = p.logs[len(p.logs)-maxLogs:]
		}
		m.mu.Unlock()
	}
	_ = p.cmd.Wait()
	close(p.exited)
	m.mu.Lock()
	defer m.mu.Unlock()
	// 仅当仍是本实例时才清空：切换预览期间旧 goroutine 迟到的收尸
	// 不得把新实例误清
	if m.preview == p {
		m.preview = nil
	}
}

// pickFreePort 向内核要一个空闲 TCP 端口（关闭监听后归还，存在理论竞态，
// 对开发预览场景可接受）。
func pickFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// previewBaseURL 由主服务器 URL 的 host 拼出预览实例 baseURL（换端口不换主机），
// 保持与用户配置的访问地址一致（如内网 IP）。
func previewBaseURL(serverURL string, port int) string {
	host := "0.0.0.0"
	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		host = u.Hostname()
	}
	return fmt.Sprintf("http://%s:%d", host, port)
}

// SetServerURL 更新预览服务器基础 URL（设置页保存后生效）。
func (m *Manager) SetServerURL(url string) { m.serverURL = url }
