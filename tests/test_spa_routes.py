# coding: utf-8
"""
SPA 路由测试 - React 迁移后前端路由应返回 index.html
"""

import json
import tempfile
from pathlib import Path

import pytest

import app as app_module


class TestSPARoutes:
    """Test SPA route serving for React migration."""

    @pytest.fixture
    def client(self, login):
        """Create test client with a temp React index.html."""
        with tempfile.NamedTemporaryFile(suffix=".html", delete=False, mode="w") as f:
            f.write("<html><body>React SPA</body></html>")
            temp_path = f.name

        original_react_index = app_module.app.config.get("REACT_INDEX")
        app_module.app.config["REACT_INDEX"] = Path(temp_path)

        original_content_dir = app_module.app.config.get("CONTENT_DIR")
        original_hugo_root = app_module.app.config.get("HUGO_ROOT")

        with tempfile.TemporaryDirectory() as temp_dir:
            content_dir = Path(temp_dir) / "content"
            content_dir.mkdir(parents=True, exist_ok=True)
            app_module.app.config["CONTENT_DIR"] = content_dir
            app_module.app.config["HUGO_ROOT"] = Path(temp_dir)
            app_module.app.config["TESTING"] = True

            with app_module.app.test_client() as client:
                login(client)
                yield client

            app_module.app.config["REACT_INDEX"] = original_react_index
            app_module.app.config["CONTENT_DIR"] = original_content_dir
            app_module.app.config["HUGO_ROOT"] = original_hugo_root

        Path(temp_path).unlink(missing_ok=True)

    # -- Page routes serve SPA --

    def test_root_serves_spa(self, client):
        """GET / should serve React SPA index.html."""
        resp = client.get("/")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    def test_posts_page_serves_spa(self, client):
        """GET /posts should serve React SPA."""
        resp = client.get("/posts")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    def test_editor_page_serves_spa(self, client):
        """GET /editor should serve React SPA."""
        resp = client.get("/editor")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    def test_editor_with_path_serves_spa(self, client):
        """GET /editor/some/file.md should serve React SPA."""
        resp = client.get("/editor/some/file.md")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    def test_server_page_serves_spa(self, client):
        """GET /server should serve React SPA."""
        resp = client.get("/server")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    def test_settings_page_serves_spa(self, client):
        """GET /settings should serve React SPA."""
        resp = client.get("/settings")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    # -- SPA fallback for unknown frontend routes --

    def test_spa_404_fallback(self, client):
        """Unknown frontend routes should fallback to SPA index.html."""
        resp = client.get("/unknown-page")
        assert resp.status_code == 200
        assert b"React SPA" in resp.data

    # -- API routes return JSON, not SPA --

    def test_api_404_returns_json(self, client):
        """API 404 should return JSON, not SPA fallback."""
        resp = client.get("/api/nonexistent")
        assert resp.status_code == 404
        data = json.loads(resp.data)
        assert data["success"] is False

    def test_api_settings_endpoint_works(self, client):
        """GET /api/settings should return JSON settings."""
        resp = client.get("/api/settings")
        assert resp.status_code == 200
        data = json.loads(resp.data)
        assert data["success"] is True

    # -- Static dist 404 returns JSON, not SPA --

    def test_static_dist_404_returns_json(self, client):
        """Missing static/dist files should return JSON, not SPA."""
        resp = client.get("/static/dist/missing.js")
        assert resp.status_code == 404
        data = json.loads(resp.data)
        assert data["success"] is False

    # -- Version API --

    def test_version_api(self, client):
        """GET /api/version should return version info."""
        resp = client.get("/api/version")
        assert resp.status_code == 200
        data = json.loads(resp.data)
        assert "version" in data
        assert isinstance(data["version"], str)
