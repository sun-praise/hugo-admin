package ai

import (
	"path/filepath"
	"testing"

	"github.com/sun-praise/hugo-admin/v3/internal/settings"
)

func TestConfigStoreDefaultsAndKeyPrecedence(t *testing.T) {
	settingsSvc := settings.NewService(filepath.Join(t.TempDir(), "settings.json"), nil, "")
	store := NewConfigStore(settingsSvc, "env-key")

	cfg := store.Get()
	if cfg.BaseURL != defaultBaseURL || cfg.Model != defaultModel {
		t.Fatalf("defaults = %#v", cfg)
	}
	if cfg.APIKey != "env-key" {
		t.Fatalf("env key = %q", cfg.APIKey)
	}
	if source, hint := store.KeyInfo(); source != "env" || hint != "" {
		t.Fatalf("env key info = %q %q", source, hint)
	}

	store.SetAPIKey("session-key-abcd1234")
	cfg = store.Get()
	if cfg.APIKey != "session-key-abcd1234" {
		t.Fatalf("session key = %q", cfg.APIKey)
	}
	if source, hint := store.KeyInfo(); source != "session" || hint != "sess...1234" {
		t.Fatalf("session key info = %q %q", source, hint)
	}
}

func TestConfigStoreReadsSettings(t *testing.T) {
	settingsSvc := settings.NewService(filepath.Join(t.TempDir(), "settings.json"), map[string]string{
		"AI_BASE_URL": "https://llm.example.com",
		"AI_MODEL":    "deepseek-v4-flash",
	}, "")
	store := NewConfigStore(settingsSvc, "")

	cfg := store.Get()
	if cfg.BaseURL != "https://llm.example.com" || cfg.Model != "deepseek-v4-flash" {
		t.Fatalf("settings config = %#v", cfg)
	}
	if cfg.APIKey != "" {
		t.Fatalf("key should be empty: %q", cfg.APIKey)
	}
	if source, _ := store.KeyInfo(); source != "none" {
		t.Fatalf("key source = %q", source)
	}

	// 设置更新后由设置页调用 Refresh 生效；请求路径不会重新读盘。
	if _, err := settingsSvc.UpdateSettings(map[string]any{
		"ai": map[string]any{"model": "deepseek-flash"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.Get().Model; got != "deepseek-v4-flash" {
		t.Fatalf("model before refresh = %q", got)
	}
	store.Refresh()
	if got := store.Get().Model; got != "deepseek-flash" {
		t.Fatalf("model after refresh = %q", got)
	}
}

func TestServiceUsesDynamicConfig(t *testing.T) {
	store := NewConfigStore(nil, "")
	svc := NewWithConfigSource(store.Get, Deps{})
	if svc.Enabled() {
		t.Fatal("empty key should disable service")
	}
	if svc.ModelName() != defaultModel {
		t.Fatalf("model = %q", svc.ModelName())
	}

	store.SetAPIKey("key-1")
	if !svc.Enabled() {
		t.Fatal("key should enable service")
	}

	store.SetAPIKey("")
	if svc.Enabled() {
		t.Fatal("cleared key should disable service")
	}
}
