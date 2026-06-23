# coding: utf-8
"""
ThemeService 单元测试
"""

import pytest

from services.settings_service import SettingsService
from services.theme_service import ThemeError, ThemeService


@pytest.fixture
def settings_service(tmp_path):
    """临时设置服务。"""
    return SettingsService(tmp_path / ".admin" / "settings.json")


@pytest.fixture
def service(tmp_path, settings_service):
    """使用临时 Hugo 根目录的主题服务实例。"""
    return ThemeService(tmp_path, settings_service=settings_service)


def test_list_themes_empty(service):
    """themes/ 目录不存在时返回空列表。"""
    assert service.list_themes() == []


def test_list_themes_detects_regular_directory(service, tmp_path):
    """普通主题目录应被列出，且不是子模块。"""
    (tmp_path / "themes" / "mytheme").mkdir(parents=True)
    themes = service.list_themes()
    assert len(themes) == 1
    assert themes[0]["name"] == "mytheme"
    assert themes[0]["is_submodule"] is False


def test_list_themes_detects_submodule(service, tmp_path):
    """子模块主题应被正确标记。"""
    (tmp_path / "themes" / "subtheme").mkdir(parents=True)
    (tmp_path / ".gitmodules").write_text(
        '[submodule "themes/subtheme"]\n'
        "\tpath = themes/subtheme\n"
        "\turl = https://example.com/theme.git\n",
        encoding="utf-8",
    )
    themes = service.list_themes()
    names = {t["name"]: t["is_submodule"] for t in themes}
    assert names.get("subtheme") is True


def test_activate_theme_persists_name(service, settings_service):
    """激活主题应将名称持久化到设置文件。"""
    (service.hugo_root / "themes" / "activetheme").mkdir(parents=True)

    result = service.activate_theme("activetheme")
    assert result["active"] is True

    settings = settings_service.get_settings()
    assert settings["theme"]["name"] == "activetheme"


def test_activate_missing_theme_raises(service):
    """激活不存在的主题应报错。"""
    with pytest.raises(ThemeError, match="主题不存在"):
        service.activate_theme("notfound")


def test_get_active_theme_returns_none_by_default(service):
    """默认无活跃主题。"""
    assert service.get_active_theme() is None


def test_get_active_theme_after_activation(service):
    """激活后应返回主题名称。"""
    (service.hugo_root / "themes" / "foo").mkdir(parents=True)
    service.activate_theme("foo")
    assert service.get_active_theme() == "foo"


def test_install_theme_rejects_existing_directory(service, tmp_path):
    """主题目录已存在时应拒绝安装。"""
    (tmp_path / "themes" / "exists").mkdir(parents=True)
    with pytest.raises(ThemeError, match="主题目录已存在"):
        service.install_theme("https://example.com/t.git", "exists")


def test_install_theme_rejects_invalid_name(service):
    """包含路径分隔符的主题名应被拒绝。"""
    with pytest.raises(ThemeError, match="不能包含路径分隔符"):
        service.install_theme("https://example.com/t.git", "foo/bar")
