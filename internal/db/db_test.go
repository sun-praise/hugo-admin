package db

import (
	"path/filepath"
	"testing"
)

func openTempDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestPushHistory(t *testing.T) {
	d := openTempDB(t)

	d.RecordPush("origin", "main", "", "abc123", 2, "feat: x", "推送成功到 origin/main", true)
	d.RecordPush("origin", "main", "abc123", "", 0, "feat: x", "推送失败: 网络错误", false)

	pushes, total, err := d.ListPushes(20, 0)
	if err != nil || total != 2 || len(pushes) != 2 {
		t.Fatalf("list = %v %d %v", pushes, total, err)
	}
	// 倒序：第二条（失败）在后插入 → 更晚的 pushed_at 排前
	if pushes[0].Success != false || pushes[0].Message != "推送失败: 网络错误" {
		t.Fatalf("第一条 = %#v", pushes[0])
	}
	second := pushes[1]
	if second.FromSHA != "" || second.ToSHA != "abc123" || second.CommitCount != 2 ||
		second.CommitMessage != "feat: x" || !second.Success {
		t.Fatalf("第二条 = %#v", second)
	}
	if second.ID != 1 || pushes[0].ID != 2 {
		t.Fatalf("自增 id = %d %d", second.ID, pushes[0].ID)
	}
	if second.PushedAtISO == "" {
		t.Fatal("pushed_at_iso 为空")
	}

	// 分页
	_, total, _ = d.ListPushes(1, 1)
	if total != 2 {
		t.Fatalf("total = %d", total)
	}
	pushes, _, _ = d.ListPushes(1, 1)
	if len(pushes) != 1 || pushes[0].ID != 1 {
		t.Fatalf("分页 = %#v", pushes)
	}
}

func TestChatRoundTrip(t *testing.T) {
	d := openTempDB(t)

	s1, err := d.CreateChatSession("会话一")
	if err != nil || len(s1.ID) != 32 || s1.Title != "会话一" {
		t.Fatalf("create = %#v %v", s1, err)
	}
	s2, _ := d.CreateChatSession("会话二")

	if _, err := d.AddChatMessage(s1.ID, "user", "你好", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddChatMessage(s1.ID, "assistant", "你好，有什么可以帮你？", "text"); err != nil {
		t.Fatal(err)
	}

	msgs, err := d.GetChatMessages(s1.ID)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages = %#v %v", msgs, err)
	}
	if msgs[0].Role != "user" || msgs[0].MessageType != "text" { // 空类型回退 text
		t.Fatalf("msgs[0] = %#v", msgs[0])
	}
	if msgs[0].CreatedAt > msgs[1].CreatedAt {
		t.Fatal("消息应按时间升序")
	}

	// 会话列表按 updated_at 倒序：s1 有新消息 → 排前
	sessions, _ := d.ListChatSessions()
	if len(sessions) != 2 || sessions[0].ID != s1.ID || sessions[1].ID != s2.ID {
		t.Fatalf("sessions = %#v", sessions)
	}

	// 标题更新与删除
	if err := d.UpdateChatSessionTitle(s2.ID, "改名"); err != nil {
		t.Fatal(err)
	}
	got, ok, _ := d.GetChatSession(s2.ID)
	if !ok || got.Title != "改名" {
		t.Fatalf("get = %#v %v", got, ok)
	}
	if err := d.DeleteChatSession(s1.ID); err != nil {
		t.Fatal(err)
	}
	if msgs, _ := d.GetChatMessages(s1.ID); len(msgs) != 0 {
		t.Fatalf("删除会话后消息应清空: %#v", msgs)
	}
	if _, ok, _ := d.GetChatSession(s1.ID); ok {
		t.Fatal("会话应已删除")
	}
}
