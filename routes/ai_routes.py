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
                # 使用 run_sync 确保工具调用被正确执行
                # run_stream_sync 会在遇到第一个文本输出时停止，不执行后续工具调用
                result = ai_service.agent.run_sync(
                    message, deps=ai_service.deps, message_history=history
                )

                # 获取最终输出文本
                output_text = result.output if hasattr(result, 'output') else str(result.data)

                # 模拟流式输出 - 按句子/段落分块发送
                # 这样前端仍然能获得渐进式的用户体验
                chunks = []
                current_chunk = ""
                for char in output_text:
                    current_chunk += char
                    # 在标点符号或换行处分块
                    if char in "。！？\n" or (char in "，、；：" and len(current_chunk) > 20):
                        chunks.append(current_chunk)
                        current_chunk = ""
                if current_chunk:
                    chunks.append(current_chunk)

                # 流式发送每个块
                for chunk in chunks:
                    yield f"data: {chunk}\n\n"

                # 流结束后发送完成标记
                yield "data: [DONE]\n\n"
            except Exception as e:
                # 发送错误信息
                error_msg = json.dumps({"error": str(e)})
                yield f"data: {error_msg}\n\n"
                yield "data: [DONE]\n\n"

        return Response(generate(), mimetype="text/event-stream")

    return ai_bp
