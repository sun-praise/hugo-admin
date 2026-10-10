# coding: utf-8 -*-
"""Selection assist endpoints — streaming translate/polish/review over SSE.

Each ``POST /api/ai/assist/<task>`` call runs one no-tools streaming LLM
request via ``AIService.quick_stream``. Deltas are forwarded to the client
as ``delta`` SSE events; when the stream ends, the accumulated text is
parsed and normalized (``services/assist_service``) and delivered as a
``done`` event carrying the task-shaped JSON contract. Unparseable output
terminates the stream with an ``error`` event instead of a fake success.
"""

import asyncio
import contextlib
import json
import logging
import queue
import threading
from concurrent.futures import ThreadPoolExecutor

from flask import Blueprint, Response, jsonify, request

from routes.ai_routes import DONE_SENTINEL, _run_async_in_thread
from services.assist_service import (
    SUPPORTED_TASKS,
    AssistNormalizeError,
    build_messages,
    extract_json,
    normalize_for,
)

logger = logging.getLogger(__name__)

# Dedicated multi-worker executor. The chat routes share a single-worker
# executor; reusing it would serialize the three parallel assist tasks and
# queue them behind AIChat streams. Each worker runs one ``anyio.run`` loop,
# so cancel scopes still never cross threads.
_assist_executor = ThreadPoolExecutor(max_workers=6, thread_name_prefix="assist-async-")

# How often produce() re-checks the disconnect flag between deltas.
_STOP_POLL_SECONDS = 0.2

MAX_SELECTED_TEXT = 5000
MAX_CONTEXT = 2000
MAX_FULL_TEXT = 30000


def _sse_event(payload: dict) -> str:
    return f"data: {json.dumps(payload, ensure_ascii=False)}\n\n"


def stream_assist_as_sse_sync(
    ai_service,
    task: str,
    *,
    selected_text: str,
    context_before: str,
    context_after: str,
    full_text: str,
):
    """Bridge ``AIService.quick_stream`` into a synchronous SSE generator."""
    q = queue.Queue(maxsize=200)
    stop_flag = threading.Event()

    async def produce():
        system_prompt, user_prompt = build_messages(
            task,
            selected_text=selected_text,
            context_before=context_before,
            context_after=context_after,
            full_text=full_text,
        )
        yield _sse_event(
            {"type": "start", "task": task, "model": ai_service.model_name}
        )

        # quick_stream has no external cancellation hook, so it runs as a
        # pump task feeding an asyncio queue; produce() polls stop_flag while
        # waiting and cancels the pump on client disconnect. Cancellation
        # unwinds quick_stream's ``async with`` and tears down the SDK
        # subprocess instead of pinning a worker until the idle timeout.
        deltas: asyncio.Queue = asyncio.Queue()
        parts: list[str] = []
        pump_error: list[BaseException] = []

        async def pump():
            try:
                async for delta in ai_service.quick_stream(system_prompt, user_prompt):
                    await deltas.put(delta)
            except BaseException as e:
                # 上游异常（超时/SDK 失败）不能被吞掉，否则会伪装成
                # “输出不含 JSON”；先记录，队列读空后由 produce 重抛。
                if not isinstance(e, asyncio.CancelledError):
                    pump_error.append(e)
                raise
            finally:
                await deltas.put(None)

        pump_task = asyncio.create_task(pump())
        try:
            while True:
                if stop_flag.is_set():
                    pump_task.cancel()
                    return
                try:
                    item = await asyncio.wait_for(
                        deltas.get(), timeout=_STOP_POLL_SECONDS
                    )
                except asyncio.TimeoutError:
                    continue
                if item is None:
                    break
                parts.append(item)
                yield _sse_event({"type": "delta", "task": task, "text": item})
        finally:
            if not pump_task.done():
                pump_task.cancel()
            with contextlib.suppress(BaseException):
                await pump_task

        if pump_error:
            raise pump_error[0]

        raw = "".join(parts)
        try:
            data = extract_json(raw)
            if data is None:
                raise AssistNormalizeError("model output contains no JSON object")
            result = normalize_for(task, data)
        except AssistNormalizeError as e:
            logger.warning("assist %s normalization failed: %s", task, e)
            yield _sse_event({"type": "error", "task": task, "error": str(e)})
            return

        yield _sse_event({"type": "done", "task": task, "result": result})

    future = _assist_executor.submit(_run_async_in_thread, produce, q, stop_flag)

    try:
        while True:
            try:
                item = q.get(timeout=0.1)
            except queue.Empty:
                if future.done():
                    exc = future.exception()
                    if exc:
                        logger.error("assist %s stream failed: %s", task, exc)
                        yield _sse_event(
                            {"type": "error", "task": task, "error": str(exc)}
                        )
                    break
                continue

            if item is DONE_SENTINEL:
                break
            yield item
    except GeneratorExit:
        stop_flag.set()
        raise
    finally:
        stop_flag.set()


def register_assist_routes(ai_service_factory):
    """Register selection-assist routes.

    ``ai_service_factory`` mirrors the pattern used by the inline-edit and
    chat routes: a callable returning the current ``AIService`` instance.
    """

    assist_bp = Blueprint("selection_assist", __name__, url_prefix="/api/ai")

    @assist_bp.route("/assist/<task>", methods=["POST"])
    def assist(task):
        if task not in SUPPORTED_TASKS:
            return (
                jsonify({"success": False, "message": f"unsupported task: {task}"}),
                404,
            )

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

        data = request.get_json(silent=True) or {}
        selected_text = data.get("selected_text")
        if not isinstance(selected_text, str) or not selected_text.strip():
            return (
                jsonify({"success": False, "message": "缺少 selected_text"}),
                400,
            )
        if len(selected_text) > MAX_SELECTED_TEXT:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": f"selected_text 超过上限 {MAX_SELECTED_TEXT} 字符",
                    }
                ),
                400,
            )

        context_before = data.get("context_before") or ""
        context_after = data.get("context_after") or ""
        for name, value in (
            ("context_before", context_before),
            ("context_after", context_after),
        ):
            if not isinstance(value, str) or len(value) > MAX_CONTEXT:
                return (
                    jsonify(
                        {
                            "success": False,
                            "message": f"{name} 必须为不超过 {MAX_CONTEXT} 字符的字符串",
                        }
                    ),
                    400,
                )

        full_text = data.get("full_text") or ""
        if task == "review":
            if not isinstance(full_text, str) or not full_text.strip():
                return (
                    jsonify(
                        {"success": False, "message": "review 任务必须提供 full_text"}
                    ),
                    400,
                )
            # Reject rather than truncate: an opinion formed on half a document
            # would read as authoritative while being misleading.
            if len(full_text) > MAX_FULL_TEXT:
                return (
                    jsonify(
                        {
                            "success": False,
                            "message": f"full_text 超过上限 {MAX_FULL_TEXT} 字符，"
                            "请缩短文章后再试",
                        }
                    ),
                    400,
                )

        return Response(
            stream_assist_as_sse_sync(
                ai_service,
                task,
                selected_text=selected_text,
                context_before=context_before,
                context_after=context_after,
                full_text=full_text,
            ),
            mimetype="text/event-stream",
        )

    return assist_bp
