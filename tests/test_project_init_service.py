# coding: utf-8
"""
ProjectInitService 单元测试
"""

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
