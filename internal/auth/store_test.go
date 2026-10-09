package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTempStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func TestStoreBootstrap(t *testing.T) {
	s := newTempStore(t)
	if !s.Verify("admin", "admin") {
		t.Fatal("默认 admin/admin 应通过")
	}
	if s.Verify("admin", "wrong") || s.Verify("other", "admin") {
		t.Fatal("错误凭据不应通过")
	}
}

func TestStoreSetPasswordPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.SetPassword("admin", "new-secret"); err != nil {
		t.Fatalf("set: %v", err)
	}
	if !s.Verify("admin", "new-secret") || s.Verify("admin", "admin") {
		t.Fatal("改密后校验不符")
	}
	// 重新加载：确认已落盘
	s2, err := OpenStore(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !s2.Verify("admin", "new-secret") {
		t.Fatal("重载后新密码失效")
	}
	if err := s2.SetPassword("nobody", "x"); err != ErrUsernameMismatch {
		t.Fatalf("用户名不匹配应报错，得到 %v", err)
	}
}

func TestStoreFailClosedOnCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil || !strings.Contains(err.Error(), "损坏") {
		t.Fatalf("损坏文件应 fail closed，得到 %v", err)
	}
	// 缺字段
	if err := os.WriteFile(path, []byte(`{"username":"a"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(path); err == nil {
		t.Fatal("缺 password_hash 应报错")
	}
}
