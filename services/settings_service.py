# coding: utf-8
"""
应用设置服务
负责读取和持久化可在 UI 中编辑的配置项
"""

import json
import os
from pathlib import Path
from threading import Lock


class SettingsValidationError(ValueError):
    """用户输入的设置值不合法"""


class SettingsStorageError(RuntimeError):
    """设置读取/写入失败"""


class SettingsService:
    """应用设置服务"""

    def __init__(self, settings_file, defaults=None, legacy_settings_file=None):
        self.settings_file = Path(settings_file)
        self.defaults = defaults or {}
        self.legacy_settings_file = (
            Path(legacy_settings_file) if legacy_settings_file else None
        )
        self._lock = Lock()
        self.settings_file.parent.mkdir(parents=True, exist_ok=True)
        self._migrate_legacy_file_if_needed()

    def get_settings(self):
        """获取当前设置（默认值 + 文件覆盖）"""
        with self._lock:
            return self._load_settings_unlocked()

    def to_public_settings(self, settings):
        """返回可安全暴露给前端的设置（隐藏敏感值）"""
        ai_settings = settings.get("ai", {})

        return {
            "ai": {
                "base_url": ai_settings.get("base_url", "https://api.deepseek.com"),
                "model": ai_settings.get("model", "deepseek-chat"),
            }
        }

    def update_settings(self, updates):
        """更新并持久化设置"""
        if not isinstance(updates, dict):
            raise SettingsValidationError("设置格式无效")

        with self._lock:
            current = self._load_settings_unlocked()
            ai_updates = updates.get("ai", {})

            if "ai" in updates and not isinstance(ai_updates, dict):
                raise SettingsValidationError("AI 设置格式无效")

            if "base_url" in ai_updates:
                current["ai"]["base_url"] = ai_updates["base_url"]
            if "model" in ai_updates:
                current["ai"]["model"] = ai_updates["model"]

            normalized = self._normalize_and_validate(current)
            self._write_settings_file(normalized)

            return normalized

    def _migrate_legacy_file_if_needed(self):
        if not self.legacy_settings_file:
            return

        if self.settings_file.exists() or not self.legacy_settings_file.exists():
            return

        try:
            with self.legacy_settings_file.open("r", encoding="utf-8") as f:
                legacy_settings = json.load(f)

            if not isinstance(legacy_settings, dict):
                return

            merged = self._default_settings()
            legacy_ai = legacy_settings.get("ai", {})
            if isinstance(legacy_ai, dict):
                if "base_url" in legacy_ai:
                    merged["ai"]["base_url"] = legacy_ai["base_url"]
                if "model" in legacy_ai:
                    merged["ai"]["model"] = legacy_ai["model"]

            normalized = self._normalize_and_validate(merged)
            self._write_settings_file(normalized)
            self._cleanup_legacy_file()
        except (
            json.JSONDecodeError,
            OSError,
            SettingsValidationError,
            SettingsStorageError,
        ):
            # 迁移失败时不阻塞应用启动，后续继续走默认配置
            return

    def _cleanup_legacy_file(self):
        if not self.legacy_settings_file:
            return

        try:
            self.legacy_settings_file.unlink(missing_ok=True)
        except OSError:
            # 忽略清理失败，避免影响主流程
            return

    def _write_settings_file(self, settings):
        try:
            with self.settings_file.open("w", encoding="utf-8") as f:
                json.dump(settings, f, ensure_ascii=False, indent=2)
            self._ensure_file_permissions()
        except OSError as e:
            raise SettingsStorageError(f"保存设置失败: {e}") from e

    def _ensure_file_permissions(self):
        try:
            os.chmod(self.settings_file, 0o600)
        except OSError:
            # 某些平台可能不支持 chmod，忽略权限设置失败
            return

    def _load_settings_unlocked(self):
        file_exists = self.settings_file.exists()
        settings = self._default_settings()
        file_settings = self._read_file_settings()
        ai_from_file = file_settings.get("ai", {})
        needs_sanitize = False

        if isinstance(ai_from_file, dict):
            if "base_url" in ai_from_file:
                settings["ai"]["base_url"] = ai_from_file["base_url"]
            if "model" in ai_from_file:
                settings["ai"]["model"] = ai_from_file["model"]
            if "api_key" in ai_from_file:
                needs_sanitize = True

        normalized = self._normalize_and_validate(settings)
        if not file_exists or needs_sanitize:
            self._write_settings_file(normalized)

        return normalized

    def _default_settings(self):
        return {
            "ai": {
                "base_url": self.defaults.get("AI_BASE_URL")
                or "https://api.deepseek.com",
                "model": self.defaults.get("AI_MODEL") or "deepseek-chat",
            }
        }

    def _read_file_settings(self):
        if not self.settings_file.exists():
            return {}

        try:
            with self.settings_file.open("r", encoding="utf-8") as f:
                data = json.load(f)
        except json.JSONDecodeError as e:
            raise SettingsStorageError(f"设置文件 JSON 格式错误: {e}") from e
        except OSError as e:
            raise SettingsStorageError(f"读取设置文件失败: {e}") from e

        if data is None:
            return {}

        if not isinstance(data, dict):
            raise SettingsStorageError("设置文件格式错误: 顶层必须为对象")

        return data

    def _normalize_and_validate(self, settings):
        ai_settings = settings.get("ai", {})
        if not isinstance(ai_settings, dict):
            raise SettingsValidationError("AI 设置格式无效")

        base_url = ai_settings.get("base_url", "")
        model = ai_settings.get("model", "")

        if not isinstance(base_url, str) or not base_url.strip():
            raise SettingsValidationError("AI Base URL 不能为空")

        base_url = base_url.strip()
        if not base_url.startswith(("http://", "https://")):
            raise SettingsValidationError("AI Base URL 必须以 http:// 或 https:// 开头")

        if not isinstance(model, str) or not model.strip():
            raise SettingsValidationError("AI 模型不能为空")

        model = model.strip()

        return {
            "ai": {
                "base_url": base_url,
                "model": model,
            }
        }

    @staticmethod
    def _mask_api_key(api_key):
        if not api_key:
            return ""

        if len(api_key) <= 8:
            return "*" * len(api_key)

        return f"{api_key[:4]}...{api_key[-4:]}"
