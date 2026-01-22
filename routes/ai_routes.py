# coding: utf-8
"""
AI 助手相关路由
"""

import asyncio
import json
import queue
import threading
from flask import Blueprint, request, jsonify, Response
from pydantic_ai.messages import (
    PartDeltaEvent,
    TextPartDelta,
    FunctionToolCallEvent,
    FunctionToolResultEvent,
)
from pydantic_ai import AgentRunResultEvent

# 创建 Blueprint
ai_bp = Blueprint("ai", __name__, url_prefix="/api/ai")

DONE_SENTINEL = object()


def _sse_data_line(text: str) -> str:
    """格式化 SSE 数据行，转义换行符防止破坏 SSE 帧格式"""
    text = text.replace("\r", "").replace("\n", "\\n")
    return f"data: {text}\n\n"


def stream_agent_as_sse_sync(agent, *, message: str, deps, history=None):
    """
    将 PydanticAI agent 的事件流转换为同步 SSE 生成器

    使用后台线程运行异步事件流，通过队列桥接到同步生成器
    这样可以实时流式传输文本内容、工具调用和工具结果

    Args:
        agent: PydanticAI Agent 实例
        message: 用户消息
        deps: Agent 依赖
        history: 消息历史（可选）

    Yields:
        SSE 格式的数据行
    """
    q = queue.Queue(maxsize=200)
    stop_flag = threading.Event()
    history = history or []

    async def produce():
        try:
            async for event in agent.run_stream_events(
                message, deps=deps, message_history=history
            ):
                if stop_flag.is_set():
                    break

                # 1) 流式传输模型文本增量
                if isinstance(event, PartDeltaEvent) and isinstance(
                    event.delta, TextPartDelta
                ):
                    q.put(_sse_data_line(event.delta.content_delta))

                # 2) 流式传输工具调用
                elif isinstance(event, FunctionToolCallEvent):
                    payload = {
                        "type": "tool_call",
                        "tool": event.part.tool_name,
                        "args": event.part.args,
                        "tool_call_id": event.part.tool_call_id,
                    }
                    q.put(_sse_data_line(json.dumps(payload, ensure_ascii=False)))

                # 3) 流式传输工具结果
                elif isinstance(event, FunctionToolResultEvent):
                    payload = {
                        "type": "tool_result",
                        "tool_call_id": event.tool_call_id,
                        "result": event.result.content,
                    }
                    q.put(_sse_data_line(json.dumps(payload, ensure_ascii=False)))

                # 4) 运行完成（保证最终事件）
                elif isinstance(event, AgentRunResultEvent):
                    q.put(DONE_SENTINEL)
        except Exception as e:
            error_payload = json.dumps(
                {"type": "error", "error": str(e)}, ensure_ascii=False
            )
            q.put(_sse_data_line(error_payload))
            q.put(DONE_SENTINEL)

    def thread_main():
        asyncio.run(produce())

    t = threading.Thread(target=thread_main, daemon=True)
    t.start()

    try:
        while True:
            item = q.get()
            if item is DONE_SENTINEL:
                yield "data: [DONE]\n\n"
                break
            yield item  # 已经是 SSE 格式
    except GeneratorExit:
        # 客户端断开连接
        stop_flag.set()
        raise
    finally:
        stop_flag.set()


def register_ai_routes(ai_service):
    """
    注册 AI 相关路由

    Args:
        ai_service: AIService 实例
    """

    @ai_bp.route("/chat", methods=["POST"])
    def ai_chat():
        """AI 聊天接口（支持流式响应，包括工具执行结果）"""
        if not ai_service.enabled:
            return jsonify(
                {
                    "success": False,
                    "message": "AI service is not configured. Set DEEPSEEK_API_KEY to enable.",
                }
            ), 503

        data = request.get_json()
        message = data.get("message")
        history = data.get("history", [])

        if not message:
            return jsonify({"success": False, "message": "缺少消息内容"}), 400

        def generate():
            yield from stream_agent_as_sse_sync(
                ai_service.agent,
                message=message,
                deps=ai_service.deps,
                history=history,
            )

        return Response(generate(), mimetype="text/event-stream")

    return ai_bp
