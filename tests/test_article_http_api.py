# coding: utf-8
"""Article 发布 API 的 HTTP 契约测试。

fixture 同 test_file_http_api 模式（临时内容目录 + 固定 seed），
另加两篇草稿供发布链路使用。published_at/operation_id 等时间性字段
与含绝对路径的错误消息由 Go 侧回放时归一化。
"""

import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.post_service import PostService  # noqa: E402

SEEDS = {
    "post/hello.md": (
        "---\ntitle: Hello\ndate: 2026-01-01\ndraft: false\n---\n\nHello body。\n"
    ),
    "post/dual.md": "---\ntitle: Dual\n---\n---\ninner\n---\nreal body\n",
    "post/lock.md": "锁定基准\n",
    "post/draft.md": (
        "---\ntitle: 草稿文章\ndraft: true\ntags:\n  - go\n---\n\n草稿正文\n"
    ),
    "post/draft2.md": "---\ntitle: 第二草稿\ndraft: true\n---\n\n第二草稿\n",
}


@pytest.fixture
def article_client(auth_store, login):
    content_dir = Path(tempfile.mkdtemp())
    post_dir = content_dir / "post"
    post_dir.mkdir()
    for rel, content in SEEDS.items():
        (content_dir / rel).write_text(content, encoding="utf-8")

    original_post = app_module.registry.post_service
    original_ref = app_module.registry.ref_service
    app_module.registry.post_service = PostService(str(content_dir), use_cache=False)
    app_module.registry.ref_service = SimpleNamespace(update_file=lambda p: None)
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.registry.post_service = original_post
        app_module.registry.ref_service = original_ref


class TestArticleHTTPAPI:
    def test_status(self, article_client):
        data = article_client.get(
            "/api/article/status?file_path=post/hello.md"
        ).get_json()
        assert data["success"] is True
        assert data["status"]["is_draft"] is False
        assert data["status"]["is_publishable"] is False

        # dual.md 无 draft 字段 → 默认草稿
        data = article_client.get(
            "/api/article/status?file_path=post/dual.md"
        ).get_json()
        assert data["status"]["is_draft"] is True

    def test_status_missing_param(self, article_client):
        resp = article_client.get("/api/article/status")
        assert resp.status_code == 400
        assert resp.get_json()["error_code"] == "MISSING_PARAMETER"

    def test_bulk_status(self, article_client):
        data = article_client.post(
            "/api/article/status/bulk",
            json={"file_paths": ["post/hello.md", "post/draft.md"]},
        ).get_json()
        assert data["count"] == 2
        assert data["results"][0]["status"]["is_draft"] is False
        assert data["results"][1]["status"]["is_draft"] is True

    def test_publish_draft(self, article_client):
        resp = article_client.post(
            "/api/article/publish", json={"file_path": "post/draft.md"}
        )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["message"] == "文章发布成功"
        assert data["article_path"] == "post/draft.md"
        assert data["draft_status_changed"] is True

        # 发布后状态翻转
        status = article_client.get(
            "/api/article/status?file_path=post/draft.md"
        ).get_json()["status"]
        assert status["is_draft"] is False

    def test_publish_already_published(self, article_client):
        resp = article_client.post(
            "/api/article/publish", json={"file_path": "post/hello.md"}
        )
        assert resp.status_code == 409
        data = resp.get_json()
        assert data["error"] == "文章已经发布"
        assert data["error_code"] == "PUBLISH_FAILED"

    def test_publish_missing(self, article_client):
        resp = article_client.post(
            "/api/article/publish", json={"file_path": "post/none.md"}
        )
        assert resp.status_code == 404
        assert "文件不存在" in resp.get_json()["error"]

    def test_bulk_publish_mixed(self, article_client):
        resp = article_client.post(
            "/api/article/publish/bulk",
            json={"file_paths": ["post/draft2.md", "post/none.md"]},
        )
        assert resp.status_code == 207
        data = resp.get_json()
        assert data["published_count"] == 1
        assert data["failed_count"] == 1
        assert data["total_count"] == 2
        assert data["results"][0]["success"] is True
        assert "文件不存在" in data["results"][1]["message"]
