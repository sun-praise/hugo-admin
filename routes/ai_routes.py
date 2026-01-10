# coding: utf-8
"""
AI 助手相关路由
"""

import json
from flask import Blueprint, request, jsonify, Response

# 创建 Blueprint
ai_bp = Blueprint("ai", __name__, url_prefix="/api/ai")


def register_ai_routes(ai_service):
    """
    注册 AI 相关路由

    Args:
        ai_service: AIService 实例
    """

    @ai_bp.route("/chat", methods=["POST"])
    def ai_chat():
        """AI 聊天接口（支持流式响应）"""
        data = request.get_json()
        message = data.get("message")
        history = data.get("history", [])

        if not message:
            return jsonify({"success": False, "message": "缺少消息内容"}), 400

        def generate():
            try:
                # 使用 pydantic-ai 1.x 的同步流式 API
                result = ai_service.agent.run_stream_sync(
                    message, deps=ai_service.deps, message_history=history
                )

                # 流式输出文本内容
                for chunk in result.stream_text(delta=True):
                    yield f"data: {chunk}\n\n"

                # 流结束后发送完成标记
                yield "data: [DONE]\n\n"
            except Exception as e:
                # 发送错误信息
                error_msg = json.dumps({"error": str(e)})
                yield f"data: {error_msg}\n\n"

        return Response(generate(), mimetype="text/event-stream")

    return ai_bp
