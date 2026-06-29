# coding: utf-8
"""
HugoServerManager 端口就绪检查 & 进程退出日志测试
"""

import socket
import threading
import time
from unittest.mock import MagicMock, patch

import pytest

from services.hugo_service import HugoServerManager


@pytest.fixture
def manager(tmp_path):
    """带有默认 server_url 的 HugoServerManager。"""
    return HugoServerManager(
        tmp_path,
        server_url="http://0.0.0.0:1313",
    )


# ── _check_port_listening ──────────────────────────────────────────


def test_port_listening_returns_true_when_port_open(manager):
    """端口被监听时 _check_port_listening 返回 True。"""
    with patch("services.hugo_service.socket.create_connection") as mock_conn:
        mock_conn.return_value.__enter__ = MagicMock()
        mock_conn.return_value.__exit__ = MagicMock(return_value=False)
        manager.is_running = True
        assert manager._check_port_listening() is True


def test_port_listening_returns_false_when_connection_refused(manager):
    """端口未被监听时 _check_port_listening 返回 False。"""
    with patch(
        "services.hugo_service.socket.create_connection",
        side_effect=ConnectionRefusedError,
    ):
        manager.is_running = True
        assert manager._check_port_listening() is False


def test_port_listening_returns_false_when_not_running(manager):
    """进程未标记为运行时 _check_port_listening 直接返回 False。"""
    manager.is_running = False
    assert manager._check_port_listening() is False


def test_port_listening_returns_false_on_timeout(manager):
    """连接超时时 _check_port_listening 返回 False。"""
    with patch(
        "services.hugo_service.socket.create_connection",
        side_effect=socket.timeout,
    ):
        manager.is_running = True
        assert manager._check_port_listening() is False


# ── get_status ready field ─────────────────────────────────────────


def test_get_status_ready_true_when_port_open(manager):
    """进程存活且端口监听时 get_status 返回 ready=True。"""
    with patch.object(manager, "_check_process_alive", return_value=True), patch.object(
        manager, "_check_port_listening", return_value=True
    ):
        manager.is_running = True
        manager.pid = 12345
        status = manager.get_status()
        assert status["running"] is True
        assert status["ready"] is True


def test_get_status_ready_false_when_port_not_open(manager):
    """进程存活但端口未监听时 get_status 返回 ready=False（正在构建）。"""
    with patch.object(manager, "_check_process_alive", return_value=True), patch.object(
        manager, "_check_port_listening", return_value=False
    ):
        manager.is_running = True
        manager.pid = 12345
        status = manager.get_status()
        assert status["running"] is True
        assert status["ready"] is False


def test_get_status_ready_false_when_not_running(manager):
    """进程未运行时 get_status 返回 running=False, ready=False。"""
    status = manager.get_status()
    assert status["running"] is False
    assert status["ready"] is False


def test_get_status_clears_dead_process(manager):
    """进程已退出时 get_status 清理状态。"""
    mock_process = MagicMock()
    mock_process.poll.return_value = 1  # exited
    manager.process = mock_process
    manager.pid = 12345
    manager.is_running = True

    status = manager.get_status()
    assert status["running"] is False
    assert status["ready"] is False
    assert manager.process is None
    assert manager.pid is None


# ── _monitor_logs exit logging ─────────────────────────────────────


def test_monitor_logs_logs_exit_when_process_dies_naturally(manager):
    """进程自然退出时 _monitor_logs 记录退出日志。"""
    mock_process = MagicMock()
    # readline returns "" immediately (process stdout closed = process exited)
    mock_process.stdout.readline.return_value = ""
    mock_process.poll.return_value = 1
    manager.process = mock_process
    manager.is_running = True
    manager.stop_log_thread = False

    manager._monitor_logs()

    exit_logs = [l for l in manager.logs if "已退出" in l["message"]]
    assert len(exit_logs) == 1
    assert exit_logs[0]["level"] == "WARNING"
    assert "退出码: 1" in exit_logs[0]["message"]


def test_monitor_logs_no_exit_log_when_stopped_by_stop(manager):
    """stop() 触发的退出不记录"已退出"日志（stop() 自己会记录）。"""
    mock_process = MagicMock()
    mock_process.stdout.readline.return_value = ""
    manager.process = mock_process
    manager.is_running = True
    manager.stop_log_thread = True  # set by stop()

    manager._monitor_logs()

    exit_logs = [l for l in manager.logs if "已退出" in l["message"]]
    assert len(exit_logs) == 0


def test_monitor_logs_passes_through_lines(manager):
    """_monitor_logs 正常转发 stdout 日志行。"""
    lines = iter(["Building...\n", "Done.\n", ""])
    mock_process = MagicMock()
    mock_process.stdout.readline.side_effect = lambda: next(lines, "")
    mock_process.poll.return_value = None
    manager.process = mock_process
    manager.is_running = True
    manager.stop_log_thread = False

    manager._monitor_logs()

    messages = [l["message"] for l in manager.logs]
    assert "Building..." in messages
    assert "Done." in messages
