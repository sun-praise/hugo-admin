# coding: utf-8
"""
主题管理路由
提供 Hugo 主题的发现、安装、激活与预览 API。
"""

from flask import Blueprint, jsonify, request
from werkzeug.exceptions import BadRequest

from services.theme_service import ThemeError, ThemeService

theme_bp = Blueprint("theme", __name__, url_prefix="/api/themes")


def register_theme_routes(registry):
    """
    注册主题管理路由。

    Args:
        registry: ServiceRegistry 实例。
    """

    def _theme_service():
        return ThemeService(
            registry.hugo_manager.hugo_root,
            settings_service=registry.settings_service,
        )

    @theme_bp.route("", methods=["GET"])
    def list_themes():
        """获取已安装主题列表。"""
        try:
            themes = _theme_service().list_themes()
            active = _theme_service().get_active_theme()
        except ThemeError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except Exception as e:
            return jsonify({"success": False, "message": f"获取主题失败: {e}"}), 500

        return jsonify(
            {
                "success": True,
                "themes": themes,
                "active_theme": active,
            }
        )

    @theme_bp.route("/install", methods=["POST"])
    def install_theme():
        """安装主题。"""
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

        repo_url = data.get("repo_url", "")
        name = data.get("name", "")
        mode = data.get("mode", "submodule")

        try:
            result = _theme_service().install_theme(repo_url, name, mode=mode)
        except ThemeError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except Exception as e:
            return jsonify({"success": False, "message": f"安装主题失败: {e}"}), 500

        return jsonify(
            {
                "success": True,
                "message": "主题安装成功",
                "theme": result,
            }
        )

    @theme_bp.route("/activate", methods=["POST"])
    def activate_theme():
        """激活主题。"""
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

        name = data.get("name", "")
        try:
            result = _theme_service().activate_theme(name)
        except ThemeError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except Exception as e:
            return jsonify({"success": False, "message": f"激活主题失败: {e}"}), 500

        return jsonify(
            {
                "success": True,
                "message": "主题已激活",
                "theme": result,
            }
        )

    @theme_bp.route("/preview", methods=["POST"])
    def preview_theme():
        """预览主题（不持久化活跃主题）。"""
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

        name = data.get("name", "")
        if not name:
            return (
                jsonify({"success": False, "message": "主题名称不能为空"}),
                400,
            )

        theme_service = _theme_service()
        if not theme_service.theme_exists(name):
            return (
                jsonify({"success": False, "message": f"主题不存在: {name}"}),
                400,
            )

        manager = registry.hugo_manager

        # 停止当前运行的服务器
        if manager.is_running:
            manager.stop()

        # 临时使用环境变量覆盖持久化主题，启动服务器
        import os

        original_env = os.environ.get("HUGO_THEME")
        try:
            os.environ["HUGO_THEME"] = name
            success, message = manager.start(debug=False)
        finally:
            if original_env is None:
                os.environ.pop("HUGO_THEME", None)
            else:
                os.environ["HUGO_THEME"] = original_env

        if not success:
            return jsonify({"success": False, "message": message}), 500

        return jsonify(
            {
                "success": True,
                "message": "预览服务器已启动",
                "preview_theme": name,
                "server_url": manager.server_url,
            }
        )

    return theme_bp
