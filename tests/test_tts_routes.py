# coding: utf-8 -*-
"""
文章级 TTS 路由 + 后台任务单元测试。
"""

import tempfile
from pathlib import Path
from unittest.mock import MagicMock

import pytest
from flask import Flask

from proto import plugin_pb2
from routes.tts_routes import (
    FM_AUDIO,
    FM_AUDIO_DURATION,
    FM_AUDIO_ID,
    _run_tts_with_emits,
    register_tts_routes,
)
from services.post_service import PostService

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------


def _make_tts_responses(success=True, url="https://r2.example.com/a.mp3"):
    """构造一个 TTSResponse 迭代器：一条 progress + 一条 result。"""
    progress = plugin_pb2.TTSResponse(
        progress=plugin_pb2.TTSProgress(
            stage="synthesizing", percent=50.0, message="ok"
        )
    )
    result = plugin_pb2.TTSResponse(
        result=plugin_pb2.TTSResult(
            success=success,
            url=url,
            duration_seconds=12.5,
            audio_id="key-abc",
            format="mp3",
            message="" if success else "boom",
        )
    )
    return [progress, result]


@pytest.fixture
def temp_setup():
    """临时 content 目录 + PostService + 一篇文章。"""
    with tempfile.TemporaryDirectory() as tmp:
        content_dir = Path(tmp)
        svc = PostService(content_dir, use_cache=False)
        article = content_dir / "post" / "demo" / "index.md"
        article.parent.mkdir(parents=True)
        article.write_text(
            "---\ntitle: Demo\ndraft: true\n---\n\n这是一段正文。\n",
            encoding="utf-8",
        )
        yield svc, article, "post/demo/index.md"


@pytest.fixture
def app_and_registry(temp_setup):
    """Flask test app + 一个 mock registry（带 post_service）。"""
    svc, _article, _rel = temp_setup
    registry = MagicMock()
    registry.post_service = svc
    registry.socketio = None  # 走同步路径（_NoopSocket）
    registry.plugin_manager = MagicMock()
    app = Flask(__name__)
    app.config["TESTING"] = True
    app.register_blueprint(register_tts_routes(registry))
    yield app, registry, svc


# ---------------------------------------------------------------------------
# _run_tts_with_emits —— 后台任务核心逻辑
# ---------------------------------------------------------------------------


class TestRunTtsWithEmits:
    def test_success_writes_frontmatter(self, temp_setup):
        svc, _article, rel = temp_setup
        # 读真实 mtime，模拟路由层读取文章时的快照
        _ok, _body, _fm, mtime = svc.read_file_with_frontmatter(rel)
        plugin_manager = MagicMock()
        stub = MagicMock()
        stub.Generate.return_value = iter(_make_tts_responses())
        plugin_manager.find_plugin_with_capability.return_value = {"name": "tts-gen"}
        plugin_manager.get_tts_generator_stub.return_value = stub

        events = []

        class Sock:
            def emit(self, event, payload):
                events.append((event, payload))

        _run_tts_with_emits(
            svc, plugin_manager, rel, "朗读文本", {}, mtime, Sock(), "scope-1"
        )

        # 写回 frontmatter
        ok, body, fm, _mtime = svc.read_file_with_frontmatter(rel)
        assert ok
        assert fm[FM_AUDIO] == "https://r2.example.com/a.mp3"
        assert fm[FM_AUDIO_DURATION] == 12.5
        assert fm[FM_AUDIO_ID] == "key-abc"
        # 进度 + done 事件
        assert events[0][0] == "tts.progress"
        done = [e for e in events if e[0] == "tts.done"]
        assert done and done[0][1]["url"] == "https://r2.example.com/a.mp3"
        # done 事件必须回传新 mtime（mtime 一致性，见 design Decision 8）
        assert done[0][1]["mtime"], "tts.done 必须携带 mtime"

    def test_plugin_missing_emits_failed(self, temp_setup):
        svc, _article, rel = temp_setup
        plugin_manager = MagicMock()
        plugin_manager.find_plugin_with_capability.return_value = None
        plugin_manager.get_tts_generator_stub.return_value = None

        events = []

        class Sock:
            def emit(self, event, payload):
                events.append((event, payload))

        _run_tts_with_emits(svc, plugin_manager, rel, "x", {}, 0.0, Sock(), "scope-2")
        failed = [e for e in events if e[0] == "tts.failed"]
        assert failed
        # frontmatter 未被改动
        _ok, _body, fm, _ = svc.read_file_with_frontmatter(rel)
        assert FM_AUDIO not in fm

    def test_plugin_returns_failure_emits_failed(self, temp_setup):
        svc, _article, rel = temp_setup
        plugin_manager = MagicMock()
        stub = MagicMock()
        stub.Generate.return_value = iter(_make_tts_responses(success=False))
        plugin_manager.find_plugin_with_capability.return_value = {"name": "tts-gen"}
        plugin_manager.get_tts_generator_stub.return_value = stub

        events = []

        class Sock:
            def emit(self, event, payload):
                events.append((event, payload))

        _run_tts_with_emits(svc, plugin_manager, rel, "x", {}, 0.0, Sock(), "scope-3")
        failed = [e for e in events if e[0] == "tts.failed"]
        assert failed and "boom" in failed[0][1]["message"]

    def test_conflict_emits_conflict(self, temp_setup):
        svc, article, rel = temp_setup
        plugin_manager = MagicMock()
        stub = MagicMock()
        stub.Generate.return_value = iter(_make_tts_responses())
        plugin_manager.find_plugin_with_capability.return_value = {"name": "tts-gen"}
        plugin_manager.get_tts_generator_stub.return_value = stub

        events = []

        class Sock:
            def emit(self, event, payload):
                events.append((event, payload))

        # 用一个陈旧的 expected_mtime（明显偏离真实值）触发冲突
        _run_tts_with_emits(svc, plugin_manager, rel, "x", {}, 1.0, Sock(), "scope-4")
        conflict = [e for e in events if e[0] == "tts.conflict"]
        assert conflict
        # 文件未被覆盖（仍无 audio 字段）
        _ok, _body, fm, _ = svc.read_file_with_frontmatter(rel)
        assert FM_AUDIO not in fm


