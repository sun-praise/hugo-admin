# coding: utf-8
"""
项目初始化 API 测试
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


def test_init_project_requires_auth(client):
    """未登录访问初始化接口应返回 401。"""
    resp = client.post(
        "/api/project/init", json={"path": "/tmp/foo", "config_format": "toml"}
    )
    assert resp.status_code == 401


def test_init_project_rejects_invalid_format(admin_client):
    """非法配置文件格式应返回 400。"""
    resp = admin_client.post(
        "/api/project/init",
        json={"path": "/tmp/foo", "config_format": "xml"},
    )
    assert resp.status_code == 400
    assert resp.get_json()["success"] is False
