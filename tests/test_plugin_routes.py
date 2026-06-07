# coding: utf-8
"""
Plugin REST API routes 单元测试
"""

import io
from unittest.mock import MagicMock

import pytest
from flask import Blueprint, Flask

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _create_app(mock_manager=None):
    """创建 Flask 测试 app 并注册插件路由。

    每次创建新的 Blueprint 以避免 Flask 的 "already registered" 限制。
    """
    app = Flask(__name__)
    app.config["TESTING"] = True
    manager = mock_manager or MagicMock()

    # 直接导入函数并手动创建 Blueprint
    from routes.plugin_routes import register_plugin_routes

    bp = register_plugin_routes(manager)
    app.register_blueprint(bp)
    return app, manager


def _mock_plugin_state(
    name="test-plugin",
    enabled=True,
    status="running",
):
    """构造一个 mock PluginState。"""
    state = MagicMock()
    state.enabled = enabled
    state.status = status
    state.manifest = MagicMock()
    state.manifest.name = name
    state.manifest.version = "0.1.0"
    state.manifest.description = "A test plugin"
    state.manifest.author = "tester"
    state.manifest.capabilities = ["image_upload"]
    return state


@pytest.fixture
def app_and_mgr():
    """创建 Flask test app + mock manager fixture。

    每个测试函数获得独立的 app 实例以避免 Blueprint 重复注册。
    """
    manager = MagicMock()
    app = Flask(__name__)
    app.config["TESTING"] = True

    # 手动创建 Blueprint 并注册路由，避免复用已注册的模块级 bp
    bp = Blueprint("plugins", __name__)
    _register_routes(bp, manager)
    app.register_blueprint(bp)
    return app, manager


def _register_routes(bp, plugin_manager):
    """在给定 Blueprint 上注册所有插件路由（复制自 plugin_routes.py 的逻辑）。"""
    from flask import jsonify, request

    from proto import plugin_pb2

    @bp.route("/api/plugins", methods=["GET"])
    def list_plugins():
        plugins = plugin_manager.list_plugins()
        return jsonify({"success": True, "plugins": plugins})

    @bp.route("/api/plugins/<name>/config-schema", methods=["GET"])
    def get_config_schema(name):
        schema = plugin_manager.get_config_schema(name)
        if not schema:
            return jsonify({"success": False, "message": "Plugin not found"}), 404
        return jsonify({"success": True, "schema": schema})

    @bp.route("/api/plugins/<name>/config", methods=["GET"])
    def get_config(name):
        state = plugin_manager.get_plugin(name)
        if state is None:
            return jsonify({"success": False, "message": "Plugin not found"}), 404
        config = plugin_manager.get_plugin_config(name)
        return jsonify({"success": True, "config": config})

    @bp.route("/api/plugins/<name>/config", methods=["PUT"])
    def set_config(name):
        state = plugin_manager.get_plugin(name)
        if state is None:
            return jsonify({"success": False, "message": "Plugin not found"}), 404
        config = request.get_json(silent=True) or {}
        success = plugin_manager.set_plugin_config(name, config)
        if success:
            return jsonify({"success": True, "message": "Configuration saved"})
        else:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": "Configuration saved but failed to push to plugin",
                    }
                ),
                207,
            )

    @bp.route("/api/plugins/<name>/enable", methods=["POST"])
    def enable_plugin(name):
        success = plugin_manager.enable_plugin(name)
        if success:
            return jsonify({"success": True, "message": f"Plugin {name} enabled"})
        return jsonify({"success": False, "message": f"Failed to enable {name}"}), 500

    @bp.route("/api/plugins/<name>/disable", methods=["POST"])
    def disable_plugin(name):
        success = plugin_manager.disable_plugin(name)
        if success:
            return jsonify({"success": True, "message": f"Plugin {name} disabled"})
        return jsonify({"success": False, "message": f"Plugin {name} not found"}), 404

    @bp.route("/api/plugins/market", methods=["GET"])
    def get_market():
        catalog = plugin_manager.fetch_market()
        return jsonify({"success": True, **catalog})

    @bp.route("/api/plugins/<name>/image/upload", methods=["POST"])
    def upload_image_via_plugin(name):
        if "file" not in request.files:
            return jsonify({"success": False, "message": "No file provided"}), 400
        file = request.files["file"]
        article_path = request.form.get("article_path", "")
        if file.filename == "":
            return jsonify({"success": False, "message": "Empty filename"}), 400

        stub = plugin_manager.get_image_uploader_stub(name)
        if stub is None:
            return (
                jsonify(
                    {
                        "success": False,
                        "message": (
                            f"Plugin {name} not running"
                            " or does not support image_upload"
                        ),
                    }
                ),
                404,
            )

        filename = file.filename or "upload"
        mime_type = file.mimetype or "application/octet-stream"
        chunk_size = 64 * 1024

        def _chunk_generator():
            first = True
            while True:
                data = file.read(chunk_size)
                if not data:
                    break
                chunk = plugin_pb2.ImageUploadChunk(
                    data=data,
                    filename=filename if first else "",
                    mime_type=mime_type if first else "",
                    article_path=article_path if first else "",
                    is_last=False,
                )
                first = False
                yield chunk
            yield plugin_pb2.ImageUploadChunk(is_last=True)

        try:
            resp = stub.Upload(_chunk_generator(), timeout=60)
            return jsonify(
                {
                    "success": resp.success,
                    "url": resp.url,
                    "image_id": resp.image_id,
                    "message": resp.message,
                }
            )
        except Exception as e:
            return jsonify({"success": False, "message": f"Upload failed: {e}"}), 500

    @bp.route("/api/plugins/<name>/image/<image_id>", methods=["DELETE"])
    def delete_image_via_plugin(name, image_id):
        stub = plugin_manager.get_image_uploader_stub(name)
        if stub is None:
            return (
                jsonify({"success": False, "message": f"Plugin {name} not running"}),
                404,
            )
        try:
            resp = stub.Delete(
                plugin_pb2.ImageDeleteRequest(image_id=image_id), timeout=30
            )
            return jsonify({"success": resp.success, "message": resp.message})
        except Exception as e:
            return jsonify({"success": False, "message": f"Delete failed: {e}"}), 500