# ---------------------------------------------------------------------------
# HTTP 路由
# ---------------------------------------------------------------------------


class TestGenerateRoute:
    def test_no_plugin_returns_400(self, app_and_registry):
        app, registry, _svc = app_and_registry
        registry.plugin_manager.find_plugin_with_capability.return_value = None
        registry.plugin_manager.get_tts_generator_stub.return_value = None
        with app.test_client() as client:
            resp = client.post(
                "/api/article/tts",
                json={"article_path": "post/demo/index.md"},
            )
        assert resp.status_code == 400
        assert "tts_generation" in resp.get_json()["message"]

    def test_missing_article_path(self, app_and_registry):
        app, _registry, _svc = app_and_registry
        with app.test_client() as client:
            resp = client.post("/api/article/tts", json={})
        assert resp.status_code == 400

    def test_sync_path_writes_frontmatter(self, app_and_registry):
        """socketio=None 时走同步路径并写回 frontmatter。"""
        app, registry, svc = app_and_registry
        stub = MagicMock()
        stub.Generate.return_value = iter(_make_tts_responses())
        registry.plugin_manager.find_plugin_with_capability.return_value = {
            "name": "tts-gen"
        }
        registry.plugin_manager.get_tts_generator_stub.return_value = stub

        with app.test_client() as client:
            resp = client.post(
                "/api/article/tts",
                json={"article_path": "post/demo/index.md"},
            )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["pending"] is False
        # 验证写回
        _ok, _body, fm, _ = svc.read_file_with_frontmatter("post/demo/index.md")
        assert fm[FM_AUDIO] == "https://r2.example.com/a.mp3"


class TestStatusRoute:
    def test_available_true(self, app_and_registry):
        app, registry, _svc = app_and_registry
        registry.plugin_manager.find_plugin_with_capability.return_value = {
            "name": "tts-gen"
        }
        registry.plugin_manager.get_config_schema.return_value = {}
        with app.test_client() as client:
            resp = client.get("/api/article/tts/status")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["available"] is True
        assert data["plugin"] == "tts-gen"

    def test_available_false_when_no_plugin(self, app_and_registry):
        app, registry, _svc = app_and_registry
        registry.plugin_manager.find_plugin_with_capability.return_value = None
        with app.test_client() as client:
            resp = client.get("/api/article/tts/status")
        assert resp.status_code == 200
        assert resp.get_json()["available"] is False


class TestDeleteRoute:
    def test_delete_clears_frontmatter(self, app_and_registry):
        app, registry, svc = app_and_registry
        # 先写入一个 audio 字段
        _ok, body, fm, _ = svc.read_file_with_frontmatter("post/demo/index.md")
        fm[FM_AUDIO] = "https://r2.example.com/old.mp3"
        fm[FM_AUDIO_ID] = "key-old"
        svc.save_file("post/demo/index.md", body, frontmatter_data=fm)

        stub = MagicMock()
        registry.plugin_manager.find_plugin_with_capability.return_value = {
            "name": "tts-gen"
        }
        registry.plugin_manager.get_tts_generator_stub.return_value = stub

        with app.test_client() as client:
            resp = client.delete(
                "/api/article/tts",
                json={"article_path": "post/demo/index.md"},
            )
        assert resp.status_code == 200
        data = resp.get_json()
        # 删除响应必须回传新 mtime（mtime 一致性，见 design Decision 8）
        assert data["success"] is True
        assert data["mtime"], "delete 响应必须携带 mtime"
        _ok, _body, fm2, _ = svc.read_file_with_frontmatter("post/demo/index.md")
        assert FM_AUDIO not in fm2
        assert FM_AUDIO_ID not in fm2
        # 插件被通知删除
        stub.Delete.assert_called_once()
