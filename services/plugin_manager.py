# coding: utf-8
"""
Plugin Manager — discovers, starts, stops, and manages plugin processes.

Each plugin runs as an independent subprocess communicating over gRPC.
The manager handles:
  - Discovery: scan ~/.hugo-admin/plugins/ for valid plugin.toml manifests
  - Lifecycle: start/stop subprocess, health-check handshake, graceful shutdown
  - gRPC client: connect to plugin, call Info/HealthCheck/SetConfig
  - Config persistence: encrypted storage of sensitive plugin config values
  - Route registration: register Flask proxy routes for plugin capabilities
"""

import json
import logging
import os
import signal
import subprocess
import sys
import threading
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Optional

import grpc

from proto import plugin_pb2, plugin_pb2_grpc
from services.plugin_manifest import (
    ManifestError,
    PluginManifest,
    parse_manifest,
    resolve_entry_path,
)

logger = logging.getLogger(__name__)

# Directory for plugin storage
PLUGIN_BASE_DIR = Path.home() / ".hugo-admin"
PLUGIN_DIR = PLUGIN_BASE_DIR / "plugins"
CONFIG_FILE = PLUGIN_BASE_DIR / "plugin-config.json"
SECRET_KEY_FILE = PLUGIN_BASE_DIR / ".secret_key"

# gRPC handshake timeout
HANDSHAKE_TIMEOUT_SECONDS = 10

# Graceful shutdown timeout
SHUTDOWN_TIMEOUT_SECONDS = 5


@dataclass
class PluginState:
    """Runtime state for a loaded plugin."""

    manifest: PluginManifest
    process: Optional[subprocess.Popen] = None
    port: int = 0
    channel: Optional[grpc.Channel] = None
    stub: Optional[plugin_pb2_grpc.PluginServiceStub] = None
    enabled: bool = False
    status: str = "stopped"  # stopped | running | error


class PluginConfigStore:
    """Persist plugin configuration with Fernet encryption for sensitive values."""

    def __init__(self, config_path: Path = CONFIG_FILE):
        self._config_path = config_path
        self._lock = threading.Lock()
        self._fernet = None
        self._data: dict[str, dict[str, Any]] = {}
        self._load()

    def _get_fernet(self):
        """Lazy-init Fernet cipher from machine secret key."""
        if self._fernet is not None:
            return self._fernet

        from cryptography.fernet import Fernet

        SECRET_KEY_FILE.parent.mkdir(parents=True, exist_ok=True)

        if SECRET_KEY_FILE.exists():
            key = SECRET_KEY_FILE.read_bytes().strip()
        else:
            key = Fernet.generate_key()
            SECRET_KEY_FILE.write_bytes(key)
            os.chmod(SECRET_KEY_FILE, 0o600)

        os.chmod(SECRET_KEY_FILE, 0o600)
        self._fernet = Fernet(key)
        return self._fernet

    def _load(self):
        """Load config from disk."""
        if self._config_path.exists():
            try:
                self._data = json.loads(self._config_path.read_text(encoding="utf-8"))
            except (json.JSONDecodeError, OSError) as e:
                logger.warning("Failed to load plugin config: %s", e)
                self._data = {}
        else:
            self._data = {}

    def _save(self):
        """Atomic write config to disk."""
        tmp = self._config_path.with_suffix(".tmp")
        tmp.write_text(json.dumps(self._data, indent=2, ensure_ascii=False))
        tmp.replace(self._config_path)

    def get_config(self, plugin_name: str) -> dict[str, Any]:
        """Get decrypted config for a plugin."""
        with self._lock:
            raw = self._data.get(plugin_name, {})
            return self._decrypt_values(raw)

    def set_config(self, plugin_name: str, config: dict[str, Any]):
        """Set and encrypt config for a plugin."""
        with self._lock:
            self._data[plugin_name] = self._encrypt_values(config)
            self._save()

    def _encrypt_values(self, config: dict[str, Any]) -> dict[str, Any]:
        """Encrypt values marked with '_encrypted' prefix (internal marker).

        In v1 we encrypt all string values. The manifest's config_schema
        can mark fields as sensitive — for simplicity we encrypt all string
        values to avoid leaking credentials by omission.
        """
        fernet = self._get_fernet()
        result = {}
        for k, v in config.items():
            if isinstance(v, str) and v:
                encrypted = fernet.encrypt(v.encode("utf-8"))
                result[k] = f"_enc:{encrypted.decode('ascii')}"
            else:
                result[k] = v
        return result

    def _decrypt_values(self, config: dict[str, Any]) -> dict[str, Any]:
        """Decrypt values with '_enc:' prefix."""
        fernet = self._get_fernet()
        result = {}
        for k, v in config.items():
            if isinstance(v, str) and v.startswith("_enc:"):
                try:
                    decrypted = fernet.decrypt(v[5:].encode("ascii"))
                    result[k] = decrypted.decode("utf-8")
                except Exception:
                    logger.warning("Failed to decrypt config value for key '%s'", k)
                    result[k] = ""
            else:
                result[k] = v
        return result


