import base64
import logging
import os
import time
from pathlib import Path

import requests

logger = logging.getLogger(__name__)

DEFAULT_MODEL = "google/gemini-3.1-flash-image-preview"
OPENROUTER_API_URL = "https://openrouter.ai/api/v1/chat/completions"


def generate_cover_image(
    title: str,
    description: str = "",
    content: str = "",
    api_key: str = "",
    model: str = "",
) -> tuple[bool, bytes]:
    """Generate a cover image based on article metadata.

    Returns (success, image_bytes_or_error_message).
    """
    api_key = api_key or os.environ.get("OPENROUTER_API_KEY", "")
    if not api_key:
        return False, "OPENROUTER_API_KEY 未配置"

    model = model or DEFAULT_MODEL

    prompt_parts = [
        "Generate a clean, visually appealing cover image for a blog post.",
        f"Title: {title}",
    ]
    if description:
        prompt_parts.append(f"Description: {description}")
    if content:
        snippet = content[:500].strip()
        prompt_parts.append(f"Content summary: {snippet}")

    prompt_parts.append(
        "Style: modern, minimalist, elegant. No text overlay. "
        "Use warm editorial tones. Aspect ratio 16:9."
    )

    prompt = "\n".join(prompt_parts)

    payload = {
        "model": model,
        "messages": [
            {
                "role": "user",
                "content": prompt,
            }
        ],
    }

    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }

    try:
        logger.info("Requesting cover image from %s", model)
        resp = requests.post(
            OPENROUTER_API_URL,
            json=payload,
            headers=headers,
            timeout=180,
        )
        resp.raise_for_status()
        data = resp.json()

        logger.info("OpenRouter response keys: %s", list(data.keys()))
        if data.get("error"):
            return False, f"API error: {data['error']}"

        choices = data.get("choices", [])
        if not choices:
            return False, "API returned no choices"

        message = choices[0].get("message", {})

        # OpenRouter returns images in message["images"] array
        images = message.get("images", [])
        if images:
            for img in images:
                if img.get("type") == "image_url":
                    url = img.get("image_url", {}).get("url", "")
                    if url.startswith("data:"):
                        b64 = url.split(",", 1)[1]
                        return True, base64.b64decode(b64)
                    elif url.startswith("http"):
                        img_resp = requests.get(url, timeout=60)
                        img_resp.raise_for_status()
                        return True, img_resp.content

        # Fallback: check message["content"]
        content_parts = message.get("content")
        if content_parts is not None:
            if isinstance(content_parts, list):
                for part in content_parts:
                    ptype = part.get("type", "")
                    if ptype == "image_url":
                        url = part.get("image_url", {}).get("url", "")
                        if url.startswith("data:"):
                            b64 = url.split(",", 1)[1]
                            return True, base64.b64decode(b64)
                        elif url.startswith("http"):
                            img_resp = requests.get(url, timeout=60)
                            img_resp.raise_for_status()
                            return True, img_resp.content

            if isinstance(content_parts, str) and "data:image" in content_parts:
                import re

                m = re.search(
                    r"data:image/[^;]+;base64,([A-Za-z0-9+/=]+)", content_parts
                )
                if m:
                    return True, base64.b64decode(m.group(1))

        return False, "No image found in API response"

    except requests.exceptions.Timeout:
        return False, "Image generation timed out (180s)"
    except requests.exceptions.HTTPError as e:
        detail = ""
        try:
            detail = e.response.json().get("error", {}).get("message", "")
        except Exception:
            detail = e.response.text[:200]
        return False, f"API error {e.response.status_code}: {detail}"
    except Exception as e:
        logger.exception("Image generation failed")
        return False, f"Image generation failed: {e}"


def save_generated_image(
    article_path: str,
    image_bytes: bytes,
    content_dir: Path,
) -> tuple[bool, str]:
    """Save generated image bytes to the article's pics/ directory.

    Returns (success, relative_url_or_error_message).
    """
    try:
        if not Path(article_path).is_absolute():
            article_file = content_dir / article_path
        else:
            article_file = Path(article_path)

        article_dir = article_file.parent
        pics_dir = article_dir / "pics"
        pics_dir.mkdir(parents=True, exist_ok=True)

        ts = int(time.time())
        filename = f"cover_{ts}.png"
        file_path = pics_dir / filename

        file_path.write_bytes(image_bytes)

        relative_url = f"pics/{filename}"
        return True, relative_url

    except Exception as e:
        logger.exception("Failed to save generated image")
        return False, f"Failed to save image: {e}"
