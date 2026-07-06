# coding: utf-8
"""
Markdown 导入服务。

将外部 .md 文件导入为一篇 Hugo 草稿文章，复用已有的 AI 服务自动补全
frontmatter（description / tags / categories）与封面图，必要时在后台生成
封面并通过 Socket.IO 上报进度。导入过程中任何 AI 步骤失败都不会丢失正文：
失败信息汇总到返回结果的 ``warnings`` 列表中。
"""

import logging
import re
import uuid
from datetime import datetime, timedelta, timezone
from pathlib import Path

import frontmatter

# 通过模块引用访问 AI 服务，避免与本模块的同名布尔参数冲突
from services import frontmatter_gen_service, image_gen_service

logger = logging.getLogger(__name__)

_CST = timezone(timedelta(hours=8))
_PATH_SEPARATORS = re.compile(r"[\\/]")


def _is_empty(value) -> bool:
    return value is None or value == "" or value == [] or value == {}


def _now_cst_iso() -> str:
    return datetime.now(_CST).strftime("%Y-%m-%dT%H:%M:%S+08:00")


def _now_date_prefix() -> str:
    return datetime.now(_CST).date().isoformat()


def _split_frontmatter(text: str) -> tuple[dict, str]:
    """拆分 frontmatter 与正文。无 frontmatter 时返回空 dict 与原文。"""
    post = frontmatter.loads(text)
    metadata = dict(post.metadata) if post.metadata else {}
    return metadata, post.content


def _derive_title(explicit, existing_fm: dict, body: str, filename: str) -> str:
    """标题派生顺序：显式传入 → 已有 frontmatter.title → 首个 H1 → 文件名。"""
    if isinstance(explicit, str) and explicit.strip():
        return explicit.strip()

    title = existing_fm.get("title")
    if isinstance(title, str) and title.strip():
        return title.strip()

    for line in body.splitlines():
        stripped = line.strip()
        if stripped.startswith("# "):
            heading = stripped[2:].strip()
            if heading:
                return heading

    stem = Path(filename or "").stem.strip()
    return stem or "untitled"


def _slugify(title: str) -> str:
    """生成目录名 slug：空格转连字符，去除路径分隔符，杜绝 `.`/`..`。"""
    slug = title.strip().replace(" ", "-")
    slug = _PATH_SEPARATORS.sub("-", slug)
    slug = slug.strip(".")
    return slug or "untitled"


def _create_article_dir(post_service, title: str) -> str:
    """在 content/post 下创建唯一文章目录，返回 content 相对路径 index.md。

    目录命名沿用 create_post 的 ``<YYYY-MM-DD>-<slug>`` 约定；若已存在则追加
    短 uuid 后缀，绝不覆盖已有文章。
    """
    date_prefix = _now_date_prefix()
    slug = _slugify(title)
    name = f"{date_prefix}-{slug}"
    target = post_service.post_dir / name
    while target.exists():
        name = f"{date_prefix}-{slug}-{uuid.uuid4().hex[:6]}"
        target = post_service.post_dir / name
    target.mkdir(parents=True, exist_ok=True)
    return f"post/{name}/index.md"


def _set_cover_field(post_service, article_path: str, cover_url: str) -> bool:
    """把 cover 字段写回文章 frontmatter，复用 PostService 的读写方法。"""
    ok, body, fm, _mtime = post_service.read_file_with_frontmatter(article_path)
    if not ok:
        return False
    fm["cover"] = cover_url
    ok2, _msg, _mtime = post_service.save_file(article_path, body, frontmatter_data=fm)
    return ok2


def generate_and_attach_cover(
    post_service,
    article_path: str,
    title: str,
    description: str,
    content: str,
    image_cfg: dict,
) -> tuple[bool, str]:
    """生成封面图并写入文章，返回 (ok, url_or_error_message)。"""
    ok, result = image_gen_service.generate_cover_image(
        title=title,
        description=description,
        content=content,
        api_key=image_cfg.get("api_key", ""),
        model=image_cfg.get("model", ""),
    )
    if not ok:
        return False, result

    save_ok, save_result = image_gen_service.save_generated_image(
        article_path, result, post_service.content_dir
    )
    if not save_ok:
        return False, save_result

    if not _set_cover_field(post_service, article_path, save_result):
        return False, "写入封面字段失败"

    if post_service.cache_service:
        post_service.cache_service.invalidate_post(
            str(post_service.content_dir / article_path)
        )
    return True, save_result


