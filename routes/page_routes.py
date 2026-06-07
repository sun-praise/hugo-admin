# coding: utf-8
"""
SPA 页面路由
"""

from pathlib import Path

from flask import Blueprint, current_app, jsonify, send_file, send_from_directory

bp = Blueprint("pages", __name__)


def register_page_routes(app):
    """
    注册 SPA 页面路由。

    Args:
        app: Flask 应用实例（用于 send_from_directory 的 root_path）
    """

    @bp.route("/")
    def index():
        """首页 - 仪表板"""
        return send_file(current_app.config["REACT_INDEX"])

    @bp.route("/posts")
    def posts_page():
        """文章列表页面"""
        return send_file(current_app.config["REACT_INDEX"])

    @bp.route("/editor")
    @bp.route("/editor/<path:file_path>")
    def editor_page(file_path=None):
        """文章编辑器页面"""
        return send_file(current_app.config["REACT_INDEX"])

    @bp.route("/server")
    def server_page():
        """Hugo 服务器控制页面"""
        return send_file(current_app.config["REACT_INDEX"])

    @bp.route("/settings")
    def settings_page():
        """设置页面"""
        return send_file(current_app.config["REACT_INDEX"])

    @bp.route("/test")
    def test_page():
        """测试页面"""
        return send_from_directory(current_app.root_path, "test_editor.html")

    @bp.route("/content/<path:filename>")
    def serve_content_files(filename):
        """提供 content 目录下的静态文件（如图片）"""
        if any(part.startswith(".") for part in Path(filename).parts):
            return jsonify({"success": False, "message": "访问被拒绝"}), 403

        content_dir = current_app.config["CONTENT_DIR"]
        return send_from_directory(content_dir, filename)

    return bp
