# coding: utf-8
"""AI 会话 API 的 HTTP 契约测试。

只覆盖确定性强的 sessions CRUD；chat/inline-edit 的流式与 LLM 依赖
内容不适合跨实现逐字节比对，由 Go 侧 mock Anthropic 驱动的单测保证
协议格式（internal/ai 与 internal/httpapi）。
"""

import sys
import tempfile
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from models.database import Database  # noqa: E402
from services.chat_history_service import ChatHistoryService  # noqa: E402


@pytest.fixture
def ai_sessions_client(auth_store, login):
    database = Database(str(Path(tempfile.mkdtemp()) / "cache.db"))
    # 注意 patch 的是 Flask app 实例属性：handler 经 current_app 读取，
    # 模块属性与实例属性是两个存储位置（踩过的坑）
    original = app_module.app.chat_history_service
    app_module.app.chat_history_service = ChatHistoryService(database)
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.app.chat_history_service = original


class TestAISessionsHTTPAPI:
    def test_create_and_list(self, ai_sessions_client):
        resp = ai_sessions_client.post("/api/ai/sessions", json={})
        assert resp.status_code == 201
        data = resp.get_json()
        assert data["success"] is True
        assert data["title"] == "新对话"
        assert data["message_count"] == 0
        assert len(data["session_id"]) == 32

        ai_sessions_client.post("/api/ai/sessions", json={"title": "自定义会话"})

        sessions = ai_sessions_client.get("/api/ai/sessions").get_json()["sessions"]
        assert len(sessions) == 2
        assert sessions[0]["title"] == "自定义会话"
        assert sessions[1]["title"] == "新对话"

    def test_get_missing(self, ai_sessions_client):
        resp = ai_sessions_client.get("/api/ai/sessions/none")
        assert resp.status_code == 404
        assert resp.get_json()["message"] == "Session not found"

    def test_delete(self, ai_sessions_client):
        session_id = ai_sessions_client.post("/api/ai/sessions", json={}).get_json()[
            "session_id"
        ]
        resp = ai_sessions_client.delete(f"/api/ai/sessions/{session_id}")
        assert resp.status_code == 200
        assert resp.get_json()["message"] == "Session deleted"
        assert (
            ai_sessions_client.get(f"/api/ai/sessions/{session_id}").status_code == 404
        )
