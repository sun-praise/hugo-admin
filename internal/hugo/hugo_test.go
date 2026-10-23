package hugo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sun-praise/hugo-admin/v3/internal/realtime"
)

// fakeHugo 写一个假 hugo 脚本：输出两行日志后挂住（直到被杀）。
func fakeHugo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "hugo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'Web Server is available at http://localhost:1313/'\necho 'Press Ctrl+C to stop'\nwhile true; do sleep 1; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestStartStatusStop(t *testing.T) {
	m := NewManager(t.TempDir(), "", nil)
	m.command = fakeHugo(t)

	// 未运行
	st := m.Status()
	if st["running"] != false || st["pid"] != nil {
		t.Fatalf("初始状态 = %#v", st)
	}

	ok, msg := m.Start(false, "")
	if !ok || !strings.Contains(msg, "Hugo 服务器已启动") {
		t.Fatalf("start = %v %q", ok, msg)
	}

	// 重复启动被拒
	ok, msg = m.Start(false, "")
	if ok || msg != "Hugo 服务器已经在运行中" {
		t.Fatalf("重复 start = %v %q", ok, msg)
	}

	// 状态：running + pid + uptime
	st = m.Status()
	if st["running"] != true || st["pid"] == nil || st["uptime"] == nil {
		t.Fatalf("运行状态 = %#v", st)
	}

	// 日志（等待监控线程消费 stdout）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.RecentLogs(100)) >= 3 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	logs := m.RecentLogs(100)
	if len(logs) < 3 {
		t.Fatalf("日志不足: %#v", logs)
	}
	if logs[1]["message"] != "Web Server is available at http://localhost:1313/" {
		t.Fatalf("日志内容 = %#v", logs[1])
	}

	ok, msg = m.Stop()
	if !ok || msg != "Hugo 服务器已停止" {
		t.Fatalf("stop = %v %q", ok, msg)
	}
	st = m.Status()
	if st["running"] != false {
		t.Fatalf("停止后状态 = %#v", st)
	}
	// 再次停止被拒
	ok, msg = m.Stop()
	if ok || msg != "Hugo 服务器未运行" {
		t.Fatalf("重复 stop = %v %q", ok, msg)
	}
}

func TestStartCommandMissing(t *testing.T) {
	m := NewManager(t.TempDir(), "", nil)
	m.command = "hugo-definitely-not-exists"
	ok, msg := m.Start(false, "")
	if ok || msg != "未找到 hugo 命令，请确保 Hugo 已安装" {
		t.Fatalf("start = %v %q", ok, msg)
	}
}

func TestLogsBroadcastViaSSE(t *testing.T) {
	broker := realtime.NewBroker()
	ch := broker.Subscribe()
	m := NewManager(t.TempDir(), "", broker)
	m.command = fakeHugo(t)

	if ok, _ := m.Start(false, ""); !ok {
		t.Fatal("start 失败")
	}
	defer m.Stop()

	// 启动日志应经 broker 推送 server_log 事件
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case ev := <-ch:
			if ev.Name == "server_log" && strings.Contains(ev.Data, "已启动") {
				return
			}
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	t.Fatal("未收到 server_log 推送")
}

