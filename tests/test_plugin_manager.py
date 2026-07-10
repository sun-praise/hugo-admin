# coding: utf-8
"""
PluginManager 和 PluginConfigStore 单元测试
"""

import json
import signal
import subprocess
import sys
import threading
import time
from pathlib import Path
from unittest.mock import MagicMock, patch

import grpc
import pytest

from services.plugin_manager import (
    HANDSHAKE_TIMEOUT_SECONDS,
    PluginConfigStore,
    PluginManager,
    PluginState,
)
from services.plugin_manifest import ManifestError, PluginManifest

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_manifest(
    name="test-plugin",
    version="0.1.0",
    entry="bin/plugin",
    capabilities=None,
    platform="",
    arch="",
    config_schema=None,
    plugin_dir=None,
) -> PluginManifest:
    """构造一个最小 PluginManifest。"""
    return PluginManifest(
        name=name,
        version=version,
        entry=entry,
        capabilities=capabilities or ["image_upload"],
        platform=platform,
        arch=arch,
        config_schema=config_schema or {},
        plugin_dir=plugin_dir or Path("/tmp/fake"),
    )


def _make_state(
    name="test-plugin",
    enabled=True,
    status="running",
    process=None,
    stub=None,
    channel=None,
    config_schema=None,
    process_poll=None,
) -> PluginState:
    """构造一个 PluginState。"""
    manifest = _make_manifest(name=name, config_schema=config_schema)
    if process is not None:
        proc = process
    else:
        proc = MagicMock(spec=subprocess.Popen)
        proc.poll.return_value = process_poll if process_poll is not None else None
        proc.pid = 12345
    return PluginState(
        manifest=manifest,
        process=proc,
        port=50051,
        channel=channel or MagicMock(),
        stub=stub or MagicMock(),
        enabled=enabled,
        status=status,
    )


def _write_plugin_toml(
    plugin_dir: Path,
    name="demo",
    version="1.0.0",
    entry="bin/run",
    capabilities=None,
):
    """在 plugin_dir 下写一个合法的 plugin.toml。"""
    caps = capabilities or {"image_upload": True}
    cap_lines = "\n".join(f"{k} = {v}" for k, v in caps.items())
    toml_text = f"""
[plugin]
name = "{name}"
version = "{version}"
entry = "{entry}"

[capabilities]
{cap_lines}
"""
    plugin_dir.mkdir(parents=True, exist_ok=True)
    (plugin_dir / "plugin.toml").write_text(toml_text, encoding="utf-8")
    return plugin_dir


# ===========================================================================
# PluginConfigStore 测试
# ===========================================================================


