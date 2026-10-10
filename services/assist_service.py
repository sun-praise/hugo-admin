# coding: utf-8 -*-
"""Selection assist service — prompts and LLM output normalization.

Ports the task prompts and JSON normalization from the standalone
selection-assist project (server/prompts.ts, normalize.ts, json.ts),
adapted from LaTeX/academic writing to blog Markdown writing.

Every task asks the model for STRICT JSON; ``normalize_for`` then coerces
whatever actually comes back into the shape the frontend panel renders,
tolerating markdown fences, surrounding prose, trailing commas and
near-miss field names.
"""

import json
import re
from typing import Any, Optional

SUPPORTED_TASKS = ("translate", "polish", "review")

STRICT_JSON_ONLY = (
    "只输出严格 JSON：不要 markdown 代码块，不要 JSON 前后的任何说明文字。"
)

POLISH_STYLE_FALLBACK = ("formal", "concise", "restructure")
REVIEW_TYPE_SET = (
    "overclaim",
    "title",
    "terminology",
    "colloquial",
    "structure",
    "other",
)
REVIEW_SEVERITY_SET = ("high", "warn", "info")
# Models trained on other review prompts often emit academic-style severities.
_REVIEW_SEVERITY_ALIASES = {"medium": "warn", "low": "info", "ok": "info"}

_FENCE_RE = re.compile(r"```(?:json)?\s*(.*?)```", re.IGNORECASE | re.DOTALL)
_TRAILING_COMMA_RE = re.compile(r",\s*([}\]])")


class AssistNormalizeError(ValueError):
    """Raised when LLM output cannot be coerced into the task contract."""


def _as_string(value: Any) -> str:
    if isinstance(value, str):
        return value
    if value is None:
        return ""
    return str(value)


def build_messages(
    task: str,
    *,
    selected_text: str,
    context_before: str = "",
    context_after: str = "",
    full_text: str = "",
) -> tuple[str, str]:
    """Build (system_prompt, user_prompt) for one assist task."""
    if task == "translate":
        system = f"""你是一名专业译者，嵌入在博客 Markdown 编辑器中。
任务：翻译 <selection>：英文源文译为简体中文，中文源文译为英文；Markdown 语法、链接与代码块保持原样。
{STRICT_JSON_ONLY}
Schema：
{{"translation": string,
 "hard_words": [{{"word": string, "source": string, "gloss": string}}]}}
- translation：完整译文。
- hard_words：0-6 个读者可能不熟的难词（术语、正式/书面词、易混词）。
  - word：译文中出现的原样子串（界面将据此逐字加下划线）。
  - source：原文中对应的词或短语。
  - gloss：一行简短释义。"""

    elif task == "polish":
        system = f"""你是一名写作编辑，嵌入在博客 Markdown 编辑器中。
任务：仅改写 <selection>，返回恰好 3 个候选。
{STRICT_JSON_ONLY}
Schema：
{{"candidates": [
  {{"style": "formal"|"concise"|"restructure", "note": string, "text": string}}]}}
- formal：更正式书面（动词精确，去口语化与自夸用语）。
- concise：更紧凑，保留全部信息，通常更短。
- restructure：句式重组（如拆长句、调整语序），语义不变。
- note：一句话说明改了什么。
- text：与选区同语言的完整改写文本，Markdown 语法保持原样。"""

    elif task == "review":
        system = f"""你是一名严格的博客文章审校，嵌入在 Markdown 编辑器中。
任务：结合完整 <document> 评审 <selection>，关注以下博客写作问题：
- overclaim：断言过强或无依据的事实性风险（如「最好」「绝对」「所有人都」）；
- title：与文章标题承诺不一致（答非所问、标题党）；
- terminology：术语前后不一致；
- colloquial：口语化或语气不当；
- structure：结构与可读性（段落过长、逻辑跳跃、冗余）。
{STRICT_JSON_ONLY}
Schema：
{{"issues": [
  {{"type": "overclaim"|"title"|"terminology"|"colloquial"|"structure"|"other",
    "severity": "high"|"warn"|"info",
    "comment": string, "before": string|null, "after": string|null}}]}}
- 给出 2-5 条意见；comment 用简体中文撰写，引用讨论中的原文词句。
- 仅当存在局部可修复的问题时给出 before/after：before 必须逐字复制自正文
  （精确子串，编辑器将据此定位替换），after 为替换文本；否则两项均为 null。
- severity：high=事实风险或严重不一致；warn=建议修改；info=提示，至多一条 info 为正面评价。"""

    else:  # pragma: no cover - routes whitelist tasks before reaching here
        raise ValueError(f"unsupported task: {task}")

    parts: list[str] = []
    if task == "review":
        parts.append(f"<document>\n{full_text}\n</document>")
        parts.append(f"<selection>\n{selected_text}\n</selection>")
    else:
        if context_before:
            parts.append(f"<context_before>\n{context_before}\n</context_before>")
        parts.append(f"<selection>\n{selected_text}\n</selection>")
        if context_after:
            parts.append(f"<context_after>\n{context_after}\n</context_after>")

    return system, "\n\n".join(parts)


