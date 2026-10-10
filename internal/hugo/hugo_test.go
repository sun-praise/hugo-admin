package hugo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/svtter/hugo-admin/internal/realtime"
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
