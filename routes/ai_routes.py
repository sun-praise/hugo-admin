# coding: utf-8
"""
AI 助手相关路由
"""

import json
import queue
import threading
from concurrent.futures import ThreadPoolExecutor

import anyio
from flask import Blueprint, request, jsonify, Response

from claude_agent_sdk import (
    AssistantMessage,
    TextBlock,
    ToolUseBlock,
    ToolResultBlock,
)
from claude_agent_sdk.types import StreamEvent

# 创建 Blueprint
ai_bp = Blueprint("ai", __name__, url_prefix="/api/ai")

DONE_SENTINEL = object()

# 单线程执行器，确保所有异步操作在同一个线程中执行
# 这避免了 anyio cancel scope 跨线程问题
_executor = ThreadPoolExecutor(max_workers=1, thread_name_prefix="ai-async-")


def _sse_data_line(text: str) -> str:
    """格式化 SSE 数据行，转义换行符防止破坏 SSE 帧格式"""
    text = text.replace("\r", "").replace("\n", "\\n")
    return f"data: {text}\n\n"


def _run_async_in_thread(coro_func, q, stop_flag):
    """
    在专用线程中运行异步协程，使用 anyio 作为后端。

    这确保了 cancel scope 在同一个任务/线程中进入和退出，
    解决了 "Attempted to exit cancel scope in a different task" 错误。
    """

    async def runner():
        try:
            async for item in coro_func():
                if stop_flag.is_set():
                    break
                q.put(item)
        except Exception as e:
            error_payload = json.dumps(
                {"type": "error", "error": str(e)}, ensure_ascii=False
            )
            q.put(_sse_data_line(error_payload))
        finally:
            q.put(DONE_SENTINEL)

    # 使用 anyio.run 而不是 asyncio.run，与 claude_agent_sdk 内部一致
    anyio.run(runner)


def stream_agent_as_sse_sync(
    ai_service,
    *,
    message: str,
    history=None,
):
    """
    将 Claude Agent SDK 的事件流转换为同步 SSE 生成器

    使用后台线程运行异步事件流，通过队列桥接到同步生成器
    这样可以实时流式传输文本内容、工具调用和工具结果

    Args:
        ai_service: AIService 实例
        message: 用户消息
        history: 消息历史（可选）

    Yields:
        SSE 格式的数据行
    """
    q = queue.Queue(maxsize=200)
    stop_flag = threading.Event()
    history = history or []

    async def produce():
        """异步生成器：处理 AI 响应流"""
        async for msg in ai_service.chat(message, history=history):
            if stop_flag.is_set():
                break

            # 1) 处理 AssistantMessage: 包含 TextBlock 和 ToolUseBlock
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, TextBlock):
                        yield _sse_data_line(block.text)
                    elif isinstance(block, ToolUseBlock):
                        payload = {
                            "type": "tool_call",
                            "tool": block.name,
                            "args": block.input,
                            "tool_call_id": block.id,
                        }
                        yield _sse_data_line(json.dumps(payload, ensure_ascii=False))

            # 2) 处理工具结果（来自 UserMessage）
            elif hasattr(msg, "content") and isinstance(msg.content, list):
                for block in msg.content:
                    if isinstance(block, ToolResultBlock):
                        payload = {
                            "type": "tool_result",
                            "tool_call_id": block.tool_use_id,
                            "result": block.content,
                        }
                        yield _sse_data_line(json.dumps(payload, ensure_ascii=False))

            # 3) 处理流事件（结束信号）
            elif isinstance(msg, StreamEvent):
                if msg.event.get("event") in ["done", "end"]:
                    break

    # 在线程池中运行异步代码
    future = _executor.submit(_run_async_in_thread, produce, q, stop_flag)

    try:
        while True:
            # 使用超时来检查线程是否仍在运行
            try:
                item = q.get(timeout=0.1)
            except queue.Empty:
                # 检查异步任务是否已完成但队列为空
                if future.done():
                    # 检查是否有异常
                    exc = future.exception()
                    if exc:
                        error_payload = json.dumps(
                            {"type": "error", "error": str(exc)}, ensure_ascii=False
                        )
                        yield _sse_data_line(error_payload)
                    yield "data: [DONE]\n\n"
                    break
                continue

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


def register_ai_routes(ai_service_factory):
    """
    注册 AI 相关路由

    Args:
        ai_service_factory: Callable that returns AIService instance
    """

    @ai_bp.route("/chat", methods=["POST"])
    def ai_chat():
        """AI 聊天接口（支持流式响应，包括工具执行结果）"""
        ai_service = ai_service_factory()
        if not ai_service.enabled:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "AI service not configured. "
                        "Set DEEPSEEK_API_KEY to enable.",
                    }
                ),
                503,
            )

        data = request.get_json()
        message = data.get("message")
        history = data.get("history", [])

        if not message:
            return jsonify({"success": False, "message": "缺少消息内容"}), 400

        def generate():
            yield from stream_agent_as_sse_sync(
                ai_service,
                message=message,
                history=history,
            )

        return Response(generate(), mimetype="text/event-stream")

    return ai_bp
