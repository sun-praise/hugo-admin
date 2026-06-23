# coding: utf-8
"""
主题预览 API 测试
"""

import pytest

import app as app_module


@pytest.fixture
def client(auth_store):
    """Flask 测试客户端。"""
    app_module.app.config["TESTING"] = True
    with app_module.app.test_client() as c:
        yield c


@pytest.fixture
def admin_client(client, login):
    """已登录管理员客户端。"""
    login(client)
    return client


def test_preview_missing_theme_returns_400(admin_client):
    """预览不存在的主题应返回 400。"""
    resp = admin_client.post("/api/themes/preview", json={"name": "missing"})
    assert resp.status_code == 400
    data = resp.get_json()
    assert data["success"] is False
    assert "不存在" in data["message"]


def test_preview_does_not_persist_active_theme(admin_client, tmp_path, monkeypatch):
    """预览不应改变持久化的活跃主题。"""
    monkeypatch.setattr(
        app_module.registry.hugo_manager, "hugo_root", tmp_path, raising=False
    )
    (tmp_path / "themes" / "previewtheme").mkdir(parents=True)

    original_active = (
        app_module.registry.settings_service.get_settings()
        .get("theme", {})
        .get("name", "")
    )

    # 模拟 start/stop 避免真实启动 hugo
    with monkeypatch.context() as m:
        m.setattr(app_module.registry.hugo_manager, "is_running", True)
        m.setattr(
            app_module.registry.hugo_manager,
            "stop",
            lambda: (True, "stopped"),
        )
        m.setattr(
            app_module.registry.hugo_manager,
            "start",
            lambda debug=False: (True, "started"),
        )
        resp = admin_client.post("/api/themes/preview", json={"name": "previewtheme"})

    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["preview_theme"] == "previewtheme"

    active_after = (
        app_module.registry.settings_service.get_settings()
        .get("theme", {})
        .get("name", "")
    )
    assert active_after == original_active


def test_preview_stops_running_server(admin_client, tmp_path, monkeypatch):
    """预览应先停止当前运行的服务器。"""
    monkeypatch.setattr(
        app_module.registry.hugo_manager, "hugo_root", tmp_path, raising=False
    )
    (tmp_path / "themes" / "previewtheme").mkdir(parents=True)

    stopped = {"called": False}
    started = {"called": False}

    def fake_stop():
        stopped["called"] = True
        return True, "stopped"

    def fake_start(debug=False):
        started["called"] = True
        return True, "started"

    monkeypatch.setattr(app_module.registry.hugo_manager, "is_running", True)
    monkeypatch.setattr(app_module.registry.hugo_manager, "stop", fake_stop)
    monkeypatch.setattr(app_module.registry.hugo_manager, "start", fake_start)

    resp = admin_client.post("/api/themes/preview", json={"name": "previewtheme"})

    assert resp.status_code == 200
    assert stopped["called"] is True
    assert started["called"] is True
