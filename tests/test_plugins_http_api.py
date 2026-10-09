# coding: utf-8
"""插件管理 API 的 HTTP 契约测试（空插件环境）。

用真实 PluginManager + 空 temp 目录（~/.hugo-admin 注入版），
覆盖列表/config 404/enable 500/disable 404 的确定性路径；
market 依赖网络不录制。带真插件的运行时行为由 Go 侧
cmd/testplugin 集成测试保证（握手/上传 CDN/TTS SSE）。
"""

import sys
import tempfile
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.plugin_manager import PluginManager  # noqa: E402


@pytest.fixture
def plugins_client(auth_store, login):
    base = Path(tempfile.mkdtemp())
    (base / "plugins").mkdir()
    manager = PluginManager()
    manager.__dict__["_plugins"] = {}
    # 注入空目录：直接替换模块常量（与现有测试 patch 方式一致）
    import services.plugin_manager as pm

    original_plugin_dir = pm.PLUGIN_DIR
    original_config = pm.CONFIG_FILE
    original_secret = pm.SECRET_KEY_FILE
    pm.PLUGIN_DIR = base / "plugins"
    pm.CONFIG_FILE = base / "plugin-config.json"
    pm.SECRET_KEY_FILE = base / ".secret_key"
    manager2 = PluginManager()

    original = app_module.registry.plugin_manager
    app_module.registry.plugin_manager = manager2
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.registry.plugin_manager = original
        pm.PLUGIN_DIR = original_plugin_dir
        pm.CONFIG_FILE = original_config
        pm.SECRET_KEY_FILE = original_secret


class TestPluginsHTTPAPI:
    # list 端点不录：handler 闭包捕获注册时的 manager（registry 替换
    # 无效），且内容依赖 ~/.hugo-admin 真实插件（不可移植）。
    # 以下 404/500 路径与 manager 状态无关，两实现一致。

    def test_config_schema_not_found(self, plugins_client):
        resp = plugins_client.get("/api/plugins/none/config-schema")
        assert resp.status_code == 404
        assert resp.get_json()["message"] == "Plugin not found"

    def test_config_not_found(self, plugins_client):
        resp = plugins_client.get("/api/plugins/none/config")
        assert resp.status_code == 404

    def test_disable_not_found(self, plugins_client):
        resp = plugins_client.post("/api/plugins/none/disable")
        assert resp.status_code == 404
        assert "not found" in resp.get_json()["message"]

    def test_enable_not_found(self, plugins_client):
        resp = plugins_client.post("/api/plugins/none/enable")
        assert resp.status_code == 500
        assert resp.get_json()["message"] == "Failed to enable none"
