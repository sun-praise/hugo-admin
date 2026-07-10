# coding: utf-8
"""
文章级 TTS（语音播报）路由。

在编辑器中为一篇文章生成语音：读取正文（剥离 frontmatter）→ 调用带
``tts_generation`` 能力的插件 → 后台任务经 Socket.IO 上报进度 → 成功后用
乐观锁把 audio URL 写回 frontmatter。复刻 article_import_service 的后台
任务 + 进度事件模式。
"""

import logging
import uuid

import grpc
from flask import Blueprint, jsonify, request

from proto import plugin_pb2

logger = logging.getLogger(__name__)

# 该插件能力的名字（与 proto / plugin.toml 中的 capabilities 一致）
TTS_CAPABILITY = "tts_generation"

# 写回 frontmatter 时使用的字段名
FM_AUDIO = "audio"
FM_AUDIO_DURATION = "audio_duration_seconds"
FM_AUDIO_FORMAT = "audio_format"
FM_AUDIO_ID = "_tts_audio_id"  # 非公开字段，作为删除托管音频的凭证


def _resolve_plugin(plugin_manager):
    """找到运行中的 TTS 插件，返回 (plugin_info, stub) 或 (None, None)。"""
    target = plugin_manager.find_plugin_with_capability(TTS_CAPABILITY)
    if target is None:
        return None, None
    stub = plugin_manager.get_tts_generator_stub(target["name"])
    return target, stub


def _run_tts_with_emits(
    post_service,
    plugin_manager,
    article_path,
    text,
    options,
    expected_mtime,
    socketio,
    event_scope,
) -> None:
    """后台任务：调插件生成语音并写回 frontmatter，全程经 Socket.IO 上报。"""

    def emit_event(event: str, payload: dict) -> None:
        try:
            socketio.emit(event, {"scope": event_scope, **payload})
        except Exception:
            logger.exception("Socket emit failed for %s", event)

    target, stub = _resolve_plugin(plugin_manager)
    if stub is None:
        emit_event("tts.failed", {"message": "未找到支持 tts_generation 的插件"})
        return

    req = plugin_pb2.TTSRequest(
        text=text,
        voice=options.get("voice", "") or "",
        model=options.get("model", "") or "",
        speed=float(options.get("speed", 1.0)),
        format=options.get("format", "") or "",
        language=options.get("language", "") or "",
        article_path=article_path or "",
    )

    try:
        result = None
        for resp in stub.Generate(req, timeout=300):
            which = resp.WhichOneof("payload")
            if which == "progress":
                p = resp.progress
                emit_event(
                    "tts.progress",
                    {
                        "stage": p.stage,
                        "percent": p.percent,
                        "message": p.message,
                    },
                )
            elif which == "result":
                result = resp.result
    except grpc.RpcError as e:
        emit_event("tts.failed", {"message": f"插件调用失败: {e.details() or e}"})
        return
    except Exception as e:  # noqa: BLE001 — 后台任务不能崩
        emit_event("tts.failed", {"message": f"生成失败: {e}"})
        return

    if result is None or not result.success:
        emit_event(
            "tts.failed",
            {"message": (result.message if result else "插件未返回结果")},
        )
        return

    # 写回 frontmatter（乐观锁）
    ok, body, fm, _mtime = post_service.read_file_with_frontmatter(article_path)
    if not ok:
        emit_event("tts.failed", {"message": "读取文章失败，无法写回 audio 字段"})
        return

    fm[FM_AUDIO] = result.url
    if result.duration_seconds > 0:
        fm[FM_AUDIO_DURATION] = result.duration_seconds
    if result.format:
        fm[FM_AUDIO_FORMAT] = result.format
    if result.audio_id:
        fm[FM_AUDIO_ID] = result.audio_id

    save_ok, save_msg, _new_mtime = post_service.save_file(
        article_path, body, frontmatter_data=fm, expected_mtime=expected_mtime
    )

    if not save_ok:
        # 乐观锁冲突：save_msg 是一个 dict
        if isinstance(save_msg, dict) and save_msg.get("conflict"):
            emit_event(
                "tts.conflict",
                {
                    "message": "文章已被修改，请保存后重试",
                    "current_mtime": save_msg.get("current_mtime"),
                    "url": result.url,
                    "audio_id": result.audio_id,
                },
            )
        else:
            emit_event("tts.failed", {"message": f"写入 frontmatter 失败: {save_msg}"})
        return

    emit_event(
        "tts.done",
        {
            "url": result.url,
            "duration_seconds": result.duration_seconds,
            "format": result.format,
            "audio_id": result.audio_id,
        },
    )


