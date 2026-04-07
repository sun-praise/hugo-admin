# coding: utf-8
"""
应用设置 API 测试
"""

import json
import tempfile
from pathlib import Path

import pytest

import app as app_module
from services.settings_service import SettingsService


class TestSettingsAPI:
    @pytest.fixture
    def client(self):
        """测试客户端，使用临时 settings 文件"""
        with tempfile.TemporaryDirectory() as temp_dir:
            content_dir = Path(temp_dir) / "content"
            content_dir.mkdir(parents=True, exist_ok=True)
            settings_file = content_dir / ".admin" / "settings.json"

            original_settings_service = app_module.settings_service
            original_content_dir = app_module.app.config.get("CONTENT_DIR")
            original_ai_base_url = app_module.app.config.get("AI_BASE_URL")
            original_ai_model = app_module.app.config.get("AI_MODEL")
            original_ai_api_key = app_module.app.config.get("AI_API_KEY")
            original_env_ai_api_key = app_module.ENV_AI_API_KEY
            original_ai_service = app_module.ai_service

            app_module.settings_service = SettingsService(
                settings_file,
                defaults={
                    "AI_BASE_URL": original_ai_base_url,
                    "AI_MODEL": original_ai_model,
                },
            )
            app_module.app.config["CONTENT_DIR"] = content_dir
            app_module.app.config["AI_BASE_URL"] = (
                original_ai_base_url or "https://api.deepseek.com"
            )
            app_module.app.config["AI_MODEL"] = original_ai_model or "deepseek-chat"
            app_module.ENV_AI_API_KEY = ""
            app_module.app.config["AI_API_KEY"] = ""
            app_module.app.config["TESTING"] = True
            app_module.ai_service = object()

            with app_module.app.test_client() as client:
                yield client, settings_file

            app_module.settings_service = original_settings_service
            app_module.app.config["CONTENT_DIR"] = original_content_dir
            app_module.app.config["AI_BASE_URL"] = original_ai_base_url
            app_module.app.config["AI_MODEL"] = original_ai_model
            app_module.app.config["AI_API_KEY"] = original_ai_api_key
            app_module.ENV_AI_API_KEY = original_env_ai_api_key
            app_module.ai_service = original_ai_service

    def test_get_settings_returns_defaults(self, client):
        """获取设置应返回默认值"""
        test_client, _ = client

        response = test_client.get("/api/settings")

        assert response.status_code == 200
        data = response.get_json()
        assert data["success"] is True
        assert data["settings"]["ai"]["base_url"]
        assert data["settings"]["ai"]["model"]

    def test_update_settings_persists_and_refreshes_ai_service(self, client):
        """保存设置后应写入文件并重置 AI 服务"""
        test_client, settings_file = client

        payload = {
            "ai": {
                "base_url": "https://api.example.com",
                "model": "deepseek-reasoner",
            }
        }

        response = test_client.put("/api/settings", json=payload)

        assert response.status_code == 200
        data = response.get_json()
        assert data["success"] is True
        assert data["settings"]["ai"]["base_url"] == payload["ai"]["base_url"]
        assert data["settings"]["ai"]["model"] == payload["ai"]["model"]

        saved = json.loads(settings_file.read_text(encoding="utf-8"))
        assert saved["ai"]["base_url"] == payload["ai"]["base_url"]
        assert saved["ai"]["model"] == payload["ai"]["model"]

        assert app_module.app.config["AI_BASE_URL"] == payload["ai"]["base_url"]
        assert app_module.app.config["AI_MODEL"] == payload["ai"]["model"]
        assert app_module.ai_service is None

    def test_update_settings_rejects_invalid_base_url(self, client):
        """不合法 base_url 应返回 400"""
        test_client, _ = client

        response = test_client.put(
            "/api/settings",
            json={"ai": {"base_url": "api.deepseek.com"}},
        )

        assert response.status_code == 400
        data = response.get_json()
        assert data["success"] is False
        assert "Base URL" in data["message"]

    def test_update_api_key_masks_in_response(self, client):
        """保存 API Key 后响应中应只返回掩码信息"""
        test_client, settings_file = client

        payload = {"ai": {"api_key": "sk-test-1234567890"}}
        response = test_client.put("/api/settings", json=payload)

        assert response.status_code == 200
        data = response.get_json()
        ai_settings = data["settings"]["ai"]
        assert ai_settings["api_key_source"] == "settings"
        assert ai_settings["api_key_configured"] is True
        assert ai_settings["api_key_hint"] == "sk-t...7890"
        assert "api_key" not in ai_settings

        saved = json.loads(settings_file.read_text(encoding="utf-8"))
        assert saved["ai"]["api_key"] == payload["ai"]["api_key"]
        assert app_module.app.config["AI_API_KEY"] == payload["ai"]["api_key"]

    def test_clear_saved_api_key_falls_back_to_env(self, client):
        """清除设置中的 API Key 后应回退到环境变量"""
        test_client, _ = client

        app_module.ENV_AI_API_KEY = "env-key-123456"
        app_module.app.config["AI_API_KEY"] = app_module.ENV_AI_API_KEY

        test_client.put("/api/settings", json={"ai": {"api_key": "saved-key-abcdef"}})

        clear_response = test_client.put("/api/settings", json={"ai": {"api_key": ""}})
        assert clear_response.status_code == 200

        data = clear_response.get_json()
        ai_settings = data["settings"]["ai"]
        assert ai_settings["api_key_source"] == "env"
        assert ai_settings["api_key_configured"] is True
        assert app_module.app.config["AI_API_KEY"] == "env-key-123456"
