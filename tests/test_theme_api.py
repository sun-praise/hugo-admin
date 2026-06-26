# coding: utf-8
"""
主题管理 API 测试
"""

import pytest

import app as app_module
from services.active_project import ActiveProjectRegistry


@pytest.fixture(autouse=True)
def _isolate_active_project(tmp_path):
    """
    把持久化的活跃项目指向临时空目录，避免上一个测试或手动操作
    留下的 themes/ 污染 ``/api/themes`` 的结果。
    """
    from pathlib import Path

    empty_site = tmp_path / "empty-blog"
    empty_site.mkdir()
    file_path = Path(app_module.app.root_path) / "data" / "active_project.txt"
    ActiveProjectRegistry(file_path).record_path(empty_site)
    yield
    ActiveProjectRegistry(file_path).clear()


@pytest.fixture(autouse=True, scope="session")
def _ensure_clean_active_project_at_start():
    """
    测试会话开始时清空持久化文件，避免上一次手动操作残留的活跃项目
    影响 test_theme_api 之外的测试。
    """
    from pathlib import Path

    file_path = Path(app_module.app.root_path) / "data" / "active_project.txt"
    if file_path.exists():
        ActiveProjectRegistry(file_path).clear()


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


def test_list_themes_requires_auth(client):
    """未登录获取主题列表应返回 401。"""
    resp = client.get("/api/themes")
    assert resp.status_code == 401


def test_list_themes_returns_empty(admin_client):
    """已登录可获取空主题列表。"""
    resp = admin_client.get("/api/themes")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["themes"] == []
    assert data["active_theme"] is None


def test_install_theme_requires_auth(client):
    """未登录安装主题应返回 401。"""
    resp = client.post(
        "/api/themes/install",
        json={"repo_url": "https://example.com/t.git", "name": "x"},
    )
    assert resp.status_code == 401


def test_activate_theme_requires_auth(client):
    """未登录激活主题应返回 401。"""
    resp = client.post("/api/themes/activate", json={"name": "x"})
    assert resp.status_code == 401


def test_preview_theme_requires_auth(client):
    """未登录预览主题应返回 401。"""
    resp = client.post("/api/themes/preview", json={"name": "x"})
    assert resp.status_code == 401


def test_activate_missing_theme_returns_400(admin_client):
    """激活不存在的主题应返回 400。"""
    resp = admin_client.post("/api/themes/activate", json={"name": "notfound"})
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False


def test_preview_missing_theme_returns_400(admin_client):
    """预览不存在的主题应返回 400。"""
    resp = admin_client.post("/api/themes/preview", json={"name": "notfound"})
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False


def test_list_available_themes_returns_defaults(admin_client):
    """默认主题接口应返回 hugo-admin 维护的主题列表（含 Fried-Rice）。"""
    resp = admin_client.get("/api/themes/available")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    names = [t["name"] for t in data["available_themes"]]
    assert "Fried-Rice" in names
    fried_rice = next(t for t in data["available_themes"] if t["name"] == "Fried-Rice")
    assert "svtter/Fried-Rice" in fried_rice["repo"]


def test_list_available_themes_requires_auth(client):
    """默认主题接口也需要登录。"""
    resp = client.get("/api/themes/available")
    assert resp.status_code == 401