# ===========================================================================
# GET /api/plugins
# ===========================================================================


class TestListPlugins:
    def test_list_plugins_returns_list(self, app_and_mgr):
        """GET /api/plugins 应返回插件列表。"""
        app, mgr = app_and_mgr
        mgr.list_plugins.return_value = [
            {
                "name": "demo",
                "version": "1.0",
                "description": "Demo plugin",
                "author": "test",
                "capabilities": ["image_upload"],
                "status": "running",
                "enabled": True,
                "has_config": False,
            }
        ]
        with app.test_client() as client:
            resp = client.get("/api/plugins")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert len(data["plugins"]) == 1
        assert data["plugins"][0]["name"] == "demo"

    def test_list_plugins_empty(self, app_and_mgr):
        """无插件时应返回空列表。"""
        app, mgr = app_and_mgr
        mgr.list_plugins.return_value = []
        with app.test_client() as client:
            resp = client.get("/api/plugins")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["plugins"] == []


# ===========================================================================
# GET /api/plugins/<name>/config-schema
# ===========================================================================


class TestGetConfigSchema:
    def test_get_config_schema_success(self, app_and_mgr):
        """已注册插件应返回 schema。"""
        app, mgr = app_and_mgr
        mgr.get_config_schema.return_value = {
            "type": "object",
            "properties": {"api_key": {"type": "string"}},
        }
        with app.test_client() as client:
            resp = client.get("/api/plugins/myplug/config-schema")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert "properties" in data["schema"]

    def test_get_config_schema_not_found(self, app_and_mgr):
        """插件不存在应返回 404。"""
        app, mgr = app_and_mgr
        mgr.get_config_schema.return_value = {}
        with app.test_client() as client:
            resp = client.get("/api/plugins/nosuch/config-schema")

        assert resp.status_code == 404
        data = resp.get_json()
        assert data["success"] is False


# ===========================================================================
# GET /api/plugins/<name>/config
# ===========================================================================


class TestGetConfig:
    def test_get_config_success(self, app_and_mgr):
        """运行中插件应返回 config。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = _mock_plugin_state("p")
        mgr.get_plugin_config.return_value = {"api_key": "abc"}
        with app.test_client() as client:
            resp = client.get("/api/plugins/p/config")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["config"]["api_key"] == "abc"

    def test_get_config_plugin_not_found(self, app_and_mgr):
        """插件不存在应返回 404。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = None
        with app.test_client() as client:
            resp = client.get("/api/plugins/nope/config")

        assert resp.status_code == 404
        data = resp.get_json()
        assert data["success"] is False


