# coding: utf-8
"""
认证 API（/api/auth/*）与全局守卫测试。
"""

import pytest

import app as app_module


@pytest.fixture
def client(auth_store):
    """未登录客户端（守卫/登录流程测试）。auth_store 保证有一个 admin/admin 账户。"""
    app_module.app.config["TESTING"] = True
    with app_module.app.test_client() as c:
        yield c


# ---------- /api/auth/login ----------


def test_login_success(client):
    resp = client.post(
        "/api/auth/login", json={"username": "admin", "password": "admin"}
    )
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["user"]["username"] == "admin"
    # 会话已建立
    assert client.get("/api/auth/me").status_code == 200


def test_login_wrong_password(client):
    resp = client.post("/api/auth/login", json={"username": "admin", "password": "bad"})
    assert resp.status_code == 401
    assert resp.get_json()["success"] is False
    # 未建立会话
    assert client.get("/api/auth/me").status_code == 401


def test_login_unknown_user(client):
    resp = client.post(
        "/api/auth/login", json={"username": "nope", "password": "admin"}
    )
    assert resp.status_code == 401


def test_login_missing_fields(client):
    resp = client.post("/api/auth/login", json={})
    assert resp.status_code == 400


# ---------- /api/auth/me ----------


def test_me_logged_out_returns_401(client):
    assert client.get("/api/auth/me").status_code == 401


# ---------- /api/auth/logout ----------


def test_logout_clears_session(client, login):
    login(client)
    assert client.get("/api/auth/me").status_code == 200
    resp = client.post("/api/auth/logout")
    assert resp.status_code == 200
    assert resp.get_json()["success"] is True
    assert client.get("/api/auth/me").status_code == 401


# ---------- /api/auth/password ----------


def test_change_password_success_rotates(client, login):
    login(client)
    resp = client.post(
        "/api/auth/password",
        json={"current_password": "admin", "new_password": "newpw"},
    )
    assert resp.status_code == 200
    # 旧密码失效，新密码可用
    assert (
        client.post(
            "/api/auth/login", json={"username": "admin", "password": "admin"}
        ).status_code
        == 401
    )
    assert (
        client.post(
            "/api/auth/login", json={"username": "admin", "password": "newpw"}
        ).status_code
        == 200
    )


def test_change_password_wrong_current(client, login):
    login(client)
    resp = client.post(
        "/api/auth/password",
        json={"current_password": "bad", "new_password": "newpw"},
    )
    # 当前密码错误是校验失败（400），不是会话过期（401）
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False


def test_change_password_requires_session(client):
    resp = client.post(
        "/api/auth/password",
        json={"current_password": "admin", "new_password": "newpw"},
    )
    assert resp.status_code == 401


# ---------- 全局守卫 ----------


def test_guard_rejects_logged_out(client):
    resp = client.get("/api/git/commits")
    assert resp.status_code == 401
    assert resp.get_json()["success"] is False


def test_guard_allowlist_stays_public(client):
    # /api/version 公开
    assert client.get("/api/version").status_code == 200
    # /api/auth/me 虽返回 401，但走自身逻辑，并非被守卫拦截（路径在白名单）
    assert client.get("/api/auth/me").status_code == 401


def test_guard_allows_logged_in(client, login):
    login(client)
    resp = client.get("/api/git/commits")
    # 登录后不再被守卫 401（具体 200/500 取决于仓库状态）
    assert resp.status_code != 401