def _run_cover_with_emits(
    post_service,
    article_path: str,
    title: str,
    description: str,
    content: str,
    image_cfg: dict,
    socketio,
    event_scope: str,
) -> None:
    """后台任务：生成封面并通过 Socket.IO 上报进度。"""

    def emit_event(event: str, payload: dict) -> None:
        try:
            socketio.emit(event, {"scope": event_scope, **payload})
        except Exception:
            logger.exception("Socket emit failed for %s", event)

    emit_event("article_import.progress", {"stage": "cover"})
    ok, result = generate_and_attach_cover(
        post_service, article_path, title, description, content, image_cfg
    )
    if ok:
        emit_event("article_import.cover_done", {"url": result})
    else:
        emit_event("article_import.cover_failed", {"message": result})


def import_markdown(
    filename,
    raw_bytes,
    *,
    title=None,
    generate_frontmatter=True,
    generate_cover=True,
    post_service,
    ai_cfg,
    image_cfg,
    socketio=None,
    event_scope=None,
) -> dict:
    """导入一个 Markdown 文件为草稿文章。

    Returns:
        ``{"path", "title", "warnings", "cover_pending"}``。
    """
    warnings: list[str] = []

    # 1. 解码（编码异常也不应导致正文丢失）
    if isinstance(raw_bytes, (bytes, bytearray)):
        text = bytes(raw_bytes).decode("utf-8", errors="replace")
    else:
        text = str(raw_bytes)

    # 2. 拆分已有 frontmatter / 正文
    existing_fm, body = _split_frontmatter(text)

    # 3. 派生标题 + 创建目录
    resolved_title = _derive_title(title, existing_fm, body, filename)
    article_path = _create_article_dir(post_service, resolved_title)

    # 4. 组装 frontmatter：标题 / 草稿 / 日期，并保留已有的富化字段
    fm: dict = {
        "title": resolved_title,
        "draft": True,
        "date": (
            existing_fm["date"]
            if isinstance(existing_fm.get("date"), str) and existing_fm["date"].strip()
            else _now_cst_iso()
        ),
    }
    for key in ("description", "categories", "tags", "cover"):
        value = existing_fm.get(key)
        if not _is_empty(value):
            fm[key] = value

    # 5. AI frontmatter 富化（仅填充缺失字段，不覆盖已有值）
    api_key = ai_cfg.get("api_key", "")
    if generate_frontmatter:
        if not api_key:
            warnings.append("未配置 AI API Key，跳过 frontmatter 生成")
        else:
            ok, result = frontmatter_gen_service.generate_frontmatter(
                content=body,
                api_key=api_key,
                base_url=ai_cfg.get("base_url", ""),
                model=ai_cfg.get("model", ""),
            )
            if not ok:
                warnings.append(f"frontmatter 生成失败：{result}")
            elif isinstance(result, dict):
                for key in ("description", "tags", "categories"):
                    if _is_empty(fm.get(key)) and not _is_empty(result.get(key)):
                        fm[key] = result[key]
    fm.setdefault("categories", [])
    fm.setdefault("tags", [])

    # 6. 写入文章（正文 + 合并后的 frontmatter，草稿，暂无封面）
    write_ok, write_msg, _mtime = post_service.save_file(
        article_path, body, frontmatter_data=fm
    )
    if not write_ok:
        return {
            "path": None,
            "title": resolved_title,
            "warnings": [f"写入文件失败：{write_msg}"],
            "cover_pending": False,
            "event_scope": event_scope,
        }

    # 7. 封面图：有 Socket.IO 时后台生成，否则同步生成
    cover_pending = False
    if generate_cover:
        if not image_cfg.get("api_key"):
            warnings.append("未配置 OPENROUTER_API_KEY，跳过封面生成")
        else:
            description = fm.get("description") or ""
            if socketio is not None and event_scope:
                cover_pending = True
                socketio.start_background_task(
                    _run_cover_with_emits,
                    post_service,
                    article_path,
                    resolved_title,
                    description,
                    body,
                    image_cfg,
                    socketio,
                    event_scope,
                )
            else:
                ok, result = generate_and_attach_cover(
                    post_service,
                    article_path,
                    resolved_title,
                    description,
                    body,
                    image_cfg,
                )
                if not ok:
                    warnings.append(f"封面生成失败：{result}")

    return {
        "path": article_path,
        "title": resolved_title,
        "warnings": warnings,
        "cover_pending": cover_pending,
        "event_scope": event_scope,
    }