class TestPluginConfigStore:
    """PluginConfigStore: Fernet 加密、load/save、线程安全。"""

    @pytest.fixture
    def store(self, tmp_path):
        """用 tmp_path 创建隔离的 PluginConfigStore。"""
        config_file = tmp_path / "plugin-config.json"
        secret_file = tmp_path / ".secret_key"
        with patch("services.plugin_manager.SECRET_KEY_FILE", secret_file):
            with patch("services.plugin_manager.CONFIG_FILE", config_file):
                s = PluginConfigStore(config_path=config_file)
                yield s

    # -- 基本 get/set --

    def test_set_and_get_roundtrip(self, store):
        """加密后存储的值应能完整解密回来。"""
        cfg = {"api_key": "secret123", "timeout": 30}
        store.set_config("my-plugin", cfg)
        result = store.get_config("my-plugin")
        assert result["api_key"] == "secret123"
        assert result["timeout"] == 30

    def test_get_nonexistent_returns_empty(self, store):
        """查询不存在的插件应返回空 dict。"""
        assert store.get_config("no-such-plugin") == {}

    def test_overwrite_config(self, store):
        """覆盖写入后应返回最新值。"""
        store.set_config("p", {"k": "v1"})
        store.set_config("p", {"k": "v2"})
        assert store.get_config("p")["k"] == "v2"

    def test_empty_string_not_encrypted(self, store):
        """空字符串不应被加密。"""
        store.set_config("p", {"key": ""})
        assert store.get_config("p")["key"] == ""

    def test_non_string_values_preserved(self, store):
        """非字符串值（int, bool, list）应原样保存。"""
        cfg = {"count": 42, "flag": True, "tags": ["a", "b"]}
        store.set_config("p", cfg)
        result = store.get_config("p")
        assert result["count"] == 42
        assert result["flag"] is True
        assert result["tags"] == ["a", "b"]

    # -- 持久化 --

    def test_config_persists_to_disk(self, store, tmp_path):
        """配置应持久化到 JSON 文件。"""
        config_file = tmp_path / "plugin-config.json"
        store.set_config("p", {"token": "abc"})
        assert config_file.exists()
        raw = json.loads(config_file.read_text(encoding="utf-8"))
        assert "p" in raw
        # 存储值应该是加密的
        assert raw["p"]["token"].startswith("_enc:")

    def test_config_reloaded_from_disk(self, tmp_path):
        """从磁盘加载已有配置应能正确解密。"""
        config_file = tmp_path / "plugin-config.json"
        secret_file = tmp_path / ".secret_key"
        with patch("services.plugin_manager.SECRET_KEY_FILE", secret_file):
            with patch("services.plugin_manager.CONFIG_FILE", config_file):
                store1 = PluginConfigStore(config_path=config_file)
                store1.set_config("p", {"password": "hunter2"})
                # 新实例加载同一文件
                store2 = PluginConfigStore(config_path=config_file)
                assert store2.get_config("p")["password"] == "hunter2"

    def test_corrupt_config_file_graceful(self, tmp_path):
        """损坏的 JSON 文件不应崩溃，应返回空 dict。"""
        config_file = tmp_path / "plugin-config.json"
        secret_file = tmp_path / ".secret_key"
        config_file.write_text("NOT JSON{{{", encoding="utf-8")
        with patch("services.plugin_manager.SECRET_KEY_FILE", secret_file):
            with patch("services.plugin_manager.CONFIG_FILE", config_file):
                store = PluginConfigStore(config_path=config_file)
                assert store.get_config("any") == {}

    # -- 线程安全 --

    def test_concurrent_set_get(self, store):
        """多线程并发读写不应丢失数据。"""
        errors = []

        def writer(plugin_name, value, count=20):
            try:
                for i in range(count):
                    store.set_config(plugin_name, {"val": f"{value}-{i}"})
            except Exception as e:
                errors.append(e)

        threads = [
            threading.Thread(target=writer, args=(f"plugin-{i}", f"v{i}"))
            for i in range(5)
        ]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert not errors, f"并发错误: {errors}"
        # 每个 plugin 应有最后一次写入的值
        for i in range(5):
            result = store.get_config(f"plugin-{i}")
            assert result["val"].startswith(f"v{i}-")

    # -- Fernet 密钥管理 --

    def test_secret_key_file_created(self, tmp_path):
        """首次使用时应自动生成密钥文件。"""
        secret_file = tmp_path / ".secret_key"
        config_file = tmp_path / "plugin-config.json"
        with patch("services.plugin_manager.SECRET_KEY_FILE", secret_file):
            with patch("services.plugin_manager.CONFIG_FILE", config_file):
                store = PluginConfigStore(config_path=config_file)
                store.set_config("p", {"k": "v"})
        assert secret_file.exists()

    def test_same_key_decrypts_across_instances(self, tmp_path):
        """使用同一密钥文件的两个实例应能互相解密。"""
        secret_file = tmp_path / ".secret_key"
        config_file = tmp_path / "plugin-config.json"
        with patch("services.plugin_manager.SECRET_KEY_FILE", secret_file):
            with patch("services.plugin_manager.CONFIG_FILE", config_file):
                s1 = PluginConfigStore(config_path=config_file)
                s1.set_config("p", {"secret": "value1"})
                s2 = PluginConfigStore(config_path=config_file)
                assert s2.get_config("p")["secret"] == "value1"


# ===========================================================================
# PluginManager 测试
# ===========================================================================


