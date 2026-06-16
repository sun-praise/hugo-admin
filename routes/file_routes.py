# coding: utf-8
"""
文件操作相关路由
"""

import logging
import uuid
from pathlib import Path

from flask import Blueprint, current_app, jsonify, request

bp = Blueprint("files", __name__)

logger = logging.getLogger(__name__)


def _form_bool(value, default=True):
    """把表单字符串解析为布尔；缺省/无法识别时返回 default。"""
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    text = str(value).strip().lower()
    if text in ("0", "false", "no", "off"):
        return False
    if text in ("1", "true", "yes", "on"):
        return True
    return default


def register_file_routes(registry, blueprint=None):
    """注册文件操作路由，依赖 registry 中的 post_service 和 ref_service

    blueprint 为 None 时使用模块级默认 Blueprint（生产用法）；测试可注入
    一个全新的 Blueprint，避免在共享的模块级 bp 上重复注册路由。
    """
    blueprint = blueprint if blueprint is not None else bp

    @blueprint.route("/api/file/read", methods=["POST"])
    def read_file():
        """读取文件内容"""
        data = request.get_json()
        file_path = data.get("path")

        if not file_path:
            return jsonify({"success": False, "message": "缺少文件路径"}), 400

        success, content = registry.post_service.read_file(file_path)

        if success:
            return jsonify({"success": True, "content": content, "path": file_path})
        else:
            return jsonify({"success": False, "message": content}), 404

    @blueprint.route("/api/file/read-with-frontmatter", methods=["POST"])
    def read_file_with_frontmatter():
        """读取文件内容，分离 frontmatter 和正文"""
        data = request.get_json()
        file_path = data.get("path")

        if not file_path:
            return jsonify({"success": False, "message": "缺少文件路径"}), 400

        success, content, fm = registry.post_service.read_file_with_frontmatter(
            file_path
        )

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

    @blueprint.route("/api/file/save", methods=["POST"])
    def save_file():
        """保存文件内容"""
        data = request.get_json()
        file_path = data.get("path")
        content = data.get("content")
        frontmatter_data = data.get("frontmatter")

        if not file_path or content is None:
            return jsonify({"success": False, "message": "缺少必要参数"}), 400

        success, message = registry.post_service.save_file(
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
                registry.ref_service.update_file(abs_path)
            except Exception as e:
                logger.exception(e)

        return jsonify({"success": success, "message": message}), (
            200 if success else 500
        )

    @blueprint.route("/api/post/create", methods=["POST"])
    def create_post():
        """创建新文章"""
        data = request.get_json()
        title = data.get("title")

        if not title:
            return jsonify({"success": False, "message": "缺少文章标题"}), 400

        success, result = registry.post_service.create_post(title)

        if success:
            return jsonify({"success": True, "path": result, "message": "文章创建成功"})
        else:
            return jsonify({"success": False, "message": result}), 500

    @blueprint.route("/api/article/import", methods=["POST"])
    def import_article():
        """上传 Markdown 文件，AI 自动补全 frontmatter 与封面后导入为草稿。

        表单字段：
        - file: .md / .markdown 文件（必填）
        - title: 可选标题覆盖
        - generate_frontmatter / generate_cover: 可选布尔开关，默认 true
        """
        if "file" not in request.files:
            return jsonify({"success": False, "message": "缺少文件"}), 400

        file = request.files["file"]
        filename = file.filename or ""
        if not filename:
            return jsonify({"success": False, "message": "文件名为空"}), 400

        ext = filename.rsplit(".", 1)[1].lower() if "." in filename else ""
        if ext not in ("md", "markdown"):
            return (
                jsonify({"success": False, "message": f"不支持的文件类型: {ext}"}),
                400,
            )

        raw = file.read()

        ai_cfg = {
            "api_key": current_app.config.get("AI_API_KEY", ""),
            "base_url": current_app.config.get(
                "AI_BASE_URL", "https://api.deepseek.com"
            ),
            "model": current_app.config.get("AI_MODEL", "deepseek-chat"),
        }
        image_cfg = {
            "api_key": current_app.config.get("OPENROUTER_API_KEY", ""),
            "model": current_app.config.get("IMAGE_GEN_MODEL", ""),
        }

        from services.article_import_service import import_markdown

        result = import_markdown(
            filename,
            raw,
            title=request.form.get("title") or None,
            generate_frontmatter=_form_bool(
                request.form.get("generate_frontmatter"), True
            ),
            generate_cover=_form_bool(request.form.get("generate_cover"), True),
            post_service=registry.post_service,
            ai_cfg=ai_cfg,
            image_cfg=image_cfg,
            socketio=getattr(registry, "socketio", None),
            event_scope=uuid.uuid4().hex,
        )

        if not result.get("path"):
            message = "; ".join(result.get("warnings")) or "导入失败"
            return jsonify({"success": False, "message": message}), 500

        return jsonify({"success": True, **result})

    return blueprint
