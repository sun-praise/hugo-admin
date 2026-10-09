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


# ============ API 契约录制（Go 重写验收基准） ============
#
# ``RECORD_CONTRACT=1 pytest ...`` 时，把所有经过 Flask 测试客户端的
# /api/* 请求与响应追加写入 CONTRACT_OUT（默认 contracts/api_samples.jsonl），
# 每行一个样本：{method, path, query, request_json, status, response_json, source}。
# Go 侧按样本回放比对状态码与响应体。默认关闭，不影响正常测试。

import json as _json
import os as _os


@pytest.fixture(autouse=True)
def _record_api_contract(request):
    if _os.environ.get("RECORD_CONTRACT") != "1":
        return
    from flask.testing import FlaskClient

    out_path = Path(_os.environ.get("CONTRACT_OUT", "contracts/api_samples.jsonl"))
    out_path.parent.mkdir(parents=True, exist_ok=True)
    original_open = FlaskClient.open

    def _decode_body(body):
        try:
            text = body.decode("utf-8")
        except (UnicodeDecodeError, AttributeError):
            return None
        try:
            return _json.loads(text)
        except ValueError:
            return text[:200]

    def _recording_open(self, *args, **kwargs):
        resp = original_open(self, *args, **kwargs)
        try:
            req = resp.request
            if req is not None and req.path.startswith("/api/"):
                sample = {
                    "method": req.method,
                    "path": req.path,
                    "query": req.query_string.decode("utf-8") or None,
                    "request_json": _decode_body(req.get_data()),
                    "status": resp.status_code,
                    "response_json": _decode_body(resp.get_data()),
                    "source": request.node.nodeid,
                }
                with open(out_path, "a", encoding="utf-8") as f:
                    f.write(_json.dumps(sample, ensure_ascii=False) + "\n")
        except Exception as e:  # 录制失败不影响测试本身
            print(f"⚠ 契约录制失败: {e}")
        return resp

    FlaskClient.open = _recording_open
    yield
    FlaskClient.open = original_open