class PluginManager:
    """Manages plugin lifecycle: discover, start, stop, configure."""

    def __init__(self):
        self._plugins: dict[str, PluginState] = {}
        self._config_store = PluginConfigStore()
        self._routes_registered = False

    # ---- Discovery ----

    def discover_plugins(self) -> list[PluginManifest]:
        """Scan ~/.hugo-admin/plugins/ and parse valid manifests.

        Returns list of successfully parsed manifests. Invalid directories
        are skipped with a warning log.
        """
        PLUGIN_DIR.mkdir(parents=True, exist_ok=True)
        manifests: list[PluginManifest] = []

        for child in sorted(PLUGIN_DIR.iterdir()):
            if not child.is_dir():
                continue

            toml_path = child / "plugin.toml"
            if not toml_path.exists():
                continue

            try:
                manifest = parse_manifest(child)
                manifests.append(manifest)
                logger.info(
                    "Discovered plugin: %s v%s (%s)",
                    manifest.name,
                    manifest.version,
                    manifest.capabilities,
                )
            except ManifestError as e:
                logger.warning("Skipping invalid plugin in %s: %s", child, e)

        return manifests

    # ---- Lifecycle ----

    def start_all(self):
        """Discover and start all plugins."""
        manifests = self.discover_plugins()
        for manifest in manifests:
            try:
                self._start_plugin(manifest)
            except Exception as e:
                logger.error("Failed to start plugin %s: %s", manifest.name, e)

    def _start_plugin(self, manifest: PluginManifest):
        """Start a single plugin subprocess and perform gRPC handshake."""
        if manifest.name in self._plugins and self._plugins[manifest.name].enabled:
            logger.warning("Plugin %s is already running", manifest.name)
            return

        # Resolve and validate entry path (Decision 9)
        entry_path = resolve_entry_path(manifest)

        # Check platform/arch compatibility
        if manifest.platform and manifest.arch:
            current_platform = sys.platform
            # Map Python platform to manifest values
            plat_map = {"linux": "linux", "darwin": "darwin", "win32": "windows"}
            expected = plat_map.get(current_platform, current_platform)
            if manifest.platform != expected:
                logger.warning(
                    "Plugin %s built for '%s', running on '%s' — skipping",
                    manifest.name,
                    manifest.platform,
                    expected,
                )
                return

        # Assign dynamic port
        import socket

        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.bind(("127.0.0.1", 0))
        port = sock.getsockname()[1]
        sock.close()

        # Start subprocess
        env = os.environ.copy()
        process = subprocess.Popen(
            [str(entry_path), "--port", str(port)],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
        )

        # Create gRPC channel
        channel = grpc.insecure_channel(f"127.0.0.1:{port}")

        # Wait for health check with timeout
        stub = plugin_pb2_grpc.PluginServiceStub(channel)
        deadline = time.time() + HANDSHAKE_TIMEOUT_SECONDS

        healthy = False
        while time.time() < deadline:
            try:
                resp = stub.HealthCheck(plugin_pb2.Empty(), timeout=1)
                if resp.healthy:
                    healthy = True
                    break
            except grpc.RpcError:
                time.sleep(0.2)

        if not healthy:
            process.kill()
            channel.close()
            raise RuntimeError(
                f"Plugin {manifest.name} failed health check within "
                f"{HANDSHAKE_TIMEOUT_SECONDS}s"
            )

        # Get plugin info
        info = stub.Info(plugin_pb2.Empty())

        # Restore config if exists
        saved_config = self._config_store.get_config(manifest.name)
        if saved_config:
            try:
                config_json = json.dumps(saved_config)
                stub.SetConfig(plugin_pb2.SetConfigRequest(config_json=config_json))
            except grpc.RpcError as e:
                logger.warning("Failed to push config to %s: %s", manifest.name, e)

        state = PluginState(
            manifest=manifest,
            process=process,
            port=port,
            channel=channel,
            stub=stub,
            enabled=True,
            status="running",
        )
        self._plugins[manifest.name] = state
        logger.info(
            "Plugin %s v%s started on port %d (capabilities: %s)",
            info.name,
            info.version,
            port,
            list(info.capabilities),
        )

    def stop_all(self):
        """Stop all running plugin processes gracefully."""
        for name, state in self._plugins.items():
            if state.enabled and state.process:
                self._stop_plugin(name, state)

    def _stop_plugin(self, name: str, state: PluginState):
        """Stop a single plugin: SIGTERM → wait → SIGKILL."""
        process = state.process
        if process is None or process.poll() is not None:
            return

        logger.info("Stopping plugin %s (pid %d)", name, process.pid)
        process.send_signal(signal.SIGTERM)

        try:
            process.wait(timeout=SHUTDOWN_TIMEOUT_SECONDS)
        except subprocess.TimeoutExpired:
            logger.warning("Plugin %s did not exit, sending SIGKILL", name)
            process.kill()
            process.wait()

        if state.channel:
            state.channel.close()

        state.enabled = False
        state.status = "stopped"
        state.process = None
        state.channel = None
        state.stub = None

    def enable_plugin(self, name: str) -> bool:
        """Enable a stopped plugin."""
        state = self._plugins.get(name)
        if state is None:
            return False
        if state.enabled:
            return True
        try:
            self._start_plugin(state.manifest)
            return True
        except Exception as e:
            logger.error("Failed to enable plugin %s: %s", name, e)
            return False

    def disable_plugin(self, name: str) -> bool:
        """Disable a running plugin (stop process, keep config)."""
        state = self._plugins.get(name)
        if state is None:
            return False
        if not state.enabled:
            return True
        self._stop_plugin(name, state)
        return True

    # ---- gRPC Client ----

    def get_stub(self, name: str) -> Optional[plugin_pb2_grpc.PluginServiceStub]:
        """Get the gRPC stub for a running plugin."""
        state = self._plugins.get(name)
        if state and state.enabled and state.stub:
            return state.stub
        return None

    def get_image_uploader_stub(
        self, name: str
    ) -> Optional[plugin_pb2_grpc.ImageUploaderStub]:
        """Get the ImageUploader gRPC stub for a running plugin."""
        state = self._plugins.get(name)
        if state and state.enabled and state.channel:
            return plugin_pb2_grpc.ImageUploaderStub(state.channel)
        return None

    def get_tts_generator_stub(
        self, name: str
    ) -> Optional[plugin_pb2_grpc.TTSGeneratorStub]:
        """Get the TTSGenerator gRPC stub for a running plugin."""
        state = self._plugins.get(name)
        if state and state.enabled and state.channel:
            return plugin_pb2_grpc.TTSGeneratorStub(state.channel)
        return None

    def find_plugin_with_capability(
        self, capability: str
    ) -> Optional[dict[str, Any]]:
        """Return the first running+enabled plugin info declaring a capability.

        Used by article-integration layers (image upload, TTS, ...) to locate
        the active plugin for a capability without repeating the scan loop.
        Returns the plugin info dict (as produced by ``list_plugins``) or None.
        """
        for info in self.list_plugins():
            if (
                info.get("status") == "running"
                and info.get("enabled")
                and capability in info.get("capabilities", [])
            ):
                return info
        return None

    # ---- Config ----

    def get_plugin_config(self, name: str) -> dict[str, Any]:
        """Get decrypted config for a plugin."""
        return self._config_store.get_config(name)

    def set_plugin_config(self, name: str, config: dict[str, Any]) -> bool:
        """Set config for a plugin and push to running instance."""
        self._config_store.set_config(name, config)

        stub = self.get_stub(name)
        if stub:
            try:
                config_json = json.dumps(config)
                resp = stub.SetConfig(
                    plugin_pb2.SetConfigRequest(config_json=config_json)
                )
                return resp.success
            except grpc.RpcError as e:
                logger.error("Failed to push config to %s: %s", name, e)
                return False
        return True  # saved but plugin not running

    def get_config_schema(self, name: str) -> dict[str, Any]:
        """Get config schema. Priority: gRPC GetConfigSchema > manifest."""
        stub = self.get_stub(name)
        if stub:
            try:
                resp = stub.GetConfigSchema(plugin_pb2.Empty())
                if resp.schema_json:
                    import json

                    return json.loads(resp.schema_json)
            except grpc.RpcError:
                pass

        state = self._plugins.get(name)
        if state:
            return state.manifest.config_schema
        return {}

    # ---- Query ----

    def list_plugins(self) -> list[dict[str, Any]]:
        """List all known plugins with status."""
        result = []
        for name, state in self._plugins.items():
            result.append(
                {
                    "name": name,
                    "version": state.manifest.version,
                    "description": state.manifest.description,
                    "author": state.manifest.author,
                    "capabilities": state.manifest.capabilities,
                    "status": state.status,
                    "enabled": state.enabled,
                    "has_config": bool(self._config_store.get_config(name)),
                }
            )
        return result

    def get_plugin(self, name: str) -> Optional[PluginState]:
        """Get runtime state for a specific plugin."""
        return self._plugins.get(name)

    # ---- Market ----

    MARKET_URL = "https://plugins.hugo-admin.dev/manifest.json"
    _market_cache: Optional[dict] = None
    _market_cache_time: float = 0
    MARKET_CACHE_TTL = 300  # 5 minutes

    def fetch_market(self) -> dict[str, Any]:
        """Fetch plugin market catalog with TTL cache."""
        now = time.time()
        if (
            self._market_cache is not None
            and now - self._market_cache_time < self.MARKET_CACHE_TTL
        ):
            return self._market_cache

        import requests

        try:
            resp = requests.get(self.MARKET_URL, timeout=10)
            resp.raise_for_status()
            data = resp.json()
            self.__class__._market_cache = data
            self.__class__._market_cache_time = now
            return data
        except Exception as e:
            logger.error("Failed to fetch plugin market: %s", e)
            # Return stale cache if available
            if self._market_cache is not None:
                return self._market_cache
            return {"version": 1, "plugins": []}
