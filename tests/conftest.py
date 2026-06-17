# coding: utf-8
"""
共享测试夹具。

新增的全局认证守卫会让所有未登录的 /api/* 请求返回 401。这里提供：

- ``auth_store``：把 ``registry.auth_service`` 指向一个临时凭据文件
  （固定 admin/admin），既避免测试写仓库的 data/auth.json，又让守卫有
  一个已知账户可用。
- ``login``：返回一个辅助函数，把给定的 Flask 测试客户端登录为 admin。
  受守卫影响的 HTTP 测试在其 ``client`` 夹具里调用 ``login(client)`` 即可。
"""

import tempfile
from pathlib import Path

import pytest

import app as app_module
from services.auth_service import AuthService


@pytest.fixture
def auth_store():
    """临时认证存储：registry.auth_service 指向 temp 文件（admin/admin）。"""
    with tempfile.TemporaryDirectory() as tmp:
        store = Path(tmp) / "auth.json"
        auth = AuthService(store, default_username="admin", default_password="admin")
        original = app_module.registry.auth_service
        app_module.registry.auth_service = auth
        try:
            yield auth
        finally:
            app_module.registry.auth_service = original


@pytest.fixture
def login(auth_store):
    """返回登录辅助：``login(client)`` 将该客户端登录为 admin。"""

    def _login(client, username="admin", password="admin"):
        resp = client.post(
            "/api/auth/login",
            json={"username": username, "password": password},
        )
        assert resp.status_code == 200, resp.get_json()
        return client

    return _login
