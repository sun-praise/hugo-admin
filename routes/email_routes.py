# coding: utf-8
"""
邮件推送相关路由
"""

from flask import Blueprint, jsonify, request

from services.email_service import EmailService

# 创建 Blueprint
bp = Blueprint("email", __name__, url_prefix="/api/email")


def register_email_routes():
    """注册邮件推送路由（EmailService 按请求创建，无需外部依赖）"""

    @bp.route("/push-latest", methods=["POST"])
    def email_push_latest():
        """推送最新文章到订阅者"""
        try:
            data = request.get_json() or {}
            debug_mode = data.get("debug_mode", False)
            force = data.get("force", False)

            email_service = EmailService(debug_mode=debug_mode)
            result = email_service.push_latest(force=force)

            status_code = 200 if result.get("success") else 400
            return jsonify(result), status_code

        except FileNotFoundError as e:
            return (
                jsonify({"success": False, "message": f"配置文件错误: {str(e)}"}),
                500,
            )
        except Exception as e:
            return jsonify({"success": False, "message": f"推送失败: {str(e)}"}), 500

    @bp.route("/push-article", methods=["POST"])
    def email_push_article():
        """推送指定文章到订阅者"""
        try:
            data = request.get_json() or {}
            url = data.get("url")
            debug_mode = data.get("debug_mode", False)
            force = data.get("force", False)

            if not url:
                return jsonify({"success": False, "message": "缺少文章 URL 参数"}), 400

            email_service = EmailService(debug_mode=debug_mode)
            result = email_service.push_article(url, force=force)

            status_code = 200 if result.get("success") else 400
            return jsonify(result), status_code

        except FileNotFoundError as e:
            return (
                jsonify({"success": False, "message": f"配置文件错误: {str(e)}"}),
                500,
            )
        except Exception as e:
            return jsonify({"success": False, "message": f"推送失败: {str(e)}"}), 500

    @bp.route("/preview-latest")
    def email_preview_latest():
        """预览最新文章邮件（不发送）"""
        try:
            email_service = EmailService()
            result = email_service.preview_latest()

            status_code = 200 if result.get("success") else 400
            return jsonify(result), status_code

        except FileNotFoundError as e:
            return (
                jsonify({"success": False, "message": f"配置文件错误: {str(e)}"}),
                500,
            )
        except Exception as e:
            return jsonify({"success": False, "message": f"预览失败: {str(e)}"}), 500

    @bp.route("/preview-article")
    def email_preview_article():
        """预览指定文章邮件（不发送）"""
        try:
            url = request.args.get("url")
            if not url:
                return jsonify({"success": False, "message": "缺少文章 URL 参数"}), 400

            email_service = EmailService()
            result = email_service.preview_article(url)

            status_code = 200 if result.get("success") else 400
            return jsonify(result), status_code

        except FileNotFoundError as e:
            return (
                jsonify({"success": False, "message": f"配置文件错误: {str(e)}"}),
                500,
            )
        except Exception as e:
            return jsonify({"success": False, "message": f"预览失败: {str(e)}"}), 500

    return bp
