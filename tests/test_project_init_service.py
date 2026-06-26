# coding: utf-8
"""
ProjectInitService 单元测试
"""

import subprocess
from unittest.mock import patch

import pytest

from services.project_init_service import ProjectInitError, ProjectInitService


@pytest.fixture
def service(tmp_path):
    """使用临时目录作为 hugo-admin 安装目录的服务实例。"""
    admin_root = tmp_path / "admin"
    admin_root.mkdir()
    return ProjectInitService(admin_root)


def test_validate_rejects_relative_path(service, tmp_path):
    """相对路径应被拒绝。"""
    with pytest.raises(ProjectInitError, match="必须是绝对路径"):
        service.validate_target_path("relative/path")


def test_validate_rejects_path_inside_admin_dir(service):
    """hugo-admin 安装目录内部的路径应被拒绝。"""
    inside = service.admin_root / "site"
    with pytest.raises(ProjectInitError, match="不能位于 hugo-admin 安装目录内"):
        service.validate_target_path(inside)


def test_validate_rejects_existing_hugo_config(service, tmp_path):
    """已包含 Hugo 配置文件的路径应被拒绝。"""
    site = tmp_path / "existing-site-with-config"
    site.mkdir()
    (site / "hugo.toml").write_text("baseURL = '/'")

    with pytest.raises(ProjectInitError, match="已包含 Hugo 配置文件"):
        service.validate_target_path(site)


def test_validate_rejects_existing_config_default(service, tmp_path):
    """config/_default/config.toml 存在时应被拒绝。"""
    site = tmp_path / "existing-site"
    (site / "config" / "_default").mkdir(parents=True)
    (site / "config" / "_default" / "config.toml").write_text("baseURL = '/'")

    with pytest.raises(ProjectInitError, match="已包含 Hugo 站点配置"):
        service.validate_target_path(site)


def test_validate_accepts_empty_directory(service, tmp_path):
    """空目录应通过校验。"""
    site = tmp_path / "empty-site"
    site.mkdir()

    path = service.validate_target_path(site)
    assert path == site.resolve()


def test_validate_accepts_nonexistent_path(service, tmp_path):
    """不存在的路径应通过校验。"""
    site = tmp_path / "new-site"

    path = service.validate_target_path(site)
    assert path == site.resolve()


# -------- 默认主题常量 --------


def test_default_theme_constants():
    """默认主题指向 svtter/Fried-Rice 仓库。"""
    assert ProjectInitService.DEFAULT_THEME_NAME == "Fried-Rice"
    assert "svtter/Fried-Rice" in ProjectInitService.DEFAULT_THEME_REPO


# -------- _install_default_theme --------


def _write_fake_theme(target_dir, name="Fried-Rice"):
    """在 themes/<name> 写入最小可识别内容，模拟已安装主题。"""
    (target_dir / name).mkdir(parents=True, exist_ok=True)
    (target_dir / name / "theme.toml").write_text('name = "Fried-Rice"\n')


def test_install_default_theme_activates_when_already_present(service, tmp_path):
    """主题已存在时只激活，不再次安装。"""
    site = tmp_path / "site"
    site.mkdir()
    _write_fake_theme(site / "themes")

    result = service._install_default_theme(site)

    assert result["name"] == "Fried-Rice"
    assert result["installed"] is False
    assert result["activated"] is True
    assert result["error"] is None
    # 激活后 .admin/settings.json 应包含 theme.name
    import json

    settings = json.loads((site / ".admin" / "settings.json").read_text())
    assert settings["theme"]["name"] == "Fried-Rice"


def test_install_default_theme_calls_install_when_missing(service, tmp_path):
    """主题不存在时调用 ThemeService.install_theme 并激活。"""
    site = tmp_path / "site"
    site.mkdir()
    # themes/ 目录不存在
    with (
        patch(
            "services.theme_service.ThemeService.install_theme",
            return_value={"name": "Fried-Rice", "mode": "copy"},
        ) as mock_install,
        patch(
            "services.theme_service.ThemeService.activate_theme",
            return_value={"name": "Fried-Rice", "active": True},
        ) as mock_activate,
    ):
        result = service._install_default_theme(site)

    assert result["installed"] is True
    assert result["activated"] is True
    assert result["error"] is None
    mock_install.assert_called_once()
    args, kwargs = mock_install.call_args
    assert args[0] == ProjectInitService.DEFAULT_THEME_REPO
    assert args[1] == ProjectInitService.DEFAULT_THEME_NAME
    assert kwargs.get("mode") == "copy"
    mock_activate.assert_called_once()


def test_install_default_theme_swallows_install_failure(service, tmp_path):
    """install 抛 ThemeError 时不抛出，返回 error 字段。"""
    from services.theme_service import ThemeError

    site = tmp_path / "site"
    site.mkdir()
    with patch(
        "services.theme_service.ThemeService.install_theme",
        side_effect=ThemeError("clone timeout"),
    ):
        result = service._install_default_theme(site)

    assert result["installed"] is False
    assert result["activated"] is False
    assert "install_failed" in (result["error"] or "")


def test_install_default_theme_swallows_unexpected_errors(service, tmp_path, caplog):
    """未捕获异常被吞掉，初始化流程不中断。"""
    site = tmp_path / "site"
    site.mkdir()
    with patch(
        "services.project_init_service.SettingsService.get_settings",
        side_effect=TypeError("settings signature mismatch"),
    ):
        with caplog.at_level("WARNING"):
            result = service._install_default_theme(site)

    assert result["installed"] is False
    assert result["activated"] is False
    assert "unexpected" in (result["error"] or "")
    # 默认主题失败不会让初始化失败


# -------- create_site 集成（hugo new site + 默认主题） --------


def test_create_site_sets_default_theme_in_config(service, tmp_path, monkeypatch):
    """create_site 写入的 hugo.toml 应包含默认主题字段。"""
    site = tmp_path / "new-blog"
    # 跳过真实主题安装（无网络/CI 环境）
    monkeypatch.setattr(
        ProjectInitService,
        "_install_default_theme",
        lambda self, root: {
            "name": "Fried-Rice",
            "repo": ProjectInitService.DEFAULT_THEME_REPO,
            "installed": False,
            "activated": False,
            "error": "skipped",
        },
    )

    if not subprocess.run(["which", "hugo"], capture_output=True).returncode == 0:
        pytest.skip("hugo CLI not available")

    result = service.create_site(site, config_format="toml")

    assert (site / "hugo.toml").exists()
    config = (site / "hugo.toml").read_text()
    assert 'theme = "Fried-Rice"' in config
    assert result["default_theme"]["name"] == "Fried-Rice"