// TestStopDuringLogStream 回归 #159：Stop 若在等待进程退出期间持有 m.mu，
// 会与 monitorLogs 的日志写入互相等待而死锁（曾以 go test 10m 超时暴露）。
func TestStopDuringLogStream(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hugo")
	// 高频输出：确保 Stop 时 monitorLogs 正活跃在写日志的路径上
	if err := os.WriteFile(script, []byte("#!/bin/sh\ni=0\nwhile true; do echo \"line $i\"; i=$((i+1)); done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), "", nil)
	m.command = script

	if ok, _ := m.Start(false, ""); !ok {
		t.Fatal("start 失败")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.RecentLogs(100)) >= 5 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(m.RecentLogs(100)) < 5 {
		t.Fatal("日志未流入")
	}

	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(8 * time.Second):
		t.Fatal("Stop 死锁：等待进程退出期间不应持有 m.mu")
	}
}

func TestProcessExitAutoCorrectsStatus(t *testing.T) {
	// 假 hugo 立即退出
	dir := t.TempDir()
	script := filepath.Join(dir, "hugo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho started\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), "", nil)
	m.command = script

	if ok, _ := m.Start(false, ""); !ok {
		t.Fatal("start 失败")
	}
	// 进程很快退出（stdout EOF → monitorLogs Wait → isRunning=false）
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.Status()["running"] == false {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("进程退出后状态未纠正: %#v", m.Status())
}

func TestFormatUptime(t *testing.T) {
	cases := map[time.Duration]string{
		59 * time.Second:            "59s",
		61 * time.Second:            "1m 1s",
		3661 * time.Second:          "1h 1m 1s",
		2*time.Hour + 3*time.Minute: "2h 3m 0s",
	}
	for d, want := range cases {
		if got := formatUptime(d); got != want {
			t.Errorf("formatUptime(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestMonitorLogsStaleCmdNotClearState 钉住收尸守卫：Stop→Start 换新实例后，
// 旧进程的 monitorLogs 迟到收尸（m.cmd != cmd）不得清掉新实例的 isRunning。
func TestMonitorLogsStaleCmdNotClearState(t *testing.T) {
	m := NewManager(t.TempDir(), "", nil)
	staleCmd := &exec.Cmd{} // 本轮监控对应的旧进程
	m.cmd = &exec.Cmd{}     // 当前实例已是另一个 cmd
	m.isRunning = true

	exited := make(chan struct{})
	m.monitorLogs(strings.NewReader(""), staleCmd, exited)
	<-exited

	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.isRunning {
		t.Fatal("旧进程的迟到收尸不应清掉新实例的 isRunning")
	}
}

func TestPreviewLifecycle(t *testing.T) {
	m := NewManager(t.TempDir(), "http://0.0.0.0:1313", nil)
	t.Cleanup(func() { m.StopPreview(); m.Stop() })

	// 未运行
	if st := m.PreviewStatus(); st["running"] != false {
		t.Fatalf("初始预览状态 = %#v", st)
	}

	// 参数与命令校验
	if _, ok, msg := m.StartPreview(""); ok || msg != "主题名称不能为空" {
		t.Fatalf("空主题 start = %v %q", ok, msg)
	}
	m.command = "hugo-definitely-not-exists"
	if _, ok, msg := m.StartPreview("demo"); ok || msg != "未找到 hugo 命令，请确保 Hugo 已安装" {
		t.Fatalf("缺命令 start = %v %q", ok, msg)
	}

	m.command = fakeHugo(t)
	url, ok, msg := m.StartPreview("demo")
	if !ok || !strings.Contains(msg, "预览服务器已启动") {
		t.Fatalf("start = %v %q", ok, msg)
	}
	if !strings.HasPrefix(url, "http://0.0.0.0:") {
		t.Fatalf("预览 URL 应复用主配置主机: %q", url)
	}

	st := m.PreviewStatus()
	if st["running"] != true || st["theme"] != "demo" || st["url"] != url || st["port"] == nil {
		t.Fatalf("预览状态 = %#v", st)
	}
	firstPID := st["pid"]

	// 预览不影响主服务器：主服务器仍可启动且互不干扰
	if ok, _ := m.Start(false, ""); !ok {
		t.Fatal("预览运行中主服务器启动失败")
	}
	if m.Status()["running"] != true {
		t.Fatal("主服务器应处于运行状态")
	}

	// 切换预览主题：旧实例被替换
	url2, ok, _ := m.StartPreview("other")
	if !ok {
		t.Fatal("切换预览失败")
	}
	st = m.PreviewStatus()
	if st["theme"] != "other" || st["url"] != url2 || st["pid"] == firstPID {
		t.Fatalf("切换后预览状态 = %#v (旧 pid %v)", st, firstPID)
	}

	// 停止预览：主服务器不受影响；重复停止被拒
	if ok, msg := m.StopPreview(); !ok || msg != "预览服务器已停止" {
		t.Fatalf("stop = %v %q", ok, msg)
	}
	if st := m.PreviewStatus(); st["running"] != false {
		t.Fatalf("停止后预览状态 = %#v", st)
	}
	if m.Status()["running"] != true {
		t.Fatal("停止预览不应影响主服务器")
	}
	if ok, msg := m.StopPreview(); ok || msg != "预览服务器未运行" {
		t.Fatalf("重复 stop = %v %q", ok, msg)
	}
}

func TestPreviewProcessExitAutoCorrectsStatus(t *testing.T) {
	// 假 hugo 立即退出（模拟端口占用/构建失败）
	dir := t.TempDir()
	script := filepath.Join(dir, "hugo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho started\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m := NewManager(t.TempDir(), "", nil)
	m.command = script

	if _, ok, _ := m.StartPreview("demo"); !ok {
		t.Fatal("start 失败")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if m.PreviewStatus()["running"] == false {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("进程退出后预览状态未纠正: %#v", m.PreviewStatus())
}

func TestPreviewBaseURL(t *testing.T) {
	cases := map[string]string{
		"":                              "http://0.0.0.0:9000",
		"http://0.0.0.0:1313":           "http://0.0.0.0:9000",
		"http://192.168.1.5:1313":       "http://192.168.1.5:9000",
		"https://blog.example.com:8443": "http://blog.example.com:9000",
		"not a url":                     "http://0.0.0.0:9000",
	}
	for in, want := range cases {
		if got := previewBaseURL(in, 9000); got != want {
			t.Errorf("previewBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
