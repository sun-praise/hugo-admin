# coding: utf-8
"""
文件操作相关路由
"""

import logging
from pathlib import Path

from flask import Blueprint, current_app, jsonify, request

bp = Blueprint("files", __name__)

logger = logging.getLogger(__name__)


def register_file_routes(post_service, ref_service):
    """注册文件操作路由，依赖 post_service 和 ref_service"""

    @bp.route("/api/file/read", methods=["POST"])
    def read_file():
        """读取文件内容"""
        data = request.get_json()
        file_path = data.get("path")

        if not file_path:
            return jsonify({"success": False, "message": "缺少文件路径"}), 400

        success, content = post_service.read_file(file_path)

        if success:
            return jsonify({"success": True, "content": content, "path": file_path})
        else:
            return jsonify({"success": False, "message": content}), 404

    @bp.route("/api/file/read-with-frontmatter", methods=["POST"])
    def read_file_with_frontmatter():
        """读取文件内容，分离 frontmatter 和正文"""
        data = request.get_json()
        file_path = data.get("path")

        if not file_path:
            return jsonify({"success": False, "message": "缺少文件路径"}), 400

        success, content, fm = post_service.read_file_with_frontmatter(file_path)

        if success:
            return jsonify(
                {
                    "success": True,
                    "content": content,
                    "frontmatter": fm,
                    "path": file_path,
                }
            )
        else:
            return jsonify({"success": False, "message": content}), 404

    @bp.route("/api/file/save", methods=["POST"])
    def save_file():
        """保存文件内容"""
        data = request.get_json()
        file_path = data.get("path")
        content = data.get("content")
        frontmatter_data = data.get("frontmatter")

        if not file_path or content is None:
            return jsonify({"success": False, "message": "缺少必要参数"}), 400

        success, message = post_service.save_file(
            file_path, content, frontmatter_data=frontmatter_data
        )

        if success:
            # 增量更新引用关系
            try:
                abs_path = str(
                    Path(file_path)
                    if Path(file_path).is_absolute()
                    else Path(current_app.config["CONTENT_DIR"]) / file_path
                )
                ref_service.update_file(abs_path)
            except Exception as e:
                logger.exception(e)

        return jsonify({"success": success, "message": message}), (
            200 if success else 500
        )

    @bp.route("/api/post/create", methods=["POST"])
    def create_post():
        """创建新文章"""
        data = request.get_json()
        title = data.get("title")

        if not title:
            return jsonify({"success": False, "message": "缺少文章标题"}), 400

        success, result = post_service.create_post(title)

        if success:
            return jsonify({"success": True, "path": result, "message": "文章创建成功"})
        else:
            return jsonify({"success": False, "message": result}), 500

    return bp
