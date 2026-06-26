# coding: utf-8
"""
ActiveProjectRegistry 单元测试
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from services.active_project import ActiveProjectRegistry


def test_record_and_load_path(tmp_path):
    """写入并读取路径。"""
    site = tmp_path / "blog"
    site.mkdir()
    reg = ActiveProjectRegistry(tmp_path / "data" / "active_project.txt")

    reg.record_path(site)

    loaded = reg.load_path()
    assert loaded is not None
    assert loaded.resolve() == site.resolve()
    assert reg.file_path.exists()


def test_load_returns_none_when_missing(tmp_path):
    """文件不存在时返回 None。"""
    reg = ActiveProjectRegistry(tmp_path / "active_project.txt")
    assert reg.load_path() is None


def test_load_returns_none_when_empty_file(tmp_path):
    """空文件返回 None。"""
    file = tmp_path / "active_project.txt"
    file.write_text("\n", encoding="utf-8")
    reg = ActiveProjectRegistry(file)
    assert reg.load_path() is None


def test_load_returns_none_when_path_invalid(tmp_path):
    """路径不存在时返回 None（不抛错）。"""
    file = tmp_path / "active_project.txt"
    file.write_text(str(tmp_path / "does-not-exist"), encoding="utf-8")
    reg = ActiveProjectRegistry(file)
    assert reg.load_path() is None


def test_record_overwrites_previous(tmp_path):
    """多次写入只保留最新路径。"""
    site1 = tmp_path / "blog1"
    site2 = tmp_path / "blog2"
    site1.mkdir()
    site2.mkdir()
    reg = ActiveProjectRegistry(tmp_path / "active_project.txt")

    reg.record_path(site1)
    reg.record_path(site2)

    loaded = reg.load_path()
    assert loaded.resolve() == site2.resolve()


def test_clear_removes_file(tmp_path):
    """clear() 移除文件。"""
    site = tmp_path / "blog"
    site.mkdir()
    reg = ActiveProjectRegistry(tmp_path / "active_project.txt")
    reg.record_path(site)
    assert reg.file_path.exists()

    reg.clear()
    assert not reg.file_path.exists()
    assert reg.load_path() is None


def test_clear_silent_when_missing(tmp_path):
    """文件不存在时 clear 不抛错。"""
    reg = ActiveProjectRegistry(tmp_path / "active_project.txt")
    reg.clear()
    assert not reg.file_path.exists()


def test_record_resolves_relative_to_cwd(tmp_path, monkeypatch):
    """相对路径会被 resolve 为绝对路径。"""
    monkeypatch.chdir(tmp_path)
    rel = Path("blog")
    rel.mkdir()
    reg = ActiveProjectRegistry(tmp_path / "active_project.txt")

    reg.record_path(rel)

    loaded = reg.load_path()
    assert loaded is not None
    assert loaded.is_absolute()
    assert loaded.resolve() == (tmp_path / "blog").resolve()
