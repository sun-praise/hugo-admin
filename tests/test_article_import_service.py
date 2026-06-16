# coding: utf-8
"""
article_import_service 单元测试。
"""

import sys
from pathlib import Path

import frontmatter as fm_lib
import pytest

sys.path.insert(0, str(Path(__file__).parent.parent))

from services import article_import_service  # noqa: E402
from services.post_service import PostService  # noqa: E402


class FakeSocketIO:
    """记录后台任务与 emit 调用，便于断言。"""

    def __init__(self):
        self.tasks = []
        self.emits = []

    def start_background_task(self, fn, *args, **kwargs):
        self.tasks.append((fn, args, kwargs))

    def emit(self, event, payload):
        self.emits.append((event, payload))


@pytest.fixture
def post_service(tmp_path):
    content = tmp_path / "content"
    (content / "post").mkdir(parents=True)
    return PostService(content, use_cache=False)


def _abs(post_service, rel_path):
    return post_service.content_dir / rel_path


def _ai_cfg(**over):
    cfg = {"api_key": "ai-key", "base_url": "http://x", "model": "m"}
    cfg.update(over)
    return cfg


def _image_cfg(**over):
    cfg = {"api_key": "or-key", "model": "img"}
    cfg.update(over)
    return cfg


# ---------------- 标题派生 ----------------


def test_title_from_frontmatter(post_service):
    md = b"---\ntitle: From Frontmatter\ndraft: true\n---\n\nbody text\n"
    res = article_import_service.import_markdown(
        "note.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert res["title"] == "From Frontmatter"
    assert res["path"].startswith("post/") and res["path"].endswith("/index.md")


def test_title_from_first_h1(post_service):
    md = b"# Heading One\n\nsome body\n"
    res = article_import_service.import_markdown(
        "note.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert res["title"] == "Heading One"


def test_title_from_filename(post_service):
    md = b"just body, no frontmatter, no heading\n"
    res = article_import_service.import_markdown(
        "My Note.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert res["title"] == "My Note"


# ---------------- slug / 目录 ----------------


def test_slug_collision_appends_suffix(post_service):
    md = b"# Same Title\n\nbody\n"
    first = article_import_service.import_markdown(
        "a.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    second = article_import_service.import_markdown(
        "b.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert first["path"] != second["path"]
    assert _abs(post_service, first["path"]).exists()
    assert _abs(post_service, second["path"]).exists()


# ---------------- frontmatter 合并 / 保留 ----------------


def test_frontmatter_merge_preserves_existing(post_service, monkeypatch):
    monkeypatch.setattr(
        article_import_service.frontmatter_gen_service,
        "generate_frontmatter",
        lambda **kw: (
            True,
            {"description": "AI desc", "tags": ["ai-tag"], "categories": ["ai-cat"]},
        ),
    )
    md = b"---\ntitle: Kept\ndescription: keep-me\ntags: [existing]\n---\n\nbody\n"
    res = article_import_service.import_markdown(
        "note.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(),
        image_cfg=_image_cfg(api_key=""),
    )
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    # 已有字段被保留，AI 不覆盖
    assert post["description"] == "keep-me"
    assert post["tags"] == ["existing"]
    # 缺失字段由 AI 填充
    assert post["categories"] == ["ai-cat"]


def test_frontmatter_graceful_without_ai_key(post_service):
    md = b"# Hello\n\nbody\n"
    res = article_import_service.import_markdown(
        "note.md",
        md,
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert res["path"]
    assert any("AI API Key" in w for w in res["warnings"])
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert post["title"] == "Hello"
    assert post["draft"] is True
    assert post["categories"] == []
    assert post["tags"] == []


# ---------------- 封面 ----------------


def test_cover_sync_attached(post_service, monkeypatch):
    monkeypatch.setattr(
        article_import_service.image_gen_service,
        "generate_cover_image",
        lambda **kw: (True, b"png-bytes"),
    )
    monkeypatch.setattr(
        article_import_service.image_gen_service,
        "save_generated_image",
        lambda article_path, image_bytes, content_dir: (True, "pics/cover_1.png"),
    )
    res = article_import_service.import_markdown(
        "note.md",
        b"# T\n\nbody\n",
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(),
        socketio=None,
        generate_frontmatter=False,
    )
    assert res["cover_pending"] is False
    assert res["warnings"] == []
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert post["cover"] == "pics/cover_1.png"


def test_cover_runs_in_background_when_socketio(post_service, monkeypatch):
    monkeypatch.setattr(
        article_import_service.image_gen_service,
        "generate_cover_image",
        lambda **kw: (True, b"png-bytes"),
    )
    monkeypatch.setattr(
        article_import_service.image_gen_service,
        "save_generated_image",
        lambda article_path, image_bytes, content_dir: (True, "pics/cover_bg.png"),
    )
    socket = FakeSocketIO()
    res = article_import_service.import_markdown(
        "note.md",
        b"# T\n\nbody\n",
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(),
        socketio=socket,
        event_scope="scope-1",
    )
    # 后台任务被调度，封面尚未写入
    assert res["cover_pending"] is True
    assert len(socket.tasks) == 1
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert "cover" not in post.metadata

    # 执行后台任务
    fn, args, _kwargs = socket.tasks[0]
    fn(*args)
    events = [e for e in socket.emits]
    assert ("article_import.progress", {"scope": "scope-1", "stage": "cover"}) in events
    assert any(
        e[0] == "article_import.cover_done" and e[1]["url"] == "pics/cover_bg.png"
        for e in events
    )
    # 封面字段已写入
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert post["cover"] == "pics/cover_bg.png"


def test_cover_disabled(post_service, monkeypatch):
    called = {"cover": 0}

    def _boom(**kw):
        called["cover"] += 1
        return True, b"x"

    monkeypatch.setattr(
        article_import_service.image_gen_service, "generate_cover_image", _boom
    )
    res = article_import_service.import_markdown(
        "note.md",
        b"# T\n\nbody\n",
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(),
        generate_frontmatter=False,
        generate_cover=False,
    )
    assert called["cover"] == 0
    assert res["warnings"] == []


def test_cover_without_key_warns(post_service):
    res = article_import_service.import_markdown(
        "note.md",
        b"# T\n\nbody\n",
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    assert any("OPENROUTER_API_KEY" in w for w in res["warnings"])
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert "cover" not in post.metadata


# ---------------- 正文 / 草稿 ----------------


def test_body_preserved_and_draft(post_service):
    body = "# Hello World\n\nThis is the **body**.\n\n- a\n- b\n"
    res = article_import_service.import_markdown(
        "note.md",
        body.encode("utf-8"),
        post_service=post_service,
        ai_cfg=_ai_cfg(api_key=""),
        image_cfg=_image_cfg(api_key=""),
    )
    post = fm_lib.load(str(_abs(post_service, res["path"])))
    assert post.content.strip() == body.strip()
    assert post["draft"] is True
