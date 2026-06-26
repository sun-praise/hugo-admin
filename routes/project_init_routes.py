# coding: utf-8
"""
项目初始化路由
提供从管理界面创建新 Hugo 站点的 API。
"""

import subprocess
from pathlib import Path

from flask import Blueprint, jsonify, request
from werkzeug.exceptions import BadRequest

from services.active_project import ActiveProjectRegistry
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
                "default_theme": result.get("default_theme"),
            }
        )

    @project_init_bp.route("/active", methods=["GET"])
    def get_active_project():
        """返回当前活跃项目路径（用于 UI 展示）。"""
        return jsonify(
            {
                "success": True,
                "path": str(app.config.get("HUGO_ROOT", "")),
            }
        )

    @project_init_bp.route("/active/reset", methods=["POST"])
    def reset_active_project():
        """清除持久化的活跃项目，使下一次启动回退到 env/default HUGO_ROOT。"""
        ActiveProjectRegistry(
            Path(app.root_path) / "data" / "active_project.txt"
        ).clear()
        return jsonify({"success": True, "message": "已清除持久化记录"})

    @project_init_bp.route("/clean-layouts", methods=["POST"])
    def clean_placeholder_layouts():
        """
        删除当前活跃项目根目录下的占位 ``layouts/``，让已安装的主题接管渲染。

        用于修复 init 早期版本（未自动清理 layouts）创建出来的"毛坯"站点。
        """
        from services.project_init_service import ProjectInitService

        hugo_root = Path(app.config.get("HUGO_ROOT", ""))
        if not hugo_root.is_dir():
            return jsonify({"success": False, "message": "活跃项目路径无效"}), 400

        themes_dir = hugo_root / "themes"
        if not themes_dir.is_dir() or not any(themes_dir.iterdir()):
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "未检测到任何已安装主题，保留占位 layouts",
                    }
                ),
                400,
            )

        ProjectInitService._remove_default_layouts(hugo_root)
        return jsonify(
            {
                "success": True,
                "message": f"已清理占位 layouts，主题接管渲染: {hugo_root}",
            }
        )

    return project_init_bp
