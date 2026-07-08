# coding: utf-8
"""
文件乐观锁（防复写）端到端测试。
"""

import tempfile
from pathlib import Path

import pytest

import app as app_module
from services.post_service import PostService


class TestFileConflictDetection:
    @pytest.fixture
    def temp_content_dir(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            content_dir = Path(temp_dir) / "content"
            content_dir.mkdir()
            yield content_dir

    @pytest.fixture
    def client(self, temp_content_dir, login):
        app_module.app.config["TESTING"] = True
        original = app_module.registry.post_service
        app_module.registry.post_service = PostService(
            temp_content_dir, use_cache=False
        )
        with app_module.app.test_client() as client:
            login(client)
            yield client
        app_module.registry.post_service = original

    @pytest.fixture
    def article(self, temp_content_dir):
        """创建一篇测试文章，返回相对路径。"""
        path = temp_content_dir / "test.md"
        path.write_text("---\ntitle: Test\n---\n\nOriginal content.\n")
        return "test.md"

    # ── 读文件返回 mtime ──

    def test_read_returns_mtime(self, client, article):
        resp = client.post("/api/file/read", json={"path": article})
        data = resp.get_json()
        assert data["success"] is True
        assert "mtime" in data
        assert isinstance(data["mtime"], (int, float))
        assert data["mtime"] > 0

    def test_read_with_frontmatter_returns_mtime(self, client, article):
        resp = client.post("/api/file/read-with-frontmatter", json={"path": article})
        data = resp.get_json()
        assert data["success"] is True
        assert "mtime" in data
        assert isinstance(data["mtime"], (int, float))

    # ── 保存返回新 mtime ──

    def test_save_returns_mtime(self, client, article):
        resp = client.post(
            "/api/file/save",
            json={"path": article, "content": "Updated.\n"},
        )
        data = resp.get_json()
        assert data["success"] is True
        assert "mtime" in data
        assert isinstance(data["mtime"], (int, float))

    # ── 乐观锁：mtime 匹配 → 成功 ──

    def test_save_with_correct_mtime_succeeds(self, client, article):
        # 先读取拿到 mtime
        read_resp = client.post("/api/file/read", json={"path": article})
        mtime = read_resp.get_json()["mtime"]

        # 用正确的 mtime 保存
        save_resp = client.post(
            "/api/file/save",
            json={
                "path": article,
                "content": "Safe update.\n",
                "expected_mtime": mtime,
            },
        )
        data = save_resp.get_json()
        assert save_resp.status_code == 200
        assert data["success"] is True

    # ── 乐观锁：mtime 不匹配 → 409 冲突 ──

    def test_save_with_wrong_mtime_returns_409(self, client, article):
        # 用一个明显过时的 mtime
        save_resp = client.post(
            "/api/file/save",
            json={
                "path": article,
                "content": "Conflicting update.\n",
                "expected_mtime": 100000.0,
            },
        )
        data = save_resp.get_json()
        assert save_resp.status_code == 409
        assert data["success"] is False
        assert data["conflict"] is True
        assert "current_content" in data
        assert "current_mtime" in data
        assert isinstance(data["current_mtime"], (int, float))

    # ── force=True 跳过 mtime 检查 ──

    def test_force_save_ignores_mtime(self, client, article):
        save_resp = client.post(
            "/api/file/save",
            json={
                "path": article,
                "content": "Forced update.\n",
                "expected_mtime": 100000.0,
                "force": True,
            },
        )
        data = save_resp.get_json()
        assert save_resp.status_code == 200
        assert data["success"] is True

    # ── 不传 expected_mtime 时无锁检查 ──

    def test_save_without_mtime_skips_lock(self, client, article):
        save_resp = client.post(
            "/api/file/save",
            json={"path": article, "content": "No lock.\n"},
        )
        data = save_resp.get_json()
        assert save_resp.status_code == 200
        assert data["success"] is True

    # ── 冲突后 force 覆盖 → 再次读取内容一致 ──

    def test_conflict_then_force_overwrites(self, client, article):
        # 触发冲突
        client.post(
            "/api/file/save",
            json={
                "path": article,
                "content": "Should conflict.\n",
                "expected_mtime": 100000.0,
            },
        )
        # force 覆盖
        client.post(
            "/api/file/save",
            json={
                "path": article,
                "content": "Forced content.\n",
                "expected_mtime": 100000.0,
                "force": True,
            },
        )
        # 验证内容
        read_resp = client.post("/api/file/read", json={"path": article})
        assert "Forced content." in read_resp.get_json()["content"]
