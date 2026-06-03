import json
import logging

import requests

logger = logging.getLogger(__name__)


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

    snippet = content[:2000].strip()
    if not snippet:
        return False, "文章内容为空"

    prompt = (
        "Based on the following article content, "
        "generate appropriate Hugo frontmatter fields.\n"
        "Return ONLY a JSON object with these keys "
        "(all optional, only if you can determine a good value):\n"
        '- "title": a concise title (string)\n'
        '- "description": a brief summary in Chinese, '
        "1-2 sentences (string)\n"
        '- "tags": array of relevant tags '
        "(array of strings, max 5)\n"
        '- "categories": array of categories '
        "(array of strings, max 2)\n\n"
        f"Article content:\n---\n{snippet}\n---\n\n"
        "Return only the JSON object, "
        "no markdown fences, no explanation."
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

    try:
        resp = requests.post(url, json=payload, headers=headers, timeout=60)
        resp.raise_for_status()
        data = resp.json()

        text = data["choices"][0]["message"]["content"].strip()
        if text.startswith("```"):
            text = text.split("\n", 1)[1].rsplit("```", 1)[0].strip()

        fm = json.loads(text)
        if not isinstance(fm, dict):
            return False, f"AI returned non-object: {type(fm).__name__}"

        return True, fm

    except json.JSONDecodeError:
        return False, f"AI returned invalid JSON: {text[:200]}"
    except requests.exceptions.Timeout:
        return False, "AI 请求超时"
    except Exception as e:
        logger.exception("Frontmatter generation failed")
        return False, f"生成失败: {e}"
