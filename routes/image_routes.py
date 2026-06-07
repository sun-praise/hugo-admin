# coding: utf-8
"""
图片上传与管理、AI 封面生成路由
"""

from pathlib import Path

from flask import Blueprint, current_app, jsonify, request

bp = Blueprint("images", __name__)


def register_image_routes(registry):
    """
    注册图片与 frontmatter 相关路由

    Args:
        registry: ServiceRegistry 实例
    """

    @bp.route("/api/image/upload", methods=["POST"])
    def upload_image():
        """上传图片到文章目录"""
        if "file" not in request.files:
            return jsonify({"success": False, "message": "没有文件"}), 400

        file = request.files["file"]
        article_path = request.form.get("article_path")

        if not article_path:
            return jsonify({"success": False, "message": "缺少文章路径"}), 400

        if file.filename == "":
            return jsonify({"success": False, "message": "文件名为空"}), 400

        # 检查文件类型
        allowed_extensions = {"png", "jpg", "jpeg", "gif", "svg", "webp"}
        filename = file.filename or ""
        ext = filename.rsplit(".", 1)[1].lower() if "." in filename else ""
        if ext not in allowed_extensions:
            return (
                jsonify({"success": False, "message": f"不支持的文件类型: {ext}"}),
                400,
            )

        success, result = registry.post_service.save_image(article_path, file)
        if success:
            return jsonify({"success": True, "url": result, "message": "图片上传成功"})
        else:
            return jsonify({"success": False, "message": result}), 500

    @bp.route("/api/image/list", methods=["POST"])
    def list_images():
        """列出文章目录下的所有图片"""
        data = request.get_json()
        article_path = data.get("article_path")

        if not article_path:
            return jsonify({"success": False, "message": "缺少文章路径"}), 400

        success, result = registry.post_service.list_images(article_path)

        if success:
            return jsonify({"success": True, "images": result})
        else:
            return jsonify({"success": False, "message": result}), 500

    @bp.route("/api/image/generate-cover", methods=["POST"])
    def generate_cover():
        """根据文章内容生成封面图片"""
        data = request.get_json()
        article_path = data.get("article_path")
        title = data.get("title", "")
        description = data.get("description", "")
        article_content = data.get("content", "")

        if not article_path:
            return jsonify({"success": False, "message": "缺少文章路径"}), 400

        api_key = current_app.config.get("OPENROUTER_API_KEY", "")
        model = current_app.config.get("IMAGE_GEN_MODEL", "")

        if not api_key:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "OPENROUTER_API_KEY 未配置，请设置环境变量",
                    }
                ),
                400,
            )

        from services.image_gen_service import (
            generate_cover_image,
            save_generated_image,
        )

        ok, result = generate_cover_image(
            title=title,
            description=description,
            content=article_content,
            api_key=api_key,
            model=model,
        )

        if not ok:
            return jsonify({"success": False, "message": result}), 500

        content_dir = Path(current_app.config["CONTENT_DIR"])
        save_ok, save_result = save_generated_image(article_path, result, content_dir)

        if not save_ok:
            return jsonify({"success": False, "message": save_result}), 500

        if registry.post_service.cache_service:
            abs_path = str(
                Path(article_path)
                if Path(article_path).is_absolute()
                else content_dir / article_path
            )
            registry.post_service.cache_service.invalidate_post(abs_path)

        return jsonify(
            {"success": True, "url": save_result, "message": "封面图片生成成功"}
        )

    return bp
