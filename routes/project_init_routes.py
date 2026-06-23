# coding: utf-8
"""
项目初始化路由
提供从管理界面创建新 Hugo 站点的 API。
"""

import subprocess
from pathlib import Path

from flask import Blueprint, jsonify, request
from werkzeug.exceptions import BadRequest

from services.project_init_service import ProjectInitError, ProjectInitService

project_init_bp = Blueprint("project_init", __name__, url_prefix="/api/project")


def register_project_init_routes(app, registry):
    """
    注册项目初始化路由。

    Args:
        app: Flask 应用实例。
        registry: ServiceRegistry 实例。
    """
    admin_root = Path(app.root_path)

    @project_init_bp.route("/init", methods=["POST"])
    def init_project():
        """创建新的 Hugo 站点并将其设为活跃项目。"""
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

        path = data.get("path", "")
        config_format = data.get("config_format", "toml")

        if not isinstance(path, str) or not path.strip():
            return (
                jsonify({"success": False, "message": "目标路径不能为空"}),
                400,
            )

        if config_format not in {"toml", "yaml"}:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "配置文件格式仅支持 toml 或 yaml",
                    }
                ),
                400,
            )

        service = ProjectInitService(admin_root)
        try:
            result = service.create_site(path, config_format=config_format)
            service.switch_active_project(app, registry, result["path"])
        except ProjectInitError as e:
            return jsonify({"success": False, "message": str(e)}), 400
        except (OSError, ValueError, TypeError, subprocess.SubprocessError) as e:
            return jsonify({"success": False, "message": f"初始化失败: {e}"}), 500

        return jsonify(
            {
                "success": True,
                "message": "Hugo 站点已创建并设为活跃项目",
                "path": result["path"],
                "config_format": result["config_format"],
            }
        )

    return project_init_bp
