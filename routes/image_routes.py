# coding: utf-8
"""
图片上传与管理、AI 封面生成路由
"""

import logging
from pathlib import Path

from flask import Blueprint, current_app, jsonify, request

bp = Blueprint("images", __name__)

logger = logging.getLogger(__name__)


def _try_plugin_upload(registry, file_storage, article_path):
    """Attempt to upload via a plugin with image_upload capability.

    Returns the URL string on success, or None to signal fallback.
    """
    plugin_manager = getattr(registry, "plugin_manager", None)
    if plugin_manager is None:
        return None

    # Find a running plugin with image_upload capability
    target = None
    for info in plugin_manager.list_plugins():
        if (
            info.get("status") == "running"
            and info.get("enabled")
            and "image_upload" in info.get("capabilities", [])
        ):
            target = info
            break

    if target is None:
        return None

    stub = plugin_manager.get_image_uploader_stub(target["name"])
    if stub is None:
        return None

    try:
        from proto import plugin_pb2

        file_bytes = file_storage.read()
        filename = file_storage.filename or "upload"
        content_type = file_storage.content_type or "application/octet-stream"
        chunk_size = 64 * 1024  # 64 KiB

        def chunk_iterator():
            offset = 0
            total = len(file_bytes)
            while offset < total:
                end = min(offset + chunk_size, total)
                is_last = end >= total
                chunk = plugin_pb2.ImageUploadChunk(
                    data=file_bytes[offset:end],
                    is_last=is_last,
                )
                # Set metadata on first chunk
                if offset == 0:
                    chunk.filename = filename
                    chunk.mime_type = content_type
                    chunk.article_path = article_path or ""
                yield chunk
                offset = end

        response = stub.Upload(chunk_iterator(), timeout=30)
        if response.success:
            return response.url
        else:
            logger.warning("Plugin image upload failed: %s", response.message)
            return None
    except Exception as e:
        logger.warning("Plugin image upload error: %s", e)
        return None


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

        # Try plugin image upload first
        plugin_url = _try_plugin_upload(registry, file, article_path)
        if plugin_url is not None:
            return jsonify(
                {"success": True, "url": plugin_url, "message": "图片上传成功"}
            )

        # Fallback: local save
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