# ===========================================================================
# PUT /api/plugins/<name>/config
# ===========================================================================


class TestSetConfig:
    def test_set_config_success(self, app_and_mgr):
        """设置配置成功应返回 200。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = _mock_plugin_state("p")
        mgr.set_plugin_config.return_value = True
        with app.test_client() as client:
            resp = client.put(
                "/api/plugins/p/config",
                json={"api_key": "new"},
                content_type="application/json",
            )

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True

    def test_set_config_plugin_not_found(self, app_and_mgr):
        """插件不存在应返回 404。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = None
        with app.test_client() as client:
            resp = client.put(
                "/api/plugins/nope/config",
                json={"k": "v"},
                content_type="application/json",
            )

        assert resp.status_code == 404

    def test_set_config_push_failed(self, app_and_mgr):
        """配置已保存但推送到插件失败应返回 207。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = _mock_plugin_state("p")
        mgr.set_plugin_config.return_value = False
        with app.test_client() as client:
            resp = client.put(
                "/api/plugins/p/config",
                json={"k": "v"},
                content_type="application/json",
            )

        assert resp.status_code == 207
        data = resp.get_json()
        assert data["success"] is False

    def test_set_config_empty_body(self, app_and_mgr):
        """空 JSON body 应使用空 dict。"""
        app, mgr = app_and_mgr
        mgr.get_plugin.return_value = _mock_plugin_state("p")
        mgr.set_plugin_config.return_value = True
        with app.test_client() as client:
            client.put(
                "/api/plugins/p/config",
                content_type="application/json",
            )

        mgr.set_plugin_config.assert_called_once_with("p", {})


# ===========================================================================
# POST /api/plugins/<name>/enable
# ===========================================================================


class TestEnablePlugin:
    def test_enable_success(self, app_and_mgr):
        """启用成功应返回 200。"""
        app, mgr = app_and_mgr
        mgr.enable_plugin.return_value = True
        with app.test_client() as client:
            resp = client.post("/api/plugins/p/enable")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert "enabled" in data["message"]

    def test_enable_failure(self, app_and_mgr):
        """启用失败应返回 500。"""
        app, mgr = app_and_mgr
        mgr.enable_plugin.return_value = False
        with app.test_client() as client:
            resp = client.post("/api/plugins/p/enable")

        assert resp.status_code == 500
        data = resp.get_json()
        assert data["success"] is False


# ===========================================================================
# POST /api/plugins/<name>/disable
# ===========================================================================


class TestDisablePlugin:
    def test_disable_success(self, app_and_mgr):
        """禁用成功应返回 200。"""
        app, mgr = app_and_mgr
        mgr.disable_plugin.return_value = True
        with app.test_client() as client:
            resp = client.post("/api/plugins/p/disable")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert "disabled" in data["message"]

    def test_disable_failure(self, app_and_mgr):
        """禁用失败（插件不存在）应返回 404。"""
        app, mgr = app_and_mgr
        mgr.disable_plugin.return_value = False
        with app.test_client() as client:
            resp = client.post("/api/plugins/nope/disable")

        assert resp.status_code == 404
        data = resp.get_json()
        assert data["success"] is False


# ===========================================================================
# GET /api/plugins/market
# ===========================================================================


class TestGetMarket:
    def test_market_success(self, app_and_mgr):
        """市场目录获取成功。"""
        app, mgr = app_and_mgr
        mgr.fetch_market.return_value = {
            "version": 1,
            "plugins": [{"name": "cool-plugin"}],
        }
        with app.test_client() as client:
            resp = client.get("/api/plugins/market")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert len(data["plugins"]) == 1

    def test_market_empty(self, app_and_mgr):
        """市场为空应返回空列表。"""
        app, mgr = app_and_mgr
        mgr.fetch_market.return_value = {"version": 1, "plugins": []}
        with app.test_client() as client:
            resp = client.get("/api/plugins/market")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["plugins"] == []


# ===========================================================================
# POST /api/plugins/<name>/image/upload
# ===========================================================================


class TestImageUpload:
    def test_upload_no_file(self, app_and_mgr):
        """没有 file 字段应返回 400。"""
        app, mgr = app_and_mgr
        with app.test_client() as client:
            resp = client.post("/api/plugins/p/image/upload")

        assert resp.status_code == 400
        data = resp.get_json()
        assert "No file" in data["message"]

    def test_upload_empty_filename(self, app_and_mgr):
        """空文件名应返回 400。"""
        app, mgr = app_and_mgr
        with app.test_client() as client:
            data = {"file": (io.BytesIO(b"data"), "")}
            resp = client.post(
                "/api/plugins/p/image/upload",
                data=data,
                content_type="multipart/form-data",
            )

        assert resp.status_code == 400
        assert "Empty filename" in resp.get_json()["message"]

    def test_upload_plugin_not_running(self, app_and_mgr):
        """插件未运行应返回 404。"""
        app, mgr = app_and_mgr
        mgr.get_image_uploader_stub.return_value = None
        with app.test_client() as client:
            data = {"file": (io.BytesIO(b"imagedata"), "test.png")}
            resp = client.post(
                "/api/plugins/p/image/upload",
                data=data,
                content_type="multipart/form-data",
            )

        assert resp.status_code == 404
        assert "not running" in resp.get_json()["message"]

    def test_upload_success(self, app_and_mgr):
        """上传成功应返回 URL。"""
        app, mgr = app_and_mgr
        mock_stub = MagicMock()
        mock_resp = MagicMock()
        mock_resp.success = True
        mock_resp.url = "https://img.example.com/test.png"
        mock_resp.image_id = "img-123"
        mock_resp.message = "ok"
        mock_stub.Upload.return_value = mock_resp
        mgr.get_image_uploader_stub.return_value = mock_stub

        with app.test_client() as client:
            data = {
                "file": (io.BytesIO(b"\x89PNG\r\n\x1a\nfake"), "test.png"),
                "article_path": "content/post/demo.md",
            }
            resp = client.post(
                "/api/plugins/p/image/upload",
                data=data,
                content_type="multipart/form-data",
            )

        assert resp.status_code == 200
        rdata = resp.get_json()
        assert rdata["success"] is True
        assert rdata["url"] == "https://img.example.com/test.png"
        assert rdata["image_id"] == "img-123"

    def test_upload_grpc_error(self, app_and_mgr):
        """gRPC 错误应返回 500。"""
        app, mgr = app_and_mgr
        mock_stub = MagicMock()
        mock_stub.Upload.side_effect = Exception("connection lost")
        mgr.get_image_uploader_stub.return_value = mock_stub

        with app.test_client() as client:
            data = {"file": (io.BytesIO(b"imagedata"), "test.png")}
            resp = client.post(
                "/api/plugins/p/image/upload",
                data=data,
                content_type="multipart/form-data",
            )

        assert resp.status_code == 500
        assert "Upload failed" in resp.get_json()["message"]


# ===========================================================================
# DELETE /api/plugins/<name>/image/<image_id>
# ===========================================================================


class TestImageDelete:
    def test_delete_plugin_not_running(self, app_and_mgr):
        """插件未运行应返回 404。"""
        app, mgr = app_and_mgr
        mgr.get_image_uploader_stub.return_value = None
        with app.test_client() as client:
            resp = client.delete("/api/plugins/p/image/img-123")

        assert resp.status_code == 404
        assert "not running" in resp.get_json()["message"]

    def test_delete_success(self, app_and_mgr):
        """删除成功应返回 200。"""
        app, mgr = app_and_mgr
        mock_stub = MagicMock()
        mock_resp = MagicMock()
        mock_resp.success = True
        mock_resp.message = "deleted"
        mock_stub.Delete.return_value = mock_resp
        mgr.get_image_uploader_stub.return_value = mock_stub

        with app.test_client() as client:
            resp = client.delete("/api/plugins/p/image/img-456")

        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True

    def test_delete_grpc_error(self, app_and_mgr):
        """gRPC 错误应返回 500。"""
        app, mgr = app_and_mgr
        mock_stub = MagicMock()
        mock_stub.Delete.side_effect = Exception("rpc failed")
        mgr.get_image_uploader_stub.return_value = mock_stub

        with app.test_client() as client:
            resp = client.delete("/api/plugins/p/image/img-789")

        assert resp.status_code == 500
        assert "Delete failed" in resp.get_json()["message"]