class TestPluginManagerDiscover:
    """PluginManager.discover_plugins() 测试。"""

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_empty_dir(self, mock_dir, tmp_path):
        """空插件目录应返回空列表。"""
        mock_dir.__str__ = lambda s: str(tmp_path)
        mock_dir.iterdir.return_value = []
        mock_dir.mkdir = MagicMock()
        pm = PluginManager()
        assert pm.discover_plugins() == []

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_valid_plugin(self, mock_dir, tmp_path):
        """合法 plugin.toml 应被正确发现。"""
        plugin_path = _write_plugin_toml(tmp_path / "demo-plugin")

        mock_dir.iterdir.return_value = [plugin_path]
        mock_dir.mkdir = MagicMock()

        pm = PluginManager()
        with patch("services.plugin_manager.parse_manifest") as mock_parse:
            manifest = _make_manifest(name="demo", plugin_dir=plugin_path)
            mock_parse.return_value = manifest
            results = pm.discover_plugins()

        assert len(results) == 1
        assert results[0].name == "demo"

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_skips_files(self, mock_dir, tmp_path):
        """非目录条目应被跳过。"""
        file_entry = tmp_path / "readme.txt"
        file_entry.write_text("not a plugin")
        mock_dir.iterdir.return_value = [file_entry]
        mock_dir.mkdir = MagicMock()

        pm = PluginManager()
        results = pm.discover_plugins()
        assert results == []

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_skips_dir_without_toml(self, mock_dir, tmp_path):
        """没有 plugin.toml 的目录应被跳过。"""
        empty_dir = tmp_path / "empty-plugin"
        empty_dir.mkdir()
        mock_dir.iterdir.return_value = [empty_dir]
        mock_dir.mkdir = MagicMock()

        pm = PluginManager()
        results = pm.discover_plugins()
        assert results == []

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_skips_invalid_manifest(self, mock_dir, tmp_path):
        """ManifestError 的目录应被跳过而非中断。"""
        plugin_path = _write_plugin_toml(tmp_path / "bad-plugin")
        mock_dir.iterdir.return_value = [plugin_path]
        mock_dir.mkdir = MagicMock()

        pm = PluginManager()
        with patch("services.plugin_manager.parse_manifest") as mock_parse:
            mock_parse.side_effect = ManifestError("bad manifest")
            results = pm.discover_plugins()

        assert results == []

    @patch("services.plugin_manager.PLUGIN_DIR")
    def test_discover_multiple_plugins(self, mock_dir, tmp_path):
        """多个合法插件应全部被发现。"""
        dirs = []
        for i in range(3):
            d = _write_plugin_toml(tmp_path / f"plugin-{i}", name=f"plugin-{i}")
            dirs.append(d)

        mock_dir.iterdir.return_value = dirs
        mock_dir.mkdir = MagicMock()

        pm = PluginManager()
        with patch("services.plugin_manager.parse_manifest") as mock_parse:
            manifests = [
                _make_manifest(name=f"plugin-{i}", plugin_dir=dirs[i]) for i in range(3)
            ]
            mock_parse.side_effect = manifests
            results = pm.discover_plugins()
        assert len(results) == 3


