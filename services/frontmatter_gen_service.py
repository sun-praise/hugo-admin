import json
import logging
import time

import requests

logger = logging.getLogger(__name__)

MAX_TAGS = 5
MAX_CATEGORIES = 2
CONTENT_SNIPPET_LEN = 2000
_CACHE_TTL = 1800

_fm_cache: dict[str, tuple[float, dict]] = {}


def _sanitize_frontmatter(fm: dict) -> dict:
    """Validate and constrain AI-generated frontmatter fields."""
    result = {}

    desc = fm.get("description")
    if isinstance(desc, str) and desc.strip():
        result["description"] = desc.strip()[:500]

    tags = fm.get("tags")
    if isinstance(tags, list):
        result["tags"] = [
            str(t).strip() for t in tags if isinstance(t, str) and t.strip()
        ][:MAX_TAGS]

    categories = fm.get("categories")
    if isinstance(categories, list):
        result["categories"] = [
            str(c).strip() for c in categories if isinstance(c, str) and c.strip()
        ][:MAX_CATEGORIES]

    return result


def generate_frontmatter(
    content: str,
    api_key: str,
    base_url: str,
    model: str,
) -> tuple[bool, dict | str]:
    """Use AI to generate frontmatter suggestions from article content.

    Returns (success, frontmatter_dict_or_error_message).
    """
    if not api_key:
        return False, "AI API Key 未配置"

    snippet = content[:CONTENT_SNIPPET_LEN].strip()
    if not snippet:
        return False, "文章内容为空"

    cache_key = str(hash(snippet))
    cached = _fm_cache.get(cache_key)
    if cached and (time.time() - cached[0]) < _CACHE_TTL:
        return True, cached[1]

    prompt = (
        "根据以下文章内容，生成合适的 Hugo frontmatter 字段。\n"
        "只返回一个 JSON 对象，包含以下键（均为可选）：\n"
        '- "description": 文章摘要，中文，1-2 句话 (string)\n'
        '- "tags": 相关标签数组 (array of strings, 最多 5 个)\n'
        '- "categories": 分类数组 (array of strings, 最多 2 个)\n\n'
        f"文章内容:\n---\n{snippet}\n---\n\n"
        "只返回 JSON 对象，不要 markdown 代码块，不要解释。"
    )

    url = f"{base_url.rstrip('/')}/chat/completions"
    payload = {
        "model": model,
        "messages": [{"role": "user", "content": prompt}],
        "temperature": 0.3,
    }
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    text = ""
    try:
        resp = requests.post(url, json=payload, headers=headers, timeout=60)
        resp.raise_for_status()
        data = resp.json()

        text = data["choices"][0]["message"]["content"].strip()

        if text.startswith("```"):
            lines = text.split("\n")
            inner = "\n".join(lines[1:])
            text = inner.rsplit("```", 1)[0].strip()

        fm = json.loads(text)
        if not isinstance(fm, dict):
            return False, "AI 返回了非对象类型"

        result = _sanitize_frontmatter(fm)
        _fm_cache[cache_key] = (time.time(), result)
        return True, result

    except json.JSONDecodeError:
        logger.warning("AI returned invalid JSON: %s", text[:200])
        return False, "AI 返回了无效的 JSON"
    except requests.exceptions.Timeout:
        return False, "AI 请求超时"
    except Exception:
        logger.exception("Frontmatter generation failed")
        return False, "生成失败，请稍后重试"
