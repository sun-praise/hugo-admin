# coding: utf-8
"""
HugoServerManager 主题参数测试
"""

import os
from unittest.mock import MagicMock, patch

import pytest

from services.hugo_service import HugoServerManager
from services.settings_service import SettingsService


@pytest.fixture
def settings_service(tmp_path):
    """带有活跃主题设置的临时设置服务。"""
    service = SettingsService(tmp_path / ".admin" / "settings.json")
    service.update_settings({"theme": {"name": "papermod"}})
    return service


def test_start_appends_theme_from_settings(settings_service, tmp_path):
    """持久化活跃主题应被附加到 hugo server 命令。"""
    manager = HugoServerManager(
        tmp_path,
        settings_service=settings_service,
        server_url="http://0.0.0.0:1313",
    )

    with patch("subprocess.Popen") as popen_mock:
        popen_mock.return_value.stdout = MagicMock()
        popen_mock.return_value.pid = 12345
        popen_mock.return_value.poll.return_value = None
        success, _ = manager.start()

    assert success is True
    call_args = popen_mock.call_args
    assert "--theme" in call_args[0][0]
    assert "papermod" in call_args[0][0]


def test_env_theme_overrides_settings(settings_service, tmp_path, monkeypatch):
    """HUGO_THEME 环境变量应覆盖持久化设置。"""
    monkeypatch.setenv("HUGO_THEME", "envtheme")

    manager = HugoServerManager(
        tmp_path,
        settings_service=settings_service,
        server_url="http://0.0.0.0:1313",
    )

    with patch("subprocess.Popen") as popen_mock:
        popen_mock.return_value.stdout = MagicMock()
        popen_mock.return_value.pid = 12345
        popen_mock.return_value.poll.return_value = None
        success, _ = manager.start()

    assert success is True
    cmd = popen_mock.call_args[0][0]
    theme_index = cmd.index("--theme")
    assert cmd[theme_index + 1] == "envtheme"


def test_no_theme_arg_when_unset(tmp_path):
    """未设置主题时不应附加 --theme。"""
    manager = HugoServerManager(
        tmp_path,
        settings_service=None,
        server_url="http://0.0.0.0:1313",
    )

    # 确保环境变量未设置
    env_theme = os.environ.pop("HUGO_THEME", None)
    try:
        with patch("subprocess.Popen") as popen_mock:
            popen_mock.return_value.stdout = MagicMock()
            popen_mock.return_value.pid = 12345
            popen_mock.return_value.poll.return_value = None
            success, _ = manager.start()

        assert success is True
        assert "--theme" not in popen_mock.call_args[0][0]
    finally:
        if env_theme is not None:
            os.environ["HUGO_THEME"] = env_theme
