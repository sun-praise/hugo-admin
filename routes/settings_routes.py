# coding: utf-8
"""
应用设置相关路由
"""

from pathlib import Path
from urllib.parse import urlparse, urlunparse

from flask import Blueprint, jsonify, request
from werkzeug.exceptions import BadRequest

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


def register_settings_routes(app, settings_state):
    """
    注册设置相关路由。

    Args:
        app: Flask 应用实例。
        settings_state: 可变状态容器，每个键对应一个单元素列表 [value]，
            以便在闭包中重新赋值。
            键: session_api_key, env_api_key, post_service, git_service,
                hugo_manager, settings_service, ref_service, ai_service, socketio
    """

    @bp.route("/api/settings")
    def get_settings():
        """获取应用设置"""
        try:
            settings = settings_state["settings_service"][0].get_settings()
            return jsonify(
                {
                    "success": True,
                    "settings": _to_public_settings(
                        settings_state["settings_service"][0],
                        settings,
                        app,
                        settings_state["session_api_key"][0],
                        settings_state["env_api_key"][0],
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

        ss = settings_state["settings_service"][0]
        try:
            updated_settings = ss.update_settings(settings_payload)
        except SettingsValidationError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except SettingsStorageError as e:
            return jsonify({"success": False, "message": str(e)}), 500

        if incoming_session_api_key is not None:
            settings_state["session_api_key"][0] = incoming_session_api_key

        app.config["AI_BASE_URL"] = updated_settings["ai"]["base_url"]
        app.config["AI_MODEL"] = updated_settings["ai"]["model"]
        app.config["AI_API_KEY"] = (
            settings_state["session_api_key"][0] or settings_state["env_api_key"][0]
        )

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
            new_ref_service.scan_all()
            new_git_service = GitService(new_root)
            new_hugo_manager = HugoServerManager(
                new_root,
                settings_state["socketio"][0],
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

            settings_state["post_service"][0] = new_post_service
            settings_state["ref_service"][0] = new_ref_service
            settings_state["git_service"][0] = new_git_service
            settings_state["hugo_manager"][0] = new_hugo_manager
            settings_state["settings_service"][0] = new_settings_service
        elif new_server_url != settings_state["hugo_manager"][0].server_url:
            settings_state["hugo_manager"][0].server_url = (
                new_server_url
                or app.config.get("HUGO_SERVER_BASE_URL", "http://0.0.0.0:1313")
            )

        settings_state["ai_service"][0] = None

        return jsonify(
            {
                "success": True,
                "message": "设置已保存",
                "settings": _to_public_settings(
                    settings_state["settings_service"][0],
                    updated_settings,
                    app,
                    settings_state["session_api_key"][0],
                    settings_state["env_api_key"][0],
                ),
            }
        )

    @bp.route("/api/version")
    def get_version():
        """获取应用版本号"""
        from __version__ import __version__

        return jsonify({"version": __version__})

    return bp
