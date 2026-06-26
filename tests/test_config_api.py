# coding: utf-8
"""
Hugo 站点配置 API 测试
"""

import pytest

import app as app_module


@pytest.fixture
def client(auth_store):
    """Flask 测试客户端。"""
    app_module.app.config["TESTING"] = True
    with app_module.app.test_client() as c:
        yield c


@pytest.fixture
def admin_client(client, login):
    """已登录管理员客户端。"""
    login(client)
    return client


@pytest.fixture
def hugo_site(tmp_path, monkeypatch):
    """创建一个带 hugo.toml 的临时 Hugo 站点，并把 HUGO_ROOT 指向它。"""
    site = tmp_path / "test-blog"
    site.mkdir()
    config = site / "hugo.toml"
    config.write_text(
        'baseURL = "https://example.org/"\n'
        'languageCode = "zh-CN"\n'
        'title = "Test Blog"\n'
        'theme = "Fried-Rice"\n',
        encoding="utf-8",
    )
    monkeypatch.setitem(app_module.app.config, "HUGO_ROOT", site)
    return site


# ──────────────── GET /api/config ────────────────


def test_get_config_requires_auth(client):
    """未登录应返回 401。"""
    resp = client.get("/api/config")
    assert resp.status_code == 401


def test_get_config_returns_content(admin_client, hugo_site):
    """已登录 + 有配置文件 → 返回配置内容。"""
    resp = admin_client.get("/api/config")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["format"] == "toml"
    assert 'baseURL = "https://example.org/"' in data["content"]
    assert data["path"].endswith("hugo.toml")


def test_get_config_returns_404_when_no_config(admin_client, tmp_path, monkeypatch):
    """站点目录存在但没有配置文件 → 404。"""
    empty_site = tmp_path / "empty-blog"
    empty_site.mkdir()
    monkeypatch.setitem(app_module.app.config, "HUGO_ROOT", empty_site)

    resp = admin_client.get("/api/config")
    assert resp.status_code == 404
    data = resp.get_json()
    assert data["success"] is False
    assert "未找到" in data["message"]


# ──────────────── PUT /api/config ────────────────


def test_put_config_saves_valid_toml(admin_client, hugo_site):
    """写入合法 TOML → 保存成功。"""
    new_content = (
        'baseURL = "https://new-blog.com/"\n'
        'languageCode = "en"\n'
        'title = "New Title"\n'
    )
    resp = admin_client.put(
        "/api/config",
        json={"content": new_content},
    )
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True

    # 验证文件已写入
    saved = (hugo_site / "hugo.toml").read_text(encoding="utf-8")
    assert 'baseURL = "https://new-blog.com/"' in saved


def test_put_config_rejects_invalid_toml(admin_client, hugo_site):
    """写入非法 TOML → 400 + 错误信息。"""
    resp = admin_client.put(
        "/api/config",
        json={"content": "this is not = valid = toml ["},
    )
    assert resp.status_code == 400
    data = resp.get_json()
    assert data["success"] is False
    assert "TOML" in data["message"]


def test_put_config_rejects_empty_content(admin_client, hugo_site):
    """空内容 → 400。"""
    resp = admin_client.put("/api/config", json={"content": ""})
    assert resp.status_code == 400
    data = resp.get_json()
    assert "不能为空" in data["message"]


def test_put_config_requires_auth(client):
    """未登录应返回 401。"""
    resp = client.put("/api/config", json={"content": 'title = "x"'})
    assert resp.status_code == 401


def test_put_config_creates_file_when_missing(admin_client, tmp_path, monkeypatch):
    """站点无配置文件时 PUT → 自动创建 hugo.toml。"""
    new_site = tmp_path / "new-blog"
    new_site.mkdir()
    monkeypatch.setitem(app_module.app.config, "HUGO_ROOT", new_site)

    content = 'baseURL = "https://new.org/"\ntitle = "New"\n'
    resp = admin_client.put("/api/config", json={"content": content})
    assert resp.status_code == 200

    assert (new_site / "hugo.toml").exists()
    saved = (new_site / "hugo.toml").read_text(encoding="utf-8")
    assert 'baseURL = "https://new.org/"' in saved
