package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(filepath.Join(t.TempDir(), "settings.json"), nil, "")
}

func TestDefaultsAndRoundTrip(t *testing.T) {
	s := newTestService(t)
	st, err := s.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if st.AI["base_url"] != "https://api.deepseek.com" || st.AI["model"] != "deepseek-chat" {
		t.Fatalf("defaults = %#v", st)
	}

	updated, err := s.UpdateSettings(map[string]any{
		"ai":       map[string]any{"base_url": "https://llm.example.com", "model": "glm-4.6"},
		"listmonk": map[string]any{"api_url": "https://lm.example.com", "api_user": "u", "api_key": "k-12345678", "blog_list_id": 3},
		"theme":    map[string]any{"name": "Fried-Rice"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AI["model"] != "glm-4.6" || updated.Listmonk["blog_list_id"] != 3 {
		t.Fatalf("updated = %#v", updated)
	}

	// 脱敏
	public := s.ToPublicSettings(updated)
	lm := public["listmonk"].(map[string]any)
	if lm["api_key"] != "k-12...5678" {
		t.Fatalf("mask = %v", lm["api_key"])
	}
}

func TestValidationErrors(t *testing.T) {
	s := newTestService(t)
	cases := []struct {
		name    string
		updates map[string]any
	}{
		{"空 base_url", map[string]any{"ai": map[string]any{"base_url": " "}}},
		{"非 http", map[string]any{"ai": map[string]any{"base_url": "ftp://x"}}},
		{"空 model", map[string]any{"ai": map[string]any{"model": ""}}},
		{"server_url 非 http", map[string]any{"hugo": map[string]any{"server_url": "abc"}}},
		{"主题含路径", map[string]any{"theme": map[string]any{"name": "../x"}}},
	}
	for _, c := range cases {
		if _, err := s.UpdateSettings(c.updates); err == nil {
			t.Errorf("%s: 应报错", c.name)
		}
	}

	// base_dir 校验：相对路径 / 不存在 / 无 Hugo 配置
	tmp := t.TempDir()
	if _, err := s.UpdateSettings(map[string]any{"hugo": map[string]any{"base_dir": "relative"}}); err == nil || !strings.Contains(err.Error(), "绝对路径") {
		t.Errorf("相对路径: %v", err)
	}
	if _, err := s.UpdateSettings(map[string]any{"hugo": map[string]any{"base_dir": tmp}}); err == nil || !strings.Contains(err.Error(), "配置文件") {
		t.Errorf("无配置目录: %v", err)
	}
	os.WriteFile(filepath.Join(tmp, "hugo.toml"), []byte("baseURL='x'\n"), 0o644)
	if _, err := s.UpdateSettings(map[string]any{"hugo": map[string]any{"base_dir": tmp}}); err != nil {
		t.Errorf("合法目录应通过: %v", err)
	}
}

func TestLegacyMigration(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "legacy.json")
	os.WriteFile(legacy, []byte(`{"ai":{"base_url":"https://old.example.com","model":"m1","api_key":"leak"}}`), 0o644)
	_ = NewService(filepath.Join(base, "settings.json"), nil, legacy)

	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy 文件应被清理")
	}
	st, err := NewService(filepath.Join(base, "settings.json"), nil, "").GetSettings()
	if err != nil || st.AI["base_url"] != "https://old.example.com" {
		t.Fatalf("迁移结果 = %#v %v", st, err)
	}
	// api_key 泄漏字段应被 sanitize
	raw, _ := os.ReadFile(filepath.Join(base, "settings.json"))
	if strings.Contains(string(raw), "leak") {
		t.Fatal("api_key 应从落盘文件剔除")
	}
}
