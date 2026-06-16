# coding: utf-8
"""
/api/article/import 路由测试。
"""

import io
import sys
from pathlib import Path

import frontmatter as fm_lib
import pytest
from flask import Blueprint, Flask

sys.path.insert(0, str(Path(__file__).parent.parent))

from routes.file_routes import register_file_routes  # noqa: E402
from services.post_service import PostService  # noqa: E402


class _Registry:
    """最小 registry：路由在请求时按属性访问 post_service / socketio。"""


_REGISTRY = _Registry()


@pytest.fixture
def env(tmp_path, monkeypatch):
    content = tmp_path / "content"
    (content / "post").mkdir(parents=True)
    _REGISTRY.post_service = PostService(content, use_cache=False)
    _REGISTRY.socketio = None

    # 每个测试注入一个全新 Blueprint，避免污染共享的模块级 bp（与导入真实
    # app.py 的集成测试共存时也不会重复注册 endpoint）。
    fresh_bp = Blueprint("files_test", __name__)
    register_file_routes(_REGISTRY, blueprint=fresh_bp)

    app = Flask(__name__)
    app.config["TESTING"] = True
    app.config["AI_API_KEY"] = "ai-key"
    app.config["AI_BASE_URL"] = "http://x"
    app.config["AI_MODEL"] = "m"
    app.config["OPENROUTER_API_KEY"] = "or-key"
    app.config["IMAGE_GEN_MODEL"] = "img"
    app.register_blueprint(fresh_bp)

    monkeypatch.setattr(
        "services.frontmatter_gen_service.generate_frontmatter",
        lambda **kw: (
            True,
            {"description": "AI desc", "tags": ["ai"], "categories": ["ai-cat"]},
        ),
    )
    monkeypatch.setattr(
        "services.image_gen_service.generate_cover_image",
        lambda **kw: (True, b"png-bytes"),
    )
    monkeypatch.setattr(
        "services.image_gen_service.save_generated_image",
        lambda article_path, image_bytes, content_dir: (True, "pics/cover_1.png"),
    )
    return app.test_client(), _REGISTRY.post_service, content, app


def _post_file(client, body, filename="note.md", **form):
    data = {"file": (io.BytesIO(body), filename)}
    data.update(form)
    return client.post(
        "/api/article/import", data=data, content_type="multipart/form-data"
    )


def test_import_happy_path(env):
    client, post_service, content, _app = env
    resp = _post_file(client, b"# Hello\n\nbody\n", title="MyTitle")
    assert resp.status_code == 200
    payload = resp.get_json()
    assert payload["success"] is True
    assert payload["title"] == "MyTitle"
    assert payload["path"].startswith("post/")
    assert payload["cover_pending"] is False

    post = fm_lib.load(str(content / payload["path"]))
    assert post["title"] == "MyTitle"
    assert post["draft"] is True
    assert post["cover"] == "pics/cover_1.png"


def test_import_title_from_h1(env):
    client, _ps, content, _app = env
    resp = _post_file(client, b"# Heading Body\n\ntext\n")
    assert resp.status_code == 200
    assert resp.get_json()["title"] == "Heading Body"


def test_import_unsupported_extension(env):
    client, _ps, _content, _app = env
    resp = _post_file(client, b"text", filename="note.txt")
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False


def test_import_missing_file(env):
    client, _ps, _content, _app = env
    resp = client.post(
        "/api/article/import", data={}, content_type="multipart/form-data"
    )
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False


def test_import_partial_success_without_image_key(env):
    client, _ps, content, app = env
    # 封面未配置 → 报告 warning，但文章仍导入成功
    app.config["OPENROUTER_API_KEY"] = ""
    resp = _post_file(client, b"# T\n\nbody\n")
    assert resp.status_code == 200
    payload = resp.get_json()
    assert payload["success"] is True
    assert payload["warnings"]  # 非空
    post = fm_lib.load(str(content / payload["path"]))
    assert "cover" not in post.metadata
