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


@pytest.fixture
def hugo_site_dir(tmp_path, monkeypatch):
    """创建一个使用 config/_default/ 多文件结构的临时 Hugo 站点。"""
    site = tmp_path / "test-blog-dir"
    site.mkdir()
    config_dir = site / "config" / "_default"
    config_dir.mkdir(parents=True)
    (config_dir / "config.toml").write_text(
        'baseURL = "https://example.org/"\ntitle = "Test"\n',
        encoding="utf-8",
    )
    (config_dir / "params.toml").write_text(
        'author = "Test Author"\n',
        encoding="utf-8",
    )
    (config_dir / "menu.toml").write_text(
        '[[main]]\nname = "Home"\nurl = "/"\n',
        encoding="utf-8",
    )
    monkeypatch.setitem(app_module.app.config, "HUGO_ROOT", site)
    return site


# ──────────────── GET /api/config ────────────────


def test_list_configs_requires_auth(client):
    """未登录应返回 401。"""
    resp = client.get("/api/config")
    assert resp.status_code == 401


def test_list_configs_returns_root_file(admin_client, hugo_site):
    """根目录单文件模式 → 返回单个文件。"""
    resp = admin_client.get("/api/config")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["mode"] == "root"
    assert len(data["files"]) == 1
    assert data["files"][0]["name"] == "hugo.toml"


def test_list_configs_returns_dir_files(admin_client, hugo_site_dir):
    """config/_default/ 模式 → 返回多个文件。"""
    resp = admin_client.get("/api/config")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["mode"] == "dir"
    names = [f["name"] for f in data["files"]]
    assert "config.toml" in names
    assert "params.toml" in names
    assert "menu.toml" in names


def test_list_configs_returns_404_when_no_config(admin_client, tmp_path, monkeypatch):
    """站点目录存在但没有配置文件 → 404。"""
    empty_site = tmp_path / "empty-blog"
    empty_site.mkdir()
    monkeypatch.setitem(app_module.app.config, "HUGO_ROOT", empty_site)

    resp = admin_client.get("/api/config")
    assert resp.status_code == 404


# ──────────────── GET /api/config/<filename> ────────────────


def test_get_config_file_returns_content(admin_client, hugo_site):
    """读取指定配置文件 → 返回内容。"""
    resp = admin_client.get("/api/config/hugo.toml")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert data["format"] == "toml"
    assert 'baseURL = "https://example.org/"' in data["content"]


def test_get_config_file_from_dir(admin_client, hugo_site_dir):
    """从 config/_default/ 读取子配置文件。"""
    resp = admin_client.get("/api/config/params.toml")
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True
    assert 'author = "Test Author"' in data["content"]


def test_get_config_file_returns_404(admin_client, hugo_site):
    """不存在的文件名 → 404。"""
    resp = admin_client.get("/api/config/nonexistent.toml")
    assert resp.status_code == 404


# ──────────────── PUT /api/config/<filename> ────────────────


def test_put_config_saves_valid_toml(admin_client, hugo_site):
    """写入合法 TOML → 保存成功。"""
    new_content = 'baseURL = "https://new-blog.com/"\ntitle = "New"\n'
    resp = admin_client.put("/api/config/hugo.toml", json={"content": new_content})
    assert resp.status_code == 200
    data = resp.get_json()
    assert data["success"] is True

    saved = (hugo_site / "hugo.toml").read_text(encoding="utf-8")
    assert 'baseURL = "https://new-blog.com/"' in saved


def test_put_config_rejects_invalid_toml(admin_client, hugo_site):
    """写入非法 TOML → 400。"""
    resp = admin_client.put(
        "/api/config/hugo.toml",
        json={"content": "this is not = valid = toml ["},
    )
    assert resp.status_code == 400
    data = resp.get_json()
    assert data["success"] is False
    assert "TOML" in data["message"]


def test_put_config_rejects_empty_content(admin_client, hugo_site):
    """空内容 → 400。"""
    resp = admin_client.put("/api/config/hugo.toml", json={"content": ""})
    assert resp.status_code == 400


def test_put_config_requires_auth(client):
    """未登录应返回 401。"""
    resp = client.put("/api/config/hugo.toml", json={"content": 'title = "x"'})
    assert resp.status_code == 401


def test_put_config_saves_to_dir(admin_client, hugo_site_dir):
    """写入 config/_default/ 下的子文件。"""
    new_content = 'author = "New Author"\n'
    resp = admin_client.put("/api/config/params.toml", json={"content": new_content})
    assert resp.status_code == 200

    saved = (hugo_site_dir / "config" / "_default" / "params.toml").read_text(
        encoding="utf-8"
    )
    assert 'author = "New Author"' in saved
