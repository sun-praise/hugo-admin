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
        yield
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

    def _extract_files(kwargs):
        """从 open(data={...}) 提取 multipart 文件项（werkzeug 形态：
        (stream, filename[, content_type]) 元组或 FileStorage）。"""
        import base64 as _b64

        data_kw = kwargs.get("data")
        if not isinstance(data_kw, dict):
            return None
        for field, f in data_kw.items():
            if isinstance(f, str):
                continue  # 普通表单字段由 _extract_fields 处理
            fname, fdata = None, None
            if isinstance(f, (tuple, list)) and len(f) >= 2 and hasattr(f[0], "read"):
                fname = f[1]
                f[0].seek(0)
                fdata = f[0].read()
                f[0].seek(0)
            elif hasattr(f, "filename") and hasattr(f, "read"):
                fname = f.filename or "file"
                f.seek(0)
                fdata = f.read()
                f.seek(0)
            if fname is not None:
                if isinstance(fdata, str):
                    fdata = fdata.encode("utf-8")
                return {
                    "field": field,
                    "filename": fname,
                    "data_b64": _b64.b64encode(fdata).decode("ascii"),
                }
        return None

    def _extract_fields(kwargs):
        """提取 multipart 请求里的非文件表单字段。"""
        data_kw = kwargs.get("data")
        if not isinstance(data_kw, dict):
            return None
        fields = {k: v for k, v in data_kw.items() if isinstance(v, str)}
        return fields or None

    def _recording_open(self, *args, **kwargs):
        # 文件提取必须在 original_open 之前：werkzeug 构建请求后会关闭
        # data 里的流，之后再读会抛 "I/O operation on closed file"
        prerequest_files = _extract_files(kwargs)
        resp = original_open(self, *args, **kwargs)
        try:
            req = resp.request
            if req is not None and req.path.startswith("/api/"):
                # 请求体取自 open(json=...) 参数：resp.request 的流已被
                # 应用消费，重读会抛 400（werkzeug 不跨实例共享缓存）
                req_body = kwargs.get("json")
                if req_body is None and isinstance(kwargs.get("data"), (str, bytes)):
                    req_body = kwargs["data"]
                sample = {
                    "method": req.method,
                    "path": req.path,
                    "query": req.query_string.decode("utf-8") or None,
                    "request_json": req_body,
                    "request_files": prerequest_files,
                    "request_fields": _extract_fields(kwargs),
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
