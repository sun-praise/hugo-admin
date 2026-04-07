# coding: utf-8
"""
SettingsService 单元测试
"""

import json

from services.settings_service import SettingsService


def test_update_settings_does_not_persist_api_key(tmp_path):
    """API Key 只用于运行时，不应写入设置文件"""
    settings_file = tmp_path / ".admin" / "settings.json"
    service = SettingsService(settings_file)

    result = service.update_settings(
        {
            "ai": {
                "base_url": "https://api.example.com",
                "model": "deepseek-reasoner",
                "api_key": "sk-secret-123456",
            }
        }
    )

    assert result["ai"]["base_url"] == "https://api.example.com"
    assert result["ai"]["model"] == "deepseek-reasoner"
    assert "api_key" not in result["ai"]

    saved = json.loads(settings_file.read_text(encoding="utf-8"))
    assert "api_key" not in saved["ai"]


def test_migrate_legacy_settings_strips_api_key(tmp_path):
    """迁移旧 settings 文件时应移除 legacy API Key"""
    new_settings_file = tmp_path / ".admin" / "settings.json"
    legacy_settings_file = tmp_path / "content" / ".admin" / "settings.json"
    legacy_settings_file.parent.mkdir(parents=True, exist_ok=True)
    legacy_settings_file.write_text(
        json.dumps(
            {
                "ai": {
                    "base_url": "https://legacy.example.com",
                    "model": "legacy-model",
                    "api_key": "legacy-secret",
                }
            },
            ensure_ascii=False,
            indent=2,
        ),
        encoding="utf-8",
    )

    service = SettingsService(
        new_settings_file,
        legacy_settings_file=legacy_settings_file,
    )
    settings = service.get_settings()

    assert settings["ai"]["base_url"] == "https://legacy.example.com"
    assert settings["ai"]["model"] == "legacy-model"
    assert "api_key" not in settings["ai"]

    migrated = json.loads(new_settings_file.read_text(encoding="utf-8"))
    assert "api_key" not in migrated["ai"]
    assert not legacy_settings_file.exists()


def test_chmod_failure_does_not_break_save(tmp_path, monkeypatch):
    """chmod 失败不应导致保存失败"""
    settings_file = tmp_path / ".admin" / "settings.json"
    service = SettingsService(settings_file)

    monkeypatch.setattr(
        "services.settings_service.os.chmod",
        lambda *_args, **_kwargs: (_ for _ in ()).throw(OSError("no chmod")),
    )

    result = service.update_settings({"ai": {"model": "deepseek-chat"}})

    assert result["ai"]["model"] == "deepseek-chat"
    assert settings_file.exists()


def test_get_settings_persists_defaults_when_file_missing(tmp_path):
    """首次读取设置时应将默认值写入文件（不含 API Key）"""
    settings_file = tmp_path / ".admin" / "settings.json"
    service = SettingsService(
        settings_file,
        defaults={
            "AI_BASE_URL": "https://api.current.example.com",
            "AI_MODEL": "current-model",
            "AI_API_KEY": "should-not-persist",
        },
    )

    settings = service.get_settings()

    assert settings["ai"]["base_url"] == "https://api.current.example.com"
    assert settings["ai"]["model"] == "current-model"
    assert "api_key" not in settings["ai"]

    saved = json.loads(settings_file.read_text(encoding="utf-8"))
    assert saved["ai"]["base_url"] == "https://api.current.example.com"
    assert saved["ai"]["model"] == "current-model"
    assert "api_key" not in saved["ai"]
