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
	"path/filepath"
	"regexp"
	"sort"
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
	// ready 在 Hugo 打印「Web Server is available」后关闭：真实就绪的唯一
	// 信号（cmd.Start() 只代表进程已 fork，端口被抢/主题编译失败时进程随后
	// 立刻退出，此时起接口若已返回 ok，前端会挂一个空 iframe）。
	ready chan struct{}
	port  int
	url   string
	theme string
	logs  []map[string]any
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
// 只停主实例：预览是独立的临时实例，「停止主服务器」不应连带杀掉用户
// 正在对比的预览；进程退出时的统一回收见 cmd/hugo-admin/main.go。
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

// ============ 主题预览实例（独立端口，不影响主服务器） ============

// StartPreview 启动主题预览服务器：自动挑选空闲端口，在独立覆盖站点
// （.admin/preview/site）中以指定主题渲染。不直接复用站点原配置——module/
// vendor/主题专属配置会压住 --theme 或与目标主题冲突（如内容 frontmatter
// 打崩其他主题的模板）；覆盖站点只采样最新若干篇、剥成最小 frontmatter，
// 任何主题都能干净渲染。已有预览实例会先停止（切换预览主题）。
func (m *Manager) StartPreview(theme string) (string, bool, string) {
	m.previewMu.Lock()
	defer m.previewMu.Unlock()

	if theme == "" {
		return "", false, "主题名称不能为空"
	}
	if !previewThemeNameRE.MatchString(theme) {
		return "", false, fmt.Sprintf("主题名称不合法: %s", theme)
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

	siteDir, err := m.preparePreviewSite(theme, baseURL)
	if err != nil {
		return "", false, fmt.Sprintf("准备预览站点失败: %v", err)
	}

	args := []string{
		"server", "--bind=0.0.0.0", "-b", baseURL,
		"-p", strconv.Itoa(port), "--disableFastRender", "-D",
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = siteDir
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
		ready:  make(chan struct{}),
		port:   port,
		url:    baseURL,
		theme:  theme,
	}
	m.mu.Lock()
	m.preview = p
	m.mu.Unlock()
	go m.monitorPreviewLogs(stdout, p)

	// 等 Hugo 真正就绪再返回：仅 Start() 成功不足以说明站点可访问。
	// 提前退出/超时都把实例回收掉，并把日志尾部交给调用方做诊断。
	select {
	case <-p.ready:
		return baseURL, true, "预览服务器已启动"
	case <-p.exited:
		return "", false, m.previewFailMessage(p, "预览进程在就绪前退出")
	case <-time.After(previewReadyTimeout):
		m.terminatePreview(p)
		return "", false, m.previewFailMessage(p, "预览服务器未在预期时间内就绪")
	}
}

// previewReadyTimeout 等待预览实例就绪的上限：首次渲染要编译主题 SCSS
// （LoveIt/DoIt/Stack），留足余量。
const previewReadyTimeout = 30 * time.Second

// previewFailMessage 拼装启动失败信息：原因 + 日志尾部（Hugo 的
// TOCSS-DART、端口占用等都只在 stdout 里有价值）。
func (m *Manager) previewFailMessage(p *previewServer, reason string) string {
	tail := m.previewLogTail(p, 5)
	if tail == "" {
		return "预览服务器启动失败: " + reason
	}
	return fmt.Sprintf("预览服务器启动失败: %s；日志末尾：%s", reason, tail)
}

// previewLogTail 取预览实例最近 n 行日志（不足则全取）。
func (m *Manager) previewLogTail(p *previewServer, n int) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	start := 0
	if len(p.logs) > n {
		start = len(p.logs) - n
	}
	lines := make([]string, 0, len(p.logs)-start)
	for _, l := range p.logs[start:] {
		if msg, ok := l["message"].(string); ok {
			lines = append(lines, msg)
		}
	}
	return strings.Join(lines, " | ")
}

var previewThemeNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

// previewSampleCount 预览采样的文章数：够看出主题列表页/文章页效果即可。
const previewSampleCount = 8

