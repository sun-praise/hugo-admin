# coding: utf-8
"""文件读写 API 的 HTTP 契约测试。

monkeypatch ``registry.post_service`` 指向临时内容目录（固定 seed 文件），
``registry.ref_service`` 换成哑实现避免扫描真实博客仓库。
``RECORD_CONTRACT=1`` 时录制契约样本；mtime/current_mtime/create 的 path
等环境相关字段由 Go 侧回放时归一化。
"""

import sys
import tempfile
from pathlib import Path
from types import SimpleNamespace

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.post_service import PostService  # noqa: E402

SEED_HELLO = "---\ntitle: Hello\ndate: 2026-01-01\ndraft: false\n---\n\nHello body。\n"
SEED_DUAL = "---\ntitle: Dual\n---\n---\ninner\n---\nreal body\n"
SEED_LOCK = "锁定基准\n"


@pytest.fixture
def file_client(auth_store, login):
    tmp = tempfile.mkdtemp()
    content_dir = Path(tmp)
    post_dir = content_dir / "post"
    post_dir.mkdir()
    (post_dir / "hello.md").write_text(SEED_HELLO, encoding="utf-8")
    (post_dir / "dual.md").write_text(SEED_DUAL, encoding="utf-8")
    (post_dir / "lock.md").write_text(SEED_LOCK, encoding="utf-8")

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


class TestFileHTTPAPI:
    def test_read_ok(self, file_client):
        resp = file_client.post("/api/file/read", json={"path": "post/hello.md"})
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["content"] == SEED_HELLO
        assert data["path"] == "post/hello.md"
        assert data["mtime"] > 0

    def test_read_missing_path(self, file_client):
        resp = file_client.post("/api/file/read", json={"path": ""})
        assert resp.status_code == 400
        assert resp.get_json()["message"] == "缺少文件路径"

    def test_read_with_frontmatter(self, file_client):
        resp = file_client.post(
            "/api/file/read-with-frontmatter", json={"path": "post/dual.md"}
        )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["frontmatter"] == {"title": "Dual"}
        assert data["content"] == "real body\n"

    def test_save_plain(self, file_client):
        resp = file_client.post(
            "/api/file/save", json={"path": "post/saved.md", "content": "# 保存\n"}
        )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["message"] == "文件保存成功"
        assert data["mtime"] > 0

    def test_save_with_frontmatter_merge(self, file_client):
        resp = file_client.post(
            "/api/file/save",
            json={
                "path": "post/hello.md",
                "content": "---\ntitle: old\n---\n\n新正文\n",
                "frontmatter": {"title": "Hello 改", "tags": ["go"], "draft": True},
            },
        )
        assert resp.status_code == 200

        # 保存后重读：frontmatter 已合并、正文中的旧块被剥离
        data = file_client.post(
            "/api/file/read-with-frontmatter", json={"path": "post/hello.md"}
        ).get_json()
        assert data["frontmatter"]["title"] == "Hello 改"
        assert data["frontmatter"]["draft"] is True
        assert data["frontmatter"]["tags"] == ["go"]
        # dumps 会 strip 正文首尾空白，故重读无尾换行
        assert data["content"] == "新正文"

    def test_save_optimistic_lock_conflict(self, file_client):
        mtime = file_client.post(
            "/api/file/read", json={"path": "post/lock.md"}
        ).get_json()["mtime"]
        resp = file_client.post(
            "/api/file/save",
            json={
                "path": "post/lock.md",
                "content": "覆盖内容\n",
                "expected_mtime": mtime - 100,
            },
        )
        assert resp.status_code == 409
        data = resp.get_json()
        assert data["success"] is False
        assert data["conflict"] is True
        assert data["current_content"] == SEED_LOCK
        assert data["message"] == "文件已被其他人修改"

    def test_save_force_overrides_lock(self, file_client):
        mtime = file_client.post(
            "/api/file/read", json={"path": "post/lock.md"}
        ).get_json()["mtime"]
        resp = file_client.post(
            "/api/file/save",
            json={
                "path": "post/lock.md",
                "content": "强制写入\n",
                "expected_mtime": mtime - 100,
                "force": True,
            },
        )
        assert resp.status_code == 200
        content = file_client.post(
            "/api/file/read", json={"path": "post/lock.md"}
        ).get_json()["content"]
        assert content == "强制写入\n"

    def test_create_post(self, file_client):
        resp = file_client.post("/api/post/create", json={"title": "契约文章"})
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["message"] == "文章创建成功"
        assert "/index.md" in data["path"]

    def test_create_post_missing_title(self, file_client):
        resp = file_client.post("/api/post/create", json={"title": ""})
        assert resp.status_code == 400
        assert resp.get_json()["message"] == "缺少文章标题"