class TestPluginManagerStartStop:
    """PluginManager start/stop lifecycle 测试。"""

    def test_start_all_starts_each_plugin(self):
        """start_all 应对每个发现的 manifest 调用 _start_plugin。"""
        pm = PluginManager()
        manifests = [_make_manifest(name=f"p{i}") for i in range(3)]
        with patch.object(pm, "discover_plugins", return_value=manifests):
            with patch.object(pm, "_start_plugin") as mock_start:
                pm.start_all()
        assert mock_start.call_count == 3

    def test_start_all_continues_on_failure(self):
        """单个插件启动失败不应阻止其他插件。"""
        pm = PluginManager()
        manifests = [_make_manifest(name="a"), _make_manifest(name="b")]
        with patch.object(pm, "discover_plugins", return_value=manifests):
            with patch.object(
                pm, "_start_plugin", side_effect=[RuntimeError("boom"), None]
            ):
                pm.start_all()
        # 没有 crash 就算通过

    def test_start_plugin_success(self):
        """_start_plugin 应成功启动子进程并通过 health check。"""
        pm = PluginManager()
        manifest = _make_manifest()

        mock_sock = MagicMock()
        mock_sock.getsockname.return_value = ("127.0.0.1", 59999)
        mock_proc = MagicMock(spec=subprocess.Popen)
        mock_proc.pid = 9999
        mock_channel = MagicMock()
        mock_stub = MagicMock()
        mock_health_resp = MagicMock()
        mock_health_resp.healthy = True
        mock_stub.HealthCheck.return_value = mock_health_resp
        mock_info = MagicMock()
        mock_info.name = "test-plugin"
        mock_info.version = "0.1.0"
        mock_info.capabilities = ["image_upload"]
        mock_stub.Info.return_value = mock_info

        # socket is imported inline inside _start_plugin, so patch sys.modules
        mock_socket_module = MagicMock()
        mock_socket_module.socket.return_value = mock_sock

        with (
            patch.object(pm, "_config_store") as mock_cs,
            patch(
                "services.plugin_manager.resolve_entry_path",
                return_value=Path("/fake/bin"),
            ),
            patch("services.plugin_manager.subprocess") as mock_subprocess,
            patch("services.plugin_manager.grpc") as mock_grpc,
            patch("services.plugin_manager.plugin_pb2_grpc") as mock_pb2_grpc,
            patch("services.plugin_manager.plugin_pb2"),
            patch("services.plugin_manager.time") as mock_time,
            patch.dict(sys.modules, {"socket": mock_socket_module}),
            patch("services.plugin_manager.os.environ", {"PATH": "/usr/bin"}),
        ):

            mock_cs.get_config.return_value = {}
            mock_subprocess.Popen.return_value = mock_proc
            mock_grpc.insecure_channel.return_value = mock_channel
            mock_pb2_grpc.PluginServiceStub.return_value = mock_stub
            mock_time.time.side_effect = [0, 1]

            pm._start_plugin(manifest)

        assert "test-plugin" in pm._plugins
        state = pm._plugins["test-plugin"]
        assert state.enabled is True
        assert state.status == "running"
        assert state.port == 59999

    def test_start_plugin_health_check_timeout(self):
        """health check 超时应 kill 子进程并抛出 RuntimeError。"""
        pm = PluginManager()
        manifest = _make_manifest()

        mock_sock = MagicMock()
        mock_sock.getsockname.return_value = ("127.0.0.1", 59998)
        mock_proc = MagicMock(spec=subprocess.Popen)
        mock_proc.pid = 9998
        mock_channel = MagicMock()
        mock_stub = MagicMock()
        mock_stub.HealthCheck.side_effect = grpc.RpcError("unavailable")

        mock_socket_module = MagicMock()
        mock_socket_module.socket.return_value = mock_sock

        start = time.time()
        with (
            patch(
                "services.plugin_manager.resolve_entry_path",
                return_value=Path("/fake/bin"),
            ),
            patch("services.plugin_manager.subprocess") as mock_subprocess,
            patch("services.plugin_manager.grpc") as mock_grpc,
            patch("services.plugin_manager.plugin_pb2_grpc") as mock_pb2_grpc,
            patch("services.plugin_manager.plugin_pb2"),
            patch("services.plugin_manager.time") as mock_time,
            patch.dict(sys.modules, {"socket": mock_socket_module}),
            patch("services.plugin_manager.os.environ", {"PATH": "/usr/bin"}),
        ):

            mock_subprocess.Popen.return_value = mock_proc
            mock_grpc.insecure_channel.return_value = mock_channel
            mock_pb2_grpc.PluginServiceStub.return_value = mock_stub
            mock_time.time.side_effect = [start, start + HANDSHAKE_TIMEOUT_SECONDS + 1]
            mock_time.sleep = MagicMock()

            with pytest.raises(RuntimeError, match="failed health check"):
                pm._start_plugin(manifest)

            mock_proc.kill.assert_called_once()
            mock_channel.close.assert_called_once()

    def test_start_plugin_already_running(self):
        """已启用的插件不应重复启动。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=True)
        pm._plugins["p"] = state

        with patch("services.plugin_manager.resolve_entry_path"):
            pm._start_plugin(state.manifest)
        # 不应创建新的 subprocess

    def test_stop_plugin_sends_sigterm(self):
        """_stop_plugin 应先发 SIGTERM。"""
        pm = PluginManager()
        proc = MagicMock(spec=subprocess.Popen)
        proc.poll.return_value = None  # still running
        proc.pid = 5555
        state = _make_state(name="p", process=proc)

        pm._stop_plugin("p", state)

        proc.send_signal.assert_called_once_with(signal.SIGTERM)
        assert state.enabled is False
        assert state.status == "stopped"
        assert state.process is None

    def test_stop_plugin_sigkill_on_timeout(self):
        """SIGTERM 超时后应发送 SIGKILL。"""
        pm = PluginManager()
        proc = MagicMock(spec=subprocess.Popen)
        proc.poll.return_value = None
        proc.pid = 5556
        proc.wait.side_effect = [subprocess.TimeoutExpired(cmd="p", timeout=5), None]
        state = _make_state(name="p", process=proc)

        pm._stop_plugin("p", state)

        proc.kill.assert_called_once()

    def test_stop_plugin_skips_dead_process(self):
        """已退出的进程不应再发信号。"""
        pm = PluginManager()
        proc = MagicMock(spec=subprocess.Popen)
        proc.poll.return_value = 0  # already exited
        state = _make_state(name="p", process=proc)

        pm._stop_plugin("p", state)
        proc.send_signal.assert_not_called()

    def test_stop_all(self):
        """stop_all 应停止所有 enabled 且有 process 的插件。"""
        pm = PluginManager()
        p1 = _make_state(name="a", enabled=True)
        p2 = _make_state(name="b", enabled=True)
        p3 = _make_state(name="c", enabled=False, process=None)
        pm._plugins = {"a": p1, "b": p2, "c": p3}

        with patch.object(pm, "_stop_plugin") as mock_stop:
            pm.stop_all()

        assert mock_stop.call_count == 2
        names = {call[0][0] for call in mock_stop.call_args_list}
        assert names == {"a", "b"}


class TestPluginManagerEnableDisable:
    """PluginManager enable/disable 测试。"""

    def test_enable_stopped_plugin(self):
        """enable 已停止的插件应调用 _start_plugin。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=False, status="stopped")
        pm._plugins["p"] = state

        with patch.object(pm, "_start_plugin") as mock_start:
            result = pm.enable_plugin("p")

        assert result is True
        mock_start.assert_called_once_with(state.manifest)

    def test_enable_already_running(self):
        """enable 已运行的插件应返回 True 但不重新启动。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=True)
        pm._plugins["p"] = state

        with patch.object(pm, "_start_plugin") as mock_start:
            result = pm.enable_plugin("p")

        assert result is True
        mock_start.assert_not_called()

    def test_enable_nonexistent(self):
        """enable 不存在的插件应返回 False。"""
        pm = PluginManager()
        assert pm.enable_plugin("nope") is False

    def test_enable_start_failure(self):
        """启动失败应返回 False。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=False, status="stopped")
        pm._plugins["p"] = state

        with patch.object(pm, "_start_plugin", side_effect=RuntimeError("fail")):
            result = pm.enable_plugin("p")

        assert result is False

    def test_disable_running_plugin(self):
        """disable 运行中的插件应调用 _stop_plugin。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=True)
        pm._plugins["p"] = state

        with patch.object(pm, "_stop_plugin") as mock_stop:
            result = pm.disable_plugin("p")

        assert result is True
        mock_stop.assert_called_once_with("p", state)

    def test_disable_already_stopped(self):
        """disable 已停止的插件应返回 True。"""
        pm = PluginManager()
        state = _make_state(name="p", enabled=False)
        pm._plugins["p"] = state

        result = pm.disable_plugin("p")
        assert result is True

    def test_disable_nonexistent(self):
        """disable 不存在的插件应返回 False。"""
        pm = PluginManager()
        assert pm.disable_plugin("nope") is False


class TestPluginManagerStubs:
    """PluginManager gRPC stub 获取测试。"""

    def test_get_stub_running(self):
        """运行中的插件应返回 stub。"""
        pm = PluginManager()
        stub = MagicMock()
        pm._plugins["p"] = _make_state(name="p", stub=stub, enabled=True)
        assert pm.get_stub("p") is stub

    def test_get_stub_stopped(self):
        """已停止的插件应返回 None。"""
        pm = PluginManager()
        pm._plugins["p"] = _make_state(name="p", enabled=False, stub=None)
        assert pm.get_stub("p") is None

    def test_get_stub_nonexistent(self):
        """不存在的插件应返回 None。"""
        pm = PluginManager()
        assert pm.get_stub("nope") is None

    @patch("services.plugin_manager.plugin_pb2_grpc")
    def test_get_image_uploader_stub_running(self, mock_pb2_grpc):
        """运行中的插件应创建 ImageUploaderStub。"""
        pm = PluginManager()
        channel = MagicMock()
        pm._plugins["p"] = _make_state(name="p", enabled=True, channel=channel)

        mock_stub = MagicMock()
        mock_pb2_grpc.ImageUploaderStub.return_value = mock_stub

        result = pm.get_image_uploader_stub("p")
        assert result is mock_stub
        mock_pb2_grpc.ImageUploaderStub.assert_called_once_with(channel)

    def test_get_image_uploader_stub_stopped(self):
        """已停止的插件应返回 None。"""
        pm = PluginManager()
        pm._plugins["p"] = _make_state(name="p", enabled=False, channel=None)
        assert pm.get_image_uploader_stub("p") is None

    @patch("services.plugin_manager.plugin_pb2_grpc")
    def test_get_tts_generator_stub_running(self, mock_pb2_grpc):
        """运行中的插件应创建 TTSGeneratorStub。"""
        pm = PluginManager()
        channel = MagicMock()
        pm._plugins["p"] = _make_state(name="p", enabled=True, channel=channel)

        mock_stub = MagicMock()
        mock_pb2_grpc.TTSGeneratorStub.return_value = mock_stub

        result = pm.get_tts_generator_stub("p")
        assert result is mock_stub
        mock_pb2_grpc.TTSGeneratorStub.assert_called_once_with(channel)

    def test_get_tts_generator_stub_stopped(self):
        """已停止的插件应返回 None。"""
        pm = PluginManager()
        pm._plugins["p"] = _make_state(name="p", enabled=False, channel=None)
        assert pm.get_tts_generator_stub("p") is None

    def test_find_plugin_with_capability_match(self):
        """应返回第一个声明该能力且运行中的插件。"""
        pm = PluginManager()
        pm._plugins["a"] = _make_state(name="a", enabled=True, status="running")
        pm._plugins["a"].manifest.capabilities = ["tts_generation"]
        # list_plugins 从 manifest 读 capabilities
        found = pm.find_plugin_with_capability("tts_generation")
        assert found is not None
        assert found["name"] == "a"

    def test_find_plugin_with_capability_no_match(self):
        """无匹配能力时返回 None。"""
        pm = PluginManager()
        pm._plugins["a"] = _make_state(name="a", enabled=True, status="running")
        pm._plugins["a"].manifest.capabilities = ["image_upload"]
        assert pm.find_plugin_with_capability("tts_generation") is None

    def test_find_plugin_with_capability_skips_disabled(self):
        """已停用插件即便声明能力也不返回。"""
        pm = PluginManager()
        pm._plugins["a"] = _make_state(name="a", enabled=False, status="stopped")
        pm._plugins["a"].manifest.capabilities = ["tts_generation"]
        assert pm.find_plugin_with_capability("tts_generation") is None


class TestPluginManagerConfig:
    """PluginManager config 操作测试。"""

    def test_get_plugin_config(self):
        """get_plugin_config 应委托给 config store。"""
        pm = PluginManager()
        with patch.object(pm, "_config_store") as mock_cs:
            mock_cs.get_config.return_value = {"key": "val"}
            result = pm.get_plugin_config("p")
        assert result == {"key": "val"}

    def test_set_plugin_config_running(self):
        """set_plugin_config 运行中插件应推送到 gRPC。"""
        pm = PluginManager()
        stub = MagicMock()
        mock_resp = MagicMock()
        mock_resp.success = True
        stub.SetConfig.return_value = mock_resp

        state = _make_state(name="p", stub=stub, enabled=True)
        pm._plugins["p"] = state

        with patch.object(pm, "_config_store"):
            result = pm.set_plugin_config("p", {"k": "v"})

        assert result is True

    def test_set_plugin_config_stopped(self):
        """set_plugin_config 已停止插件应只保存返回 True。"""
        pm = PluginManager()
        pm._plugins["p"] = _make_state(name="p", enabled=False, stub=None)

        with patch.object(pm, "_config_store"):
            result = pm.set_plugin_config("p", {"k": "v"})

        assert result is True

    def test_set_plugin_config_grpc_error(self):
        """set_plugin_config gRPC 失败应返回 False。"""
        pm = PluginManager()
        stub = MagicMock()
        stub.SetConfig.side_effect = grpc.RpcError("fail")
        state = _make_state(name="p", stub=stub, enabled=True)
        pm._plugins["p"] = state

        with patch.object(pm, "_config_store"):
            result = pm.set_plugin_config("p", {"k": "v"})

        assert result is False

    def test_get_config_schema_from_grpc(self):
        """get_config_schema 优先从 gRPC 获取。"""
        pm = PluginManager()
        stub = MagicMock()
        resp = MagicMock()
        resp.schema_json = '{"fields": []}'
        stub.GetConfigSchema.return_value = resp
        state = _make_state(name="p", stub=stub, enabled=True)
        pm._plugins["p"] = state

        result = pm.get_config_schema("p")
        assert result == {"fields": []}

    def test_get_config_schema_fallback_manifest(self):
        """gRPC 不可用时应 fallback 到 manifest config_schema。"""
        pm = PluginManager()
        schema = {"type": "object", "properties": {"k": {}}}
        state = _make_state(name="p", enabled=False, stub=None, config_schema=schema)
        pm._plugins["p"] = state

        result = pm.get_config_schema("p")
        assert result == schema

    def test_get_config_schema_grpc_error_fallback(self):
        """gRPC 异常时应 fallback 到 manifest。"""
        pm = PluginManager()
        stub = MagicMock()
        stub.GetConfigSchema.side_effect = grpc.RpcError("err")
        state = _make_state(
            name="p", stub=stub, enabled=True, config_schema={"fallback": True}
        )
        pm._plugins["p"] = state

        result = pm.get_config_schema("p")
        assert result == {"fallback": True}

    def test_get_config_schema_nonexistent(self):
        """不存在的插件应返回空 dict。"""
        pm = PluginManager()
        assert pm.get_config_schema("nope") == {}


class TestPluginManagerQuery:
    """PluginManager list_plugins / get_plugin 测试。"""

    def test_list_plugins(self):
        """list_plugins 应返回每个插件的状态摘要。"""
        pm = PluginManager()
        pm._plugins["a"] = _make_state(name="a")
        pm._plugins["b"] = _make_state(name="b", enabled=False, status="stopped")

        with patch.object(pm, "_config_store") as mock_cs:
            mock_cs.get_config.return_value = {}
            result = pm.list_plugins()

        assert len(result) == 2
        by_name = {r["name"]: r for r in result}
        assert by_name["a"]["enabled"] is True
        assert by_name["a"]["status"] == "running"
        assert by_name["b"]["enabled"] is False
        assert by_name["b"]["status"] == "stopped"

    def test_list_plugins_has_config(self):
        """has_config 应反映 config store 状态。"""
        pm = PluginManager()
        pm._plugins["a"] = _make_state(name="a")

        with patch.object(pm, "_config_store") as mock_cs:
            mock_cs.get_config.return_value = {"key": "val"}
            result = pm.list_plugins()

        assert result[0]["has_config"] is True

    def test_get_plugin_found(self):
        """get_plugin 找到时应返回 PluginState。"""
        pm = PluginManager()
        state = _make_state(name="p")
        pm._plugins["p"] = state
        assert pm.get_plugin("p") is state

    def test_get_plugin_not_found(self):
        """get_plugin 找不到时应返回 None。"""
        pm = PluginManager()
        assert pm.get_plugin("nope") is None


class TestPluginManagerMarket:
    """PluginManager.fetch_market() 测试。"""

    def test_fetch_market_success(self):
        """成功获取市场数据。"""
        pm = PluginManager()
        pm.__class__._market_cache = None
        pm.__class__._market_cache_time = 0

        mock_catalog = {"version": 1, "plugins": [{"name": "cool-plugin"}]}
        mock_requests_module = MagicMock()
        mock_resp = MagicMock()
        mock_resp.json.return_value = mock_catalog
        mock_resp.raise_for_status.return_value = None
        mock_requests_module.get.return_value = mock_resp

        with patch.dict(sys.modules, {"requests": mock_requests_module}):
            result = pm.fetch_market()

        assert result == mock_catalog

    def test_fetch_market_uses_cache(self):
        """TTL 内应使用缓存，不发请求。"""
        pm = PluginManager()
        pm.__class__._market_cache = {"version": 1, "plugins": []}
        pm.__class__._market_cache_time = time.time()

        mock_requests_module = MagicMock()
        with patch.dict(sys.modules, {"requests": mock_requests_module}):
            result = pm.fetch_market()

        mock_requests_module.get.assert_not_called()
        assert result == {"version": 1, "plugins": []}

        pm.__class__._market_cache = None
        pm.__class__._market_cache_time = 0

    def test_fetch_market_network_error_returns_empty(self):
        """网络错误应返回默认空 catalog。"""
        pm = PluginManager()
        pm.__class__._market_cache = None
        pm.__class__._market_cache_time = 0

        mock_requests_module = MagicMock()
        mock_requests_module.get.side_effect = ConnectionError("down")

        with patch.dict(sys.modules, {"requests": mock_requests_module}):
            result = pm.fetch_market()

        assert result == {"version": 1, "plugins": []}

    def test_fetch_market_network_error_returns_stale(self):
        """网络错误但有缓存时应返回过期缓存。"""
        pm = PluginManager()
        stale = {"version": 1, "plugins": [{"name": "old"}]}
        pm.__class__._market_cache = stale
        pm.__class__._market_cache_time = time.time() - 600

        mock_requests_module = MagicMock()
        mock_requests_module.get.side_effect = ConnectionError("down")

        with patch.dict(sys.modules, {"requests": mock_requests_module}):
            result = pm.fetch_market()

        assert result == stale

        pm.__class__._market_cache = None
        pm.__class__._market_cache_time = 0


class TestPluginManagerStartPluginPlatformCheck:
    """_start_plugin 平台兼容性检查。"""

    @patch("services.plugin_manager.sys")
    def test_start_plugin_skips_wrong_platform(self, mock_sys):
        """目标平台不匹配时应跳过启动。"""
        pm = PluginManager()
        manifest = _make_manifest(platform="windows", arch="amd64")
        mock_sys.platform = "linux"

        with patch(
            "services.plugin_manager.resolve_entry_path", return_value=Path("/fake/bin")
        ):
            pm._start_plugin(manifest)

        assert manifest.name not in pm._plugins

    def test_start_plugin_allows_matching_platform(self):
        """目标平台匹配时应继续启动流程。"""
        pm = PluginManager()
        manifest = _make_manifest(platform="linux", arch="amd64")

        mock_sock = MagicMock()
        mock_sock.getsockname.return_value = ("127.0.0.1", 50000)
        mock_proc = MagicMock(spec=subprocess.Popen)
        mock_proc.pid = 111
        mock_channel = MagicMock()
        mock_stub = MagicMock()
        health_resp = MagicMock()
        health_resp.healthy = True
        mock_stub.HealthCheck.return_value = health_resp
        mock_info = MagicMock()
        mock_info.name = "p"
        mock_info.version = "0.1"
        mock_info.capabilities = []
        mock_stub.Info.return_value = mock_info

        mock_socket_module = MagicMock()
        mock_socket_module.socket.return_value = mock_sock

        with (
            patch(
                "services.plugin_manager.resolve_entry_path",
                return_value=Path("/fake/bin"),
            ),
            patch("services.plugin_manager.sys") as mock_sys,
            patch("services.plugin_manager.subprocess") as mock_subprocess,
            patch("services.plugin_manager.grpc") as mock_grpc,
            patch("services.plugin_manager.plugin_pb2_grpc") as mock_pb2_grpc,
            patch("services.plugin_manager.plugin_pb2"),
            patch("services.plugin_manager.time") as mock_time,
            patch.dict(sys.modules, {"socket": mock_socket_module}),
            patch("services.plugin_manager.os.environ", {"PATH": "/usr/bin"}),
            patch.object(pm, "_config_store") as mock_cs,
        ):

            mock_sys.platform = "linux"
            mock_subprocess.Popen.return_value = mock_proc
            mock_grpc.insecure_channel.return_value = mock_channel
            mock_pb2_grpc.PluginServiceStub.return_value = mock_stub
            mock_time.time.side_effect = [0, 1]
            mock_cs.get_config.return_value = {}

            pm._start_plugin(manifest)

        assert manifest.name in pm._plugins