// preparePreviewSite 在 <hugoRoot>/.admin/preview/site 生成独立覆盖站点：
// hugo.toml 只声明预览主题与通用兼容项；themes 符号链接到站点 themes/；
// content 为最新若干篇的采样副本（仅保留标题/日期，剥离主题专属
// frontmatter）；采样正文里用到的 shortcode 生成空桩，未知 shortcode
// 渲染为空而不是构建失败。
func (m *Manager) preparePreviewSite(theme, baseURL string) (string, error) {
	siteDir := filepath.Join(m.hugoRoot, ".admin", "preview", "site")
	if err := os.RemoveAll(siteDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(siteDir, "layouts", "_shortcodes"), 0o755); err != nil {
		return "", err
	}
	// 放进 post/ 分区：主题（如 PaperMod）按 mainSections/Type 过滤列表页，
	// 根级页面的 Type 是 page 不会出现在首页列表
	if err := os.MkdirAll(filepath.Join(siteDir, "content", "post"), 0o755); err != nil {
		return "", err
	}
	// 注意：Hugo 没有能忽略 missing shortcode 的配置项（0.163.3 实测
	// ignoreErrors/ignoreLogs 均不被识别，缺失 shortcode 直接构建失败），
	// 唯一解法是下方为采样正文的每个 shortcode 生成空桩布局。
	cfg := fmt.Sprintf(`baseURL = '%s'
title = '主题预览'
theme = '%s'
defaultContentLanguage = 'zh-cn'

[security]
  allowContent = ['.*']

[markup.goldmark.renderer]
  unsafe = true
`, baseURL, theme)
	if err := os.WriteFile(filepath.Join(siteDir, "hugo.toml"), []byte(cfg), 0o644); err != nil {
		return "", err
	}
	// 预览前置校验（ThemeExists）保证 themes/<theme> 存在
	if err := os.Symlink(filepath.Join("..", "..", "..", "themes"), filepath.Join(siteDir, "themes")); err != nil {
		return "", err
	}
	return siteDir, m.writePreviewSample(siteDir)
}

// writePreviewSample 采样 content/ 最新若干篇 .md，剥 frontmatter 只留
// 标题/日期；正文中的 shortcode 名生成空桩布局。
func (m *Manager) writePreviewSample(siteDir string) error {
	type sample struct {
		path    string
		modTime time.Time
	}
	var samples []sample
	_ = filepath.WalkDir(filepath.Join(m.hugoRoot, "content"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		base := filepath.Base(path)
		if base == "_index.md" || strings.HasPrefix(base, "_index.") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		samples = append(samples, sample{path, info.ModTime()})
		return nil
	})
	sort.Slice(samples, func(i, j int) bool { return samples[i].modTime.After(samples[j].modTime) })
	if len(samples) > previewSampleCount {
		samples = samples[:previewSampleCount]
	}

	shortcodes := map[string]bool{}
	for i, s := range samples {
		raw, err := os.ReadFile(s.path)
		if err != nil {
			continue
		}
		title, body := splitFrontMatter(string(raw))
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(s.path), ".md")
		}
		doc := fmt.Sprintf("+++\ntitle = %q\ndate = %q\ndraft = false\n+++\n\n%s",
			title, s.modTime.Format("2006-01-02T15:04:05"), body)
		name := filepath.Join(siteDir, "content", "post", fmt.Sprintf("post-%02d.md", i+1))
		if err := os.WriteFile(name, []byte(doc), 0o644); err != nil {
			return err
		}
		for _, sc := range shortcodeNames(body) {
			shortcodes[sc] = true
		}
	}
	for sc := range shortcodes {
		stub := filepath.Join(siteDir, "layouts", "_shortcodes", sc+".html")
		if err := os.WriteFile(stub, nil, 0o644); err != nil {
			return err
		}
	}
	return nil
}

var titleLineRE = regexp.MustCompile(`(?m)^\s*title\s*[:=]\s*(.+)$`)

// splitFrontMatter 剥离 ---/+++ 围栏的前置元数据，返回其中的 title 与正文；
// 无围栏时全文即正文。
func splitFrontMatter(raw string) (title, body string) {
	body = raw
	nl := strings.IndexByte(raw, '\n')
	if nl <= 0 {
		return "", body
	}
	fence := strings.TrimSpace(raw[:nl])
	if fence != "---" && fence != "+++" {
		return "", body
	}
	closer := "\n" + fence
	rest := raw[nl+1:]
	end := strings.Index(rest, closer)
	if end < 0 {
		return "", body
	}
	fm := rest[:end]
	body = strings.TrimLeft(rest[end+len(closer):], "\r\n")
	if m := titleLineRE.FindStringSubmatch(fm); m != nil {
		title = strings.Trim(strings.TrimSpace(m[1]), `"'`)
	}
	return title, body
}

var shortcodeRE = regexp.MustCompile(`\{\{[<%]\s*([A-Za-z0-9_-]+)`)

// shortcodeNames 提取正文中用到的 shortcode 名（去重，保持首现顺序）。
func shortcodeNames(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range shortcodeRE.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
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
// 见到就绪行关闭 ready；stdout EOF 即进程退出：Wait 收尸并关闭 exited，
// PreviewStatus 据此自动纠正。
func (m *Manager) monitorPreviewLogs(stdout io.Reader, p *previewServer) {
	scanner := bufio.NewScanner(stdout)
	var readyOnce sync.Once
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// 就绪行形如 "Web Server is available at http://localhost:1313/
		// (bind address 0.0.0.0)"，是 Hugo 唯一可靠的「可访问」信号
		if strings.Contains(line, "Web Server is available") {
			readyOnce.Do(func() { close(p.ready) })
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
