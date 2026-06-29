# coding: utf-8
"""
Hugo 服务器管理服务
负责启动、停止和监控 Hugo 开发服务器
"""

import os
import socket
import subprocess
import threading
import time
from datetime import datetime
from pathlib import Path
from urllib.parse import urlparse

import psutil

from config import Config
from services.settings_service import SettingsStorageError, SettingsValidationError


class HugoServerManager:
    """Hugo 服务器管理器"""

    def __init__(
        self, hugo_root, socketio=None, server_url=None, settings_service=None
    ):
        """
        初始化 Hugo 服务器管理器

        Args:
            hugo_root: Hugo 项目根目录
            socketio: SocketIO 实例，用于推送日志
            server_url: Hugo 服务器基础 URL (如 http://0.0.0.0:1313)
            settings_service: 设置服务实例，用于读取持久化的活跃主题。
        """
        self.hugo_root = Path(hugo_root)
        self.socketio = socketio
        self.server_url = server_url or Config.HUGO_SERVER_BASE_URL
        self._server_port = urlparse(self.server_url).port or 1313
        self.settings_service = settings_service
        self.process = None
        self.pid = None
        self.is_running = False
        self.logs = []
        self.max_logs = 1000  # 最多保存 1000 条日志
        self.log_thread = None
        self.stop_log_thread = False
        self._operation_lock = threading.Lock()

    def start(self, debug=False, theme_override=None):
        """
        启动 Hugo 服务器

        Args:
            debug: 是否启用 debug 模式(显示草稿)
            theme_override: 临时覆盖的预览主题名称（仅本次启动有效）

        Returns:
            (success, message): 成功标志和消息
        """
        # 检查是否已经在运行
        if self.is_running and self._check_process_alive():
            return False, "Hugo 服务器已经在运行中"

        try:
            # 构建命令
            cmd = [
                "hugo",
                "server",
                "--bind=0.0.0.0",
                "-b",
                self.server_url,
                "--disableFastRender",
            ]
            # 管理后台预览服务器默认渲染草稿, 方便查看新建文章
            cmd.append("-D")
            # Docker 部署时禁用了 Hugo Modules, 需要显式指定主题。
            # 环境变量优先级高于持久化设置, 避免破坏现有 Docker 部署。
            theme = (
                theme_override if theme_override else os.environ.get("HUGO_THEME", "")
            )
            if not theme and self.settings_service is not None:
                try:
                    settings = self.settings_service.get_settings()
                    theme = settings.get("theme", {}).get("name", "")
                except (SettingsStorageError, SettingsValidationError):
                    theme = ""
            if theme:
                cmd.extend(["--theme", theme])

            # 启动进程
            self.process = subprocess.Popen(
                cmd,
                cwd=self.hugo_root,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                bufsize=1,
                universal_newlines=True,
            )

            self.pid = self.process.pid
            self.is_running = True

            # 清空旧日志
            self.logs = []
            self._add_log(f"Hugo 服务器已启动 (PID: {self.pid})", level="SUCCESS")

            # 启动日志监控线程
            self.stop_log_thread = False
            self.log_thread = threading.Thread(target=self._monitor_logs, daemon=True)
            self.log_thread.start()

            return True, f"Hugo 服务器已启动 (PID: {self.pid})"

        except FileNotFoundError:
            return False, "未找到 hugo 命令，请确保 Hugo 已安装"
        except Exception as e:
            return False, f"启动失败: {str(e)}"

    def preview_theme(self, theme_name):
        """
        使用指定主题重启 Hugo 服务器进行预览（不持久化主题设置）。

        该方法在内部加锁，保证 stop/start 序列原子执行，避免并发请求导致
        主题状态错乱或服务器停留在停止状态。

        Args:
            theme_name: 要预览的主题名称。

        Returns:
            (success, message): 成功标志和消息。
        """
        with self._operation_lock:
            if self.is_running:
                self.stop()
            return self.start(debug=False, theme_override=theme_name)

    def stop(self):
        """
        停止 Hugo 服务器

        Returns:
            (success, message): 成功标志和消息
        """
        if not self.is_running or not self.process:
            return False, "Hugo 服务器未运行"

        try:
            # 停止日志监控线程
            self.stop_log_thread = True

            # 尝试优雅停止
            if self.process.poll() is None:
                self.process.terminate()
                try:
                    self.process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    # 强制杀死
                    self.process.kill()
                    self.process.wait()

            self._add_log("Hugo 服务器已停止", level="INFO")
            self.is_running = False
            self.process = None
            self.pid = None

            return True, "Hugo 服务器已停止"

        except Exception as e:
            return False, f"停止失败: {str(e)}"

    def get_status(self):
        """
        获取服务器状态

        Returns:
            dict: 服务器状态信息
        """
        # 检查进程是否还活着
        if self.is_running:
            if not self._check_process_alive():
                self.is_running = False
                self.process = None
                self.pid = None

        status = {
            "running": self.is_running,
            "ready": self.is_running and self._check_port_listening(),
            "pid": self.pid,
            "uptime": None,
            "cpu_percent": None,
            "memory_mb": None,
        }

        # 如果正在运行，获取详细信息
        if self.is_running and self.pid:
            try:
                proc = psutil.Process(self.pid)
                status["uptime"] = self._format_uptime(proc.create_time())
                status["cpu_percent"] = proc.cpu_percent(interval=0.1)
                status["memory_mb"] = round(proc.memory_info().rss / 1024 / 1024, 2)
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                pass

        return status

    def get_recent_logs(self, count=100):
        """
        获取最近的日志

        Args:
            count: 获取日志数量

        Returns:
            list: 日志列表
        """
        return self.logs[-count:]

    def _check_process_alive(self):
        """检查进程是否还活着"""
        if not self.process:
            return False

        # 检查进程是否已结束
        if self.process.poll() is not None:
            return False

        # 使用 psutil 检查进程
        try:
            proc = psutil.Process(self.pid)
            return proc.is_running()
        except (psutil.NoSuchProcess, psutil.AccessDenied):
            return False
    def _check_port_listening(self, timeout=0.5):
        """检查 Hugo 服务器端口是否正在监听"""
        if not self.is_running:
            return False
        try:
            with socket.create_connection(
                ("127.0.0.1", self._server_port), timeout=timeout
            ):
                return True
        except (ConnectionRefusedError, OSError, socket.timeout):
            return False

    def _monitor_logs(self):
        """监控进程输出日志(在后台线程运行)"""
        if not self.process:
            return

        try:
            for line in iter(self.process.stdout.readline, ""):
                if self.stop_log_thread:
                    break

                if line:
                    line = line.strip()
                    if line:
                        self._add_log(line)

        except Exception as e:
            self._add_log(f"日志监控异常: {str(e)}", level="ERROR")
        finally:
            # 非 stop() 触发的退出(进程崩溃/OOM) → 记录日志,
            # 状态由 get_status() → _check_process_alive() 统一清理,
            # 不在此处修改 is_running/process/pid 避免与 stop() 竞态。
            if not self.stop_log_thread and self.process:
                exit_code = self.process.poll()
                self._add_log(
                    f"Hugo 服务器已退出 (退出码: {exit_code})",
                    level="WARNING",
                )

    def _add_log(self, message, level="INFO"):
        """
        添加日志

        Args:
            message: 日志消息
            level: 日志级别 (INFO, SUCCESS, WARNING, ERROR)
        """
        log_entry = {
            "timestamp": datetime.now().strftime("%H:%M:%S"),
            "level": level,
            "message": message,
        }

        self.logs.append(log_entry)

        # 限制日志数量
        if len(self.logs) > self.max_logs:
            self.logs = self.logs[-self.max_logs :]

        # 通过 WebSocket 推送日志
        if self.socketio:
            try:
                self.socketio.emit("server_log", log_entry)
            except Exception:
                pass  # 忽略推送失败

    @staticmethod
    def _format_uptime(create_time):
        """格式化运行时间"""
        uptime_seconds = time.time() - create_time
        hours = int(uptime_seconds // 3600)
        minutes = int((uptime_seconds % 3600) // 60)
        seconds = int(uptime_seconds % 60)

        if hours > 0:
            return f"{hours}h {minutes}m {seconds}s"
        elif minutes > 0:
            return f"{minutes}m {seconds}s"
        else:
            return f"{seconds}s"