def register_tts_routes(registry):
    """注册文章级 TTS 路由。"""
    bp = Blueprint("tts", __name__)

    @bp.route("/api/article/tts/status", methods=["GET"])
    def tts_status():
        """返回 TTS 能力是否可用（前端据此显隐按钮）。"""
        plugin_manager = getattr(registry, "plugin_manager", None)
        available = False
        name = None
        voices: list[str] = []
        if plugin_manager is not None:
            target = plugin_manager.find_plugin_with_capability(TTS_CAPABILITY)
            if target is not None:
                available = True
                name = target["name"]
                # config_schema 可能声明可选音色列表
                try:
                    schema = plugin_manager.get_config_schema(name) or {}
                    voices = (schema.get("properties") or {}).get(
                        "voices", {}
                    ).get("items", {}).get("enum", [])
                except Exception:  # noqa: BLE001
                    voices = []
        return jsonify({"success": True, "available": available, "plugin": name,
                        "voices": voices})

    @bp.route("/api/article/tts", methods=["POST"])
    def generate_article_tts():
        """为一篇文章生成语音播报。

        入参: {article_path, voice?, model?, speed?, format?, language?, text?}
        - 若提供 ``text`` 则直接用它；否则读取文章正文（剥离 frontmatter）。
        立即返回 event_scope，进度与结果经 Socket.IO（tts.*）上报。
        """
        data = request.get_json(silent=True) or {}
        article_path = data.get("article_path")
        if not article_path:
            return jsonify({"success": False, "message": "缺少文章路径"}), 400

        plugin_manager = getattr(registry, "plugin_manager", None)
        if plugin_manager is None:
            return jsonify({"success": False, "message": "插件系统未初始化"}), 400

        target, stub = _resolve_plugin(plugin_manager)
        if target is None or stub is None:
            return (
                jsonify(
                    {"success": False, "message": "未找到支持 tts_generation 的插件"}
                ),
                400,
            )

        text = data.get("text")
        expected_mtime = None
        if not text:
            ok, body, _fm, mtime = registry.post_service.read_file_with_frontmatter(
                article_path
            )
            if not ok:
                return jsonify({"success": False, "message": "读取文章失败"}), 400
            text = body
            expected_mtime = mtime

        text = (text or "").strip()
        if not text:
            return jsonify({"success": False, "message": "文章内容为空"}), 400

        socketio = getattr(registry, "socketio", None)
        event_scope = str(uuid.uuid4())
        options = {
            k: data.get(k)
            for k in ("voice", "model", "speed", "format", "language")
            if data.get(k) is not None
        }

        if socketio is None:
            # 无 Socket.IO 时退化为同步执行（主要供脚本/测试）
            _run_tts_with_emits(
                registry.post_service,
                plugin_manager,
                article_path,
                text,
                options,
                expected_mtime,
                _NoopSocket(),
                event_scope,
            )
            return jsonify(
                {"success": True, "pending": False, "event_scope": event_scope}
            )

        socketio.start_background_task(
            _run_tts_with_emits,
            registry.post_service,
            plugin_manager,
            article_path,
            text,
            options,
            expected_mtime,
            socketio,
            event_scope,
        )
        return jsonify(
            {"success": True, "pending": True, "event_scope": event_scope}
        )

    @bp.route("/api/article/tts", methods=["DELETE"])
    def delete_article_tts():
        """删除文章的语音：清 frontmatter 音频字段，并通知插件删托管 object。

        入参: {article_path}
        """
        data = request.get_json(silent=True) or {}
        article_path = data.get("article_path")
        if not article_path:
            return jsonify({"success": False, "message": "缺少文章路径"}), 400

        ok, body, fm, _mtime = registry.post_service.read_file_with_frontmatter(
            article_path
        )
        if not ok:
            return jsonify({"success": False, "message": "读取文章失败"}), 400

        audio_id = fm.pop(FM_AUDIO_ID, "")
        for k in (FM_AUDIO, FM_AUDIO_DURATION, FM_AUDIO_FORMAT):
            fm.pop(k, None)

        save_ok, save_msg, _new_mtime = registry.post_service.save_file(
            article_path, body, frontmatter_data=fm
        )
        if not save_ok:
            return jsonify({"success": False, "message": f"清除 audio 字段失败: {save_msg}"}), 500

        # 通知插件删除托管音频（若有凭证）
        if audio_id:
            plugin_manager = getattr(registry, "plugin_manager", None)
            if plugin_manager is not None:
                target, stub = _resolve_plugin(plugin_manager)
                if stub is not None:
                    try:
                        stub.Delete(
                            plugin_pb2.TTSDeleteRequest(audio_id=audio_id),
                            timeout=30,
                        )
                    except Exception as e:  # noqa: BLE001
                        logger.warning("插件删除托管音频失败（已清 frontmatter）: %s", e)

        return jsonify({"success": True, "message": "已删除语音"})

    return bp


class _NoopSocket:
    """无 Socket.IO 时的空实现（同步路径用）。"""

    def emit(self, *_args, **_kwargs):
        return None
