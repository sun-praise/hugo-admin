# coding: utf-8
"""
Inline AI edit rewrite endpoint.

A lightweight non-streaming LLM call that rewrites a small Markdown fragment
selected in the editor. Backed by ``AIService.quick_rewrite`` (Claude Agent
SDK with tools disabled).
"""

import logging
import re

from flask import Blueprint, jsonify, request

from services.ai_service import InlineEditEmptyResultError, InlineEditTimeoutError

logger = logging.getLogger(__name__)


def _looks_like_markdown(line: str) -> bool:
    """A line is treated as Markdown if it carries any Markdown syntax marker.

    Blank lines count as Markdown (they're neutral).  Plain prose — including
    Chinese sentences without any markdown marker — does NOT count, because
    the inline-edit result is allowed to be plain prose and stripping it
    would discard the model's actual answer.
    """
    s = line.strip()
    if not s:
        return True
    return bool(re.search(r"(`|#|\*\*|\[|>|\* |- |\+ |\d+\. )", s))


def _unwrap_single_fence(text: str) -> tuple[str, bool]:
    """If ``text`` is a single fenced code block, return its contents.

    Returns ``(new_text, fired)``. ``fired`` is True when the unwrap actually
    applied. The whole-document case where the model wrapped the entire
    response in ```markdown ... ``` is the one we unwrap; a single block
    surrounded by other text is left alone.
    """
    stripped = text.strip()
    if not stripped.startswith("```"):
        return text, False
    if not stripped.endswith("```"):
        return text, False
    body = stripped[3:-3]
    # Detect a single fence: no inner ``` fences.
    if "```" in body:
        return text, False
    # Drop optional language tag on the first line ("```markdown\n...\n```").
    lines = body.splitlines()
    if lines and lines[0].strip() and not _looks_like_markdown(lines[0]):
        # First line is just a language identifier like 'markdown'.
        if re.match(r"^[a-zA-Z0-9_+\-]+$", lines[0].strip()):
            lines = lines[1:]
    inner = "\n".join(lines).strip()
    if not inner:
        return text, False
    return inner, True


def _post_process(raw: str) -> tuple[str, list[str]]:
    """Run the post-processing fallbacks. Returns ``(clean, log_reasons)``.

    We only handle the case where the model wrapped the whole response in a
    single fenced code block.  We do NOT strip "leading non-Markdown lines" —
    that heuristic is unsafe for plain-prose selections (Chinese sentences
    without any markdown marker) and was the source of false negatives.
    """
    reasons: list[str] = []
    text = raw.strip()
    text, fired = _unwrap_single_fence(text)
    if fired:
        reasons.append("unwrapped_single_fence")
    return text, reasons


def _build_prompts(
    selected_text: str,
    instruction: str,
    context_before: str,
    context_after: str,
) -> tuple[str, str]:
    system_prompt = (
        "你是一个 Markdown 改写助手。"
        "用户会给你一段 Markdown 片段和它前后的上下文，以及改写指令。"
        "你只输出改写后的 Markdown 片段本身，不要任何解释、前缀、后缀、"
        "代码块包裹或换行声明。"
    )
    user_prompt = (
        f"上下文（前）：\n{context_before}\n\n"
        f"需要改写的片段：\n{selected_text}\n\n"
        f"上下文（后）：\n{context_after}\n\n"
        f"改写指令：{instruction}"
    )
    return system_prompt, user_prompt


def register_inline_edit_routes(ai_service_factory):
    """Register the inline-edit blueprint.

    ``ai_service_factory`` mirrors the existing pattern used by
    ``register_ai_routes`` — a no-arg callable that returns the current
    ``AIService`` (or its ``_DisabledAIService`` mock when the API key is
    unset).
    """
    inline_edit_bp = Blueprint("inline_edit", __name__, url_prefix="/api/ai")

    @inline_edit_bp.route("/inline-edit", methods=["POST"])
    def inline_edit():
        data = request.get_json(silent=True) or {}
        selected_text = (data.get("selected_text") or "").strip()
        instruction = (data.get("instruction") or "").strip()
        context_before = (data.get("context_before") or "").strip()
        context_after = (data.get("context_after") or "").strip()

        if not selected_text:
            return (
                jsonify({"success": False, "message": "缺少 selected_text"}),
                400,
            )
        if not instruction:
            return (
                jsonify({"success": False, "message": "缺少 instruction"}),
                400,
            )

        ai_service = ai_service_factory()
        if not getattr(ai_service, "enabled", False):
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

        system_prompt, user_prompt = _build_prompts(
            selected_text, instruction, context_before, context_after
        )

        import anyio

        try:
            revised_text = anyio.run(
                ai_service.quick_rewrite, system_prompt, user_prompt
            )
        except InlineEditEmptyResultError:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "改写失败：模型无输出",
                    }
                ),
                500,
            )
        except InlineEditTimeoutError:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "改写超时，请稍后重试",
                    }
                ),
                504,
            )
        except RuntimeError as e:
            logger.warning("quick_rewrite runtime error: %s", e)
            return (
                jsonify({"success": False, "message": str(e)}),
                500,
            )
        except Exception as e:  # noqa: BLE001
            logger.exception("quick_rewrite failed")
            return (
                jsonify(
                    {
                        "success": False,
                        "message": f"改写失败：{e}",
                    }
                ),
                500,
            )

        cleaned, reasons = _post_process(revised_text)
        if reasons:
            logger.warning(
                "inline-edit post-processing fired: %s | raw=%r",
                ",".join(reasons),
                revised_text[:200],
            )
            revised_text = cleaned
        if not revised_text.strip():
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "改写失败：模型无输出",
                    }
                ),
                500,
            )

        return jsonify(
            {
                "success": True,
                "revised_text": revised_text,
                "model": getattr(ai_service, "model_name", "unknown"),
            }
        )

    return inline_edit_bp
