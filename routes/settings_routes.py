# coding: utf-8
"""
应用设置相关路由
"""

from pathlib import Path
from urllib.parse import urlparse, urlunparse

from flask import Blueprint, jsonify, request
from werkzeug.exceptions import BadRequest

from models.database import Database
from services.git_service import GitService
from services.hugo_service import HugoServerManager
from services.post_service import PostService
from services.reference_service import ReferenceService
from services.settings_service import (
    SettingsService,
    SettingsStorageError,
    SettingsValidationError,
)

bp = Blueprint("settings", __name__)


def _ensure_server_url_has_port(app, url: str) -> str:
    """若 URL 已带 scheme 但缺少显式端口，则按 HUGO_SERVER_PORT 补齐。"""
    if not url:
        return url
    parsed = urlparse(url)
    if not parsed.scheme or not parsed.hostname or parsed.port is not None:
        return url
    default_port = app.config.get("HUGO_SERVER_PORT", 1313)
    return urlunparse(parsed._replace(netloc=f"{parsed.hostname}:{default_port}"))


def _to_public_settings(settings_service, settings, app, session_api_key, env_api_key):
    """将设置转换为前端可展示格式，并补充密钥来源信息"""
    public_settings = settings_service.to_public_settings(settings)

    public_settings["hugo"]["base_dir"] = public_settings["hugo"]["base_dir"] or str(
        app.config.get("HUGO_ROOT", "")
    )
    public_settings["hugo"]["server_url"] = _ensure_server_url_has_port(
        app,
        public_settings["hugo"].get("server_url", "")
        or app.config.get("HUGO_SERVER_BASE_URL", "http://0.0.0.0:1313"),
    )

    if session_api_key:
        source = "session"
        configured = True
        api_key_hint = settings_service._mask_api_key(session_api_key)
    elif env_api_key:
        source = "env"
        configured = True
        api_key_hint = ""
    else:
        source = "none"
        configured = False
        api_key_hint = ""

    public_settings["ai"]["api_key_source"] = source
    public_settings["ai"]["api_key_configured"] = configured
    public_settings["ai"]["api_key_hint"] = api_key_hint

    return public_settings


def register_settings_routes(app, registry):
    """
    注册设置相关路由。

    Args:
        app: Flask 应用实例。
        registry: 服务注册表，提供所有可变服务实例的属性访问。
    """

    @bp.route("/api/settings")
    def get_settings():
        """获取应用设置"""
        try:
            settings = registry.settings_service.get_settings()
            return jsonify(
                {
                    "success": True,
                    "settings": _to_public_settings(
                        registry.settings_service,
                        settings,
                        app,
                        registry.session_api_key,
                        registry.env_api_key,
                    ),
                }
            )
        except (SettingsValidationError, SettingsStorageError) as e:
            return jsonify({"success": False, "message": str(e)}), 500

    @bp.route("/api/settings", methods=["PUT"])
    def update_settings():
        """更新应用设置"""
        if not request.is_json:
            return (
                jsonify({"success": False, "message": "请求体必须是 JSON 对象"}),
                400,
            )

        try:
            data = request.get_json(silent=False)
        except BadRequest:
            return jsonify({"success": False, "message": "请求体不是合法 JSON"}), 400

        if not isinstance(data, dict):
            return (
                jsonify({"success": False, "message": "请求体必须是 JSON 对象"}),
                400,
            )

        payload = data.get("settings", data)

        if not isinstance(payload, dict):
            return jsonify({"success": False, "message": "设置格式无效"}), 400

        settings_payload = payload
        incoming_session_api_key = None
        ai_payload = payload.get("ai")
        if isinstance(ai_payload, dict) and "api_key" in ai_payload:
            api_key = ai_payload.get("api_key")
            if not isinstance(api_key, str):
                return (
                    jsonify({"success": False, "message": "AI API Key 格式无效"}),
                    400,
                )

            incoming_session_api_key = api_key.strip()
            settings_payload = dict(payload)
            settings_payload["ai"] = dict(ai_payload)
            settings_payload["ai"].pop("api_key", None)

        ss = registry.settings_service
        try:
            updated_settings = ss.update_settings(settings_payload)
        except SettingsValidationError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except SettingsStorageError as e:
            return jsonify({"success": False, "message": str(e)}), 500

        if incoming_session_api_key is not None:
            registry.session_api_key = incoming_session_api_key

        app.config["AI_BASE_URL"] = updated_settings["ai"]["base_url"]
        app.config["AI_MODEL"] = updated_settings["ai"]["model"]
        app.config["AI_API_KEY"] = registry.session_api_key or registry.env_api_key

        new_hugo_root = updated_settings.get("hugo", {}).get("base_dir", "")
        new_server_url = _ensure_server_url_has_port(
            app, updated_settings.get("hugo", {}).get("server_url", "")
        )
        if new_hugo_root and str(app.config["HUGO_ROOT"]) != new_hugo_root:
            new_root = Path(new_hugo_root)
            app.config["HUGO_ROOT"] = new_root
            app.config["CONTENT_DIR"] = new_root / "content"
            new_post_service = PostService(app.config["CONTENT_DIR"], use_cache=True)
            new_ref_service = ReferenceService(
                app.config["CONTENT_DIR"],
                (
                    new_post_service.cache_service.db
                    if new_post_service.cache_service
                    else None
                ),
            )
            new_db_path = Path(app.config["CONTENT_DIR"]) / ".admin" / "cache.db"
            new_db = Database(str(new_db_path))
            new_git_service = GitService(new_root, database=new_db)
            new_hugo_manager = HugoServerManager(
                new_root,
                registry.socketio,
                server_url=new_server_url or None,
            )
            new_settings_service = SettingsService(
                new_root / ".admin" / "settings.json",
                legacy_settings_file=Path(app.config["CONTENT_DIR"])
                / ".admin"
                / "settings.json",
                defaults={
                    "AI_BASE_URL": app.config.get(
                        "AI_BASE_URL", "https://api.deepseek.com"
                    ),
                    "AI_MODEL": app.config.get("AI_MODEL", "deepseek-chat"),
                    "HUGO_BASE_DIR": str(new_root),
                    "HUGO_SERVER_URL": new_server_url
                    or app.config.get("HUGO_SERVER_BASE_URL", "http://0.0.0.0:1313"),
                },
            )

            registry.post_service = new_post_service
            registry.ref_service = new_ref_service
            registry.git_service = new_git_service
            registry.hugo_manager = new_hugo_manager
            registry.database = new_db
            registry.settings_service = new_settings_service
        elif new_server_url != registry.hugo_manager.server_url:
            fallback_url = _ensure_server_url_has_port(
                app, app.config.get("HUGO_SERVER_BASE_URL", "http://0.0.0.0:1313")
            )
            registry.hugo_manager.server_url = new_server_url or fallback_url

        registry.ai_service = None

        return jsonify(
            {
                "success": True,
                "message": "设置已保存",
                "settings": _to_public_settings(
                    registry.settings_service,
                    updated_settings,
                    app,
                    registry.session_api_key,
                    registry.env_api_key,
                ),
            }
        )

    return bp
