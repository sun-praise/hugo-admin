# coding: utf-8
"""
文章发布、Git 状态、系统发布相关路由
"""
from datetime import datetime

from flask import Blueprint, jsonify, request

# 创建 Blueprint
bp = Blueprint("publish", __name__)


def register_publish_routes(registry):
    """
    注册文章发布、Git 状态、系统发布路由

    :param registry: ServiceRegistry 实例
    :return: Blueprint
    """

    @bp.route("/api/article/publish", methods=["POST"])
    def publish_article():
        """发布单个文章"""
        data = request.get_json()

        if not data or "file_path" not in data:
            return (
                jsonify(
                    {
                        "success": False,
                        "error": "缺少 file_path 参数",
                        "error_code": "MISSING_PARAMETER",
                    }
                ),
                400,
            )

        file_path = data["file_path"]
        success, message, operation_id = registry.post_service.publish_article(
            file_path
        )

        if success:
            return jsonify(
                {
                    "success": True,
                    "message": message,
                    "operation_id": operation_id,
                    "article_path": file_path,
                    "draft_status_changed": True,
                    "published_at": datetime.now().isoformat() + "Z",
                }
            )
        else:
            # 根据错误消息返回适当的 HTTP 状态码
            if "不存在" in message:
                status_code = 404
            elif "已经发布" in message or "访问被拒绝" in message:
                status_code = 409
            else:
                status_code = 400

            return (
                jsonify(
                    {"success": False, "error": message, "error_code": "PUBLISH_FAILED"}
                ),
                status_code,
            )

    @bp.route("/api/article/status")
    def get_article_status():
        """获取文章发布状态"""
        file_path = request.args.get("file_path")

        if not file_path:
            return (
                jsonify(
                    {
                        "success": False,
                        "error": "缺少 file_path 参数",
                        "error_code": "MISSING_PARAMETER",
                    }
                ),
                400,
            )

        status = registry.post_service.get_publish_status(file_path)

        if "error" in status:
            status_code = 404 if "不存在" in status["error"] else 400
            return (
                jsonify(
                    {
                        "success": False,
                        "error": status["error"],
                        "error_code": "STATUS_CHECK_FAILED",
                    }
                ),
                status_code,
            )

        return jsonify({"success": True, "status": status})

    @bp.route("/api/article/status/bulk", methods=["POST"])
    def get_bulk_article_status():
        """批量获取文章发布状态"""
        data = request.get_json()

        if not data or "file_paths" not in data:
            return (
                jsonify(
                    {
                        "success": False,
                        "error": "缺少 file_paths 参数",
                        "error_code": "MISSING_PARAMETER",
                    }
                ),
                400,
            )

        file_paths = data["file_paths"]
        results = []

        for file_path in file_paths:
            status = registry.post_service.get_publish_status(file_path)
            results.append({"file_path": file_path, "status": status})

        return jsonify({"success": True, "results": results, "count": len(results)})

    @bp.route("/api/article/publish/bulk", methods=["POST"])
    def bulk_publish_articles():
        """批量发布文章"""
        data = request.get_json()

        if not data or "file_paths" not in data:
            return (
                jsonify(
                    {
                        "success": False,
                        "error": "缺少 file_paths 参数",
                        "error_code": "MISSING_PARAMETER",
                    }
                ),
                400,
            )

        file_paths = data["file_paths"]

        try:
            result = registry.post_service.bulk_publish_articles(file_paths)

            # 根据结果返回适当的 HTTP 状态码
            if result["success"] or not result["failed_count"]:
                status_code = 200
            elif result["failed_count"] > 0 and result["published_count"] > 0:
                status_code = 207  # Multi-Status
            else:
                status_code = 400

            return jsonify(result), status_code
        except Exception as e:
            return (
                jsonify(
                    {
                        "success": False,
                        "error": f"批量发布失败: {str(e)}",
                        "error_code": "BULK_PUBLISH_FAILED",
                    }
                ),
                500,
            )

    # ============ Git / 系统发布相关 API ============

    @bp.route("/api/git/status")
    def git_status():
        """获取 Git 仓库状态"""
        try:
            status = registry.git_service.get_status()
            return jsonify(status)
        except Exception as e:
            return (
                jsonify({"success": False, "message": f"获取 Git 状态失败: {str(e)}"}),
                500,
            )

    @bp.route("/api/git/commits")
    def git_commits():
        """获取最近的提交记录"""
        try:
            count = request.args.get("count", 10, type=int)
            result = registry.git_service.get_recent_commits(count)
            return jsonify(result)
        except Exception as e:
            return (
                jsonify({"success": False, "message": f"获取提交记录失败: {str(e)}"}),
                500,
            )

    @bp.route("/api/publish/system", methods=["POST"])
    def publish_system():
        """系统发布 - 执行 git add, commit, push 完整流程"""
        try:
            data = request.get_json() or {}
            commit_message = data.get("message")

            result = registry.git_service.publish_system(commit_message)

            if result["success"]:
                return jsonify(result), 200
            else:
                return jsonify(result), 400

        except Exception as e:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": f"系统发布失败: {str(e)}",
                        "steps": {},
                    }
                ),
                500,
            )

    return bp
