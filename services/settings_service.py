# coding: utf-8
"""
应用设置服务
负责读取和持久化可在 UI 中编辑的配置项
"""

import json
from pathlib import Path
from threading import Lock


class SettingsService:
    """应用设置服务"""

    def __init__(self, settings_file, defaults=None):
        self.settings_file = Path(settings_file)
        self.defaults = defaults or {}
        self._lock = Lock()
        self.settings_file.parent.mkdir(parents=True, exist_ok=True)

    def get_settings(self):
        """获取当前设置（默认值 + 文件覆盖）"""
        with self._lock:
            return self._load_settings_unlocked()

    def to_public_settings(self, settings):
        """返回可安全暴露给前端的设置（隐藏敏感值）"""
        ai_settings = settings.get("ai", {})
        api_key = ai_settings.get("api_key", "")

        return {
            "ai": {
                "base_url": ai_settings.get("base_url", "https://api.deepseek.com"),
                "model": ai_settings.get("model", "deepseek-chat"),
                "api_key_configured": bool(api_key),
                "api_key_hint": self._mask_api_key(api_key),
            }
        }

    def update_settings(self, updates):
        """更新并持久化设置"""
        if not isinstance(updates, dict):
            raise ValueError("设置格式无效")

        with self._lock:
            current = self._load_settings_unlocked()
            ai_updates = updates.get("ai", {})

            if "ai" in updates and not isinstance(ai_updates, dict):
                raise ValueError("AI 设置格式无效")

            if "base_url" in ai_updates:
                current["ai"]["base_url"] = ai_updates["base_url"]
            if "model" in ai_updates:
                current["ai"]["model"] = ai_updates["model"]
            if "api_key" in ai_updates:
                current["ai"]["api_key"] = ai_updates["api_key"]

            normalized = self._normalize_and_validate(current)

            try:
                with self.settings_file.open("w", encoding="utf-8") as f:
                    json.dump(normalized, f, ensure_ascii=False, indent=2)
            except OSError as e:
                raise ValueError(f"保存设置失败: {e}") from e

            return normalized

    def _load_settings_unlocked(self):
        settings = self._default_settings()
        file_settings = self._read_file_settings()
        ai_from_file = file_settings.get("ai", {})

        if isinstance(ai_from_file, dict):
            if "base_url" in ai_from_file:
                settings["ai"]["base_url"] = ai_from_file["base_url"]
            if "model" in ai_from_file:
                settings["ai"]["model"] = ai_from_file["model"]
            if "api_key" in ai_from_file:
                settings["ai"]["api_key"] = ai_from_file["api_key"]

        return self._normalize_and_validate(settings)

    def _default_settings(self):
        return {
            "ai": {
                "base_url": self.defaults.get("AI_BASE_URL")
                or "https://api.deepseek.com",
                "model": self.defaults.get("AI_MODEL") or "deepseek-chat",
                "api_key": self.defaults.get("AI_API_KEY") or "",
            }
        }

    def _read_file_settings(self):
        if not self.settings_file.exists():
            return {}

        try:
            with self.settings_file.open("r", encoding="utf-8") as f:
                data = json.load(f)
        except json.JSONDecodeError as e:
            raise ValueError(f"设置文件 JSON 格式错误: {e}") from e
        except OSError as e:
            raise ValueError(f"读取设置文件失败: {e}") from e

        if data is None:
            return {}

        if not isinstance(data, dict):
            raise ValueError("设置文件格式错误: 顶层必须为对象")

        return data

    def _normalize_and_validate(self, settings):
        ai_settings = settings.get("ai", {})
        if not isinstance(ai_settings, dict):
            raise ValueError("AI 设置格式无效")

        base_url = ai_settings.get("base_url", "")
        model = ai_settings.get("model", "")
        api_key = ai_settings.get("api_key", "")

        if not isinstance(base_url, str) or not base_url.strip():
            raise ValueError("AI Base URL 不能为空")

        base_url = base_url.strip()
        if not base_url.startswith(("http://", "https://")):
            raise ValueError("AI Base URL 必须以 http:// 或 https:// 开头")

        if not isinstance(model, str) or not model.strip():
            raise ValueError("AI 模型不能为空")

        model = model.strip()

        if not isinstance(api_key, str):
            raise ValueError("AI API Key 格式无效")

        api_key = api_key.strip()

        return {
            "ai": {
                "base_url": base_url,
                "model": model,
                "api_key": api_key,
            }
        }

    @staticmethod
    def _mask_api_key(api_key):
        if not api_key:
            return ""

        if len(api_key) <= 8:
            return "*" * len(api_key)

        return f"{api_key[:4]}...{api_key[-4:]}"
