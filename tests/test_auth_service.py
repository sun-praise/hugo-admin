# coding: utf-8
"""
AuthService 单元测试：默认账户引导、校验、改密。
"""

import json
from pathlib import Path

import pytest

from services.auth_service import AuthService, AuthStoreError


def _store(tmp_path: Path) -> Path:
    return tmp_path / "auth.json"


def test_bootstrap_default_from_explicit(tmp_path):
    store = _store(tmp_path)
    auth = AuthService(store, default_username="admin", default_password="s3cret")
    data = json.loads(store.read_text(encoding="utf-8"))
    assert data["username"] == "admin"
    # 明文绝不落盘
    assert data["password_hash"] != "s3cret"
    assert "s3cret" not in store.read_text(encoding="utf-8")
    assert auth.verify("admin", "s3cret") is True


def test_bootstrap_default_admin_admin_when_env_unset(tmp_path, monkeypatch):
    monkeypatch.delenv("ADMIN_USERNAME", raising=False)
    monkeypatch.delenv("ADMIN_PASSWORD", raising=False)
    auth = AuthService(_store(tmp_path))
    assert auth.verify("admin", "admin") is True


def test_existing_store_not_overwritten(tmp_path):
    store = _store(tmp_path)
    AuthService(store, default_username="admin", default_password="first")
    # 再次构造（密码不同）不应重置已存在的账户
    AuthService(store, default_username="admin", default_password="second")
    auth = AuthService(store, default_username="admin", default_password="second")
    assert auth.verify("admin", "first") is True
    assert auth.verify("admin", "second") is False


def test_verify_rejects_wrong_credentials(tmp_path):
    auth = AuthService(
        _store(tmp_path), default_username="admin", default_password="pw"
    )
    assert auth.verify("admin", "wrong") is False
    assert auth.verify("other", "pw") is False
    assert auth.verify("", "") is False
    assert auth.verify("admin", "") is False


def test_set_password_rotates(tmp_path):
    store = _store(tmp_path)
    auth = AuthService(store, default_username="admin", default_password="old")
    auth.set_password("admin", "newpw")
    assert auth.verify("admin", "old") is False
    assert auth.verify("admin", "newpw") is True
    # 新明文不落盘
    assert "newpw" not in store.read_text(encoding="utf-8")


def test_set_password_rejects_mismatch_and_empty(tmp_path):
    auth = AuthService(
        _store(tmp_path), default_username="admin", default_password="pw"
    )
    with pytest.raises(ValueError):
        auth.set_password("other", "x")
    with pytest.raises(ValueError):
        auth.set_password("admin", "")


def test_set_password_atomic_on_write_failure(tmp_path, monkeypatch):
    """写盘失败时内存不得漂移到新密码。"""
    auth = AuthService(
        _store(tmp_path), default_username="admin", default_password="old"
    )

    def boom(account):
        raise OSError("disk full")

    monkeypatch.setattr(auth, "_write", boom)
    with pytest.raises(OSError):
        auth.set_password("admin", "newpw")
    # 写盘失败：内存仍为旧密码
    assert auth.verify("admin", "old") is True
    assert auth.verify("admin", "newpw") is False


def test_get_user_returns_public_info(tmp_path):
    auth = AuthService(
        _store(tmp_path), default_username="admin", default_password="pw"
    )
    assert auth.get_user() == {"username": "admin"}


# ---------- fail closed：损坏的凭据文件绝不静默重置账户 ----------


def test_corrupt_json_fails_closed(tmp_path):
    store = _store(tmp_path)
    store.write_text("{not valid json", encoding="utf-8")
    with pytest.raises(AuthStoreError):
        AuthService(store, default_username="admin", default_password="pw")


def test_incomplete_store_fails_closed(tmp_path):
    store = _store(tmp_path)
    store.write_text(json.dumps({"username": "admin"}), encoding="utf-8")
    with pytest.raises(AuthStoreError):
        AuthService(store, default_username="admin", default_password="pw")


def test_unreadable_store_fails_closed(tmp_path):
    store = _store(tmp_path)
    store.write_text("ok-ignore", encoding="utf-8")
    store.chmod(0o000)
    try:
        with pytest.raises(AuthStoreError):
            AuthService(store, default_username="admin", default_password="pw")
    finally:
        store.chmod(0o600)  # 恢复权限，便于 tmp_path 清理


def test_empty_store_bootstraps(tmp_path):
    """空文件视为首次启动，正常引导默认账户（不视为损坏）。"""
    store = _store(tmp_path)
    store.write_text("", encoding="utf-8")
    auth = AuthService(store, default_username="admin", default_password="pw")
    assert auth.verify("admin", "pw") is True