def extract_json(raw: str) -> Optional[Any]:
    """Extract JSON from arbitrary LLM output.

    Tolerates markdown code fences, prose before/after the JSON object and
    trailing commas. Returns None when no JSON object can be recovered.
    """
    s = (raw or "").strip()
    if not s:
        return None

    fence = _FENCE_RE.search(s)
    if fence:
        s = fence.group(1).strip()

    start = s.find("{")
    end = s.rfind("}")
    if start == -1 or end <= start:
        return None

    body = s[start : end + 1]
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        pass

    body = _TRAILING_COMMA_RE.sub(r"\1", body)
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        return None


def _first_string(mapping: dict, *keys: str) -> str:
    for key in keys:
        if key in mapping:
            value = _as_string(mapping[key]).strip()
            if value:
                return value
    return ""


def normalize_translate(raw: Any) -> Optional[dict]:
    if not isinstance(raw, dict):
        return None
    translation = _first_string(raw, "translation", "zh", "text")
    if not translation:
        return None

    words_raw = raw.get("hard_words")
    if not isinstance(words_raw, list):
        words_raw = raw.get("terms") if isinstance(raw.get("terms"), list) else []

    hard_words = []
    for item in words_raw:
        if not isinstance(item, dict):
            continue
        word = _first_string(item, "word", "label", "zh")
        if not word:
            continue
        hard_words.append(
            {
                "word": word,
                "source": _first_string(item, "source", "en", "orig"),
                "gloss": _first_string(item, "gloss", "note", "def"),
            }
        )
        if len(hard_words) >= 8:
            break

    return {"translation": translation, "hard_words": hard_words}


def normalize_polish(raw: Any) -> Optional[dict]:
    if isinstance(raw, list):
        candidates_raw = raw
    elif isinstance(raw, dict):
        candidates_raw = raw.get("candidates")
        if not isinstance(candidates_raw, list):
            candidates_raw = []
    else:
        return None

    candidates = []
    for item in candidates_raw:
        if not isinstance(item, dict):
            continue
        text = _first_string(item, "text", "rewrite")
        if not text:
            continue
        style = _first_string(item, "style")
        candidates.append(
            {
                "style": style or POLISH_STYLE_FALLBACK[len(candidates)],
                "note": _first_string(item, "note", "reason"),
                "text": text,
            }
        )
        if len(candidates) >= 3:
            break

    # The API contract promises exactly three candidates (formal / concise /
    # restructure). A degraded result would silently break the panel's
    # three-style layout, so fewer valid candidates is a normalize failure
    # and the client can retry.
    if len(candidates) < 3:
        return None
    return {"candidates": candidates}


def normalize_review(raw: Any) -> Optional[dict]:
    if not isinstance(raw, dict):
        return None
    issues_raw = raw.get("issues")
    if not isinstance(issues_raw, list):
        return None

    issues = []
    for item in issues_raw:
        if not isinstance(item, dict):
            continue
        comment = _first_string(item, "comment", "detail", "description", "title")
        if not comment:
            continue

        before = _first_string(item, "before")
        after = _first_string(item, "after")
        fix = item.get("fix")
        if (not before or not after) and isinstance(fix, dict):
            before = before or _first_string(fix, "before", "find")
            after = after or _first_string(fix, "after", "replace")
        if not before or not after:
            before = after = None

        issue_type = _first_string(item, "type", "category")
        severity = _first_string(item, "severity", "sev").lower()
        if severity in _REVIEW_SEVERITY_ALIASES:
            severity = _REVIEW_SEVERITY_ALIASES[severity]
        if severity not in REVIEW_SEVERITY_SET:
            severity = "warn"

        issues.append(
            {
                "type": issue_type if issue_type in REVIEW_TYPE_SET else "other",
                "severity": severity,
                "comment": comment,
                "before": before or None,
                "after": after or None,
            }
        )
        if len(issues) >= 8:
            break

    # An empty issues list is a valid outcome: a clean selection simply has
    # nothing to report, and the panel renders "没有问题".
    return {"issues": issues}


_NORMALIZERS = {
    "translate": normalize_translate,
    "polish": normalize_polish,
    "review": normalize_review,
}


def normalize_for(task: str, raw: Any) -> dict:
    """Coerce extracted JSON into the task contract or raise AssistNormalizeError."""
    normalizer = _NORMALIZERS.get(task)
    if normalizer is None:
        raise AssistNormalizeError(f"unsupported task: {task}")
    result = normalizer(raw)
    if result is None:
        raise AssistNormalizeError(
            f"model output does not match the {task} JSON contract"
        )
    return result
