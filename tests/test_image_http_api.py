# coding: utf-8
"""图片上传/列表 API 的 HTTP 契约测试。

上传走 multipart（录制器提取 files 为 base64，Go 回放重构 multipart）。
fixture 字节固定，上传文件名固定，url 响应确定可严格比对。
"""

import io
import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.post_service import PostService  # noqa: E402

# 固定 PNG 字节（8 字节 magic + 内容）
PNG_BYTES = b"\x89PNG\r\n\x1a\n" + b"contract-image-body" * 3

ARTICLE = "---\ntitle: 图片文章\ndate: 2026-03-01\ndraft: false\n---\n\n正文\n"


@pytest.fixture
def image_client(auth_store, login):
    content_dir = Path(tempfile.mkdtemp())
    post_dir = content_dir / "post"
    post_dir.mkdir()
    (post_dir / "img.md").write_text(ARTICLE, encoding="utf-8")

    original_post = app_module.registry.post_service
    original_ref = app_module.registry.ref_service
    original_pm = getattr(app_module.registry, "plugin_manager", None)
    app_module.registry.post_service = PostService(str(content_dir), use_cache=False)
    app_module.registry.ref_service = SimpleNamespace(update_file=lambda p: None)
    # 无插件环境：跳过插件上传路径，走本地保存
    app_module.registry.plugin_manager = None
    client = app_module.app.test_client()
    login(client)
    try:
        yield client, content_dir
    finally:
        app_module.registry.post_service = original_post
        app_module.registry.ref_service = original_ref
        app_module.registry.plugin_manager = original_pm


class TestImageHTTPAPI:
    def test_upload(self, image_client):
        client, _ = image_client
        resp = client.post(
            "/api/image/upload",
            data={
                "article_path": "post/img.md",
                "file": (io.BytesIO(PNG_BYTES), "契约图片.png", "image/png"),
            },
            content_type="multipart/form-data",
        )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data == {
            "success": True,
            "url": "pics/契约图片.png",
            "message": "图片上传成功",
        }

    def test_upload_missing_path(self, image_client):
        client, _ = image_client
        resp = client.post(
            "/api/image/upload",
            data={"file": (io.BytesIO(PNG_BYTES), "x.png", "image/png")},
            content_type="multipart/form-data",
        )
        assert resp.status_code == 400
        assert resp.get_json()["message"] == "缺少文章路径"

    def test_upload_no_file(self, image_client):
        client, _ = image_client
        resp = client.post("/api/image/upload", data={"article_path": "post/img.md"})
        assert resp.status_code == 400
        assert resp.get_json()["message"] == "没有文件"

    def test_upload_bad_type(self, image_client):
        client, _ = image_client
        resp = client.post(
            "/api/image/upload",
            data={
                "article_path": "post/img.md",
                "file": (io.BytesIO(b"BM"), "x.bmp", "image/bmp"),
            },
            content_type="multipart/form-data",
        )
        assert resp.status_code == 400
        assert "不支持的文件类型: bmp" in resp.get_json()["message"]

    def test_list(self, image_client):
        client, _ = image_client
        # 经已实现的保存端点改写文章（含图片引用），保证回放可重放
        save = client.post(
            "/api/file/save",
            json={
                "path": "post/img.md",
                "content": ARTICLE
                + "\n![契约](pics/契约图片.png)\n![cdn](https://cdn.example.com/a.webp)\n",
            },
        )
        assert save.status_code == 200

        resp = client.post("/api/image/list", json={"article_path": "post/img.md"})
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["images"] == [
            {"name": "契约图片.png", "url": "pics/契约图片.png"},
            {"name": "a.webp", "url": "https://cdn.example.com/a.webp"},
        ]

    def test_list_missing_path(self, image_client):
        client, _ = image_client
        resp = client.post("/api/image/list", json={})
        assert resp.status_code == 400
        assert resp.get_json()["message"] == "缺少文章路径"
