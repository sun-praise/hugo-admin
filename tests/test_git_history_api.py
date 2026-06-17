# coding: utf-8
"""
GET /api/git/pushes 与 /api/git/commits（enriched）的 API 测试。
"""

import subprocess
import tempfile
from pathlib import Path

import pytest

import app as app_module
from models.database import Database
from services.git_service import GitService


class TestGitHistoryAPI:
    @pytest.fixture
    def temp_repo_with_db(self):
        """临时 git 仓库 + 临时 Database，挂到 registry 上供路由使用。"""
        with tempfile.TemporaryDirectory() as d:
            d = Path(d)
            repo = d / "repo"
            repo.mkdir()
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            subprocess.run(
                ["git", "config", "user.email", "t@t.t"], cwd=repo, check=True
            )
            subprocess.run(["git", "config", "user.name", "t"], cwd=repo, check=True)
            (repo / "a.txt").write_text("a")
            subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
            subprocess.run(["git", "commit", "-qm", "init"], cwd=repo, check=True)

            db = Database(str(d / "cache.db"))
            gs = GitService(repo, database=db)

            # 备份并替换 registry 上的服务
            orig_git = app_module.registry.git_service
            orig_db = app_module.registry.database
            app_module.registry.git_service = gs
            app_module.registry.database = db
            try:
                yield repo, db
            finally:
                app_module.registry.git_service = orig_git
                app_module.registry.database = orig_db

    @pytest.fixture
    def client(self, login):
        app_module.app.config["TESTING"] = True
        with app_module.app.test_client() as c:
            login(c)
            yield c

    # ---------- /api/git/pushes ----------

    def test_pushes_empty(self, client, temp_repo_with_db):
        """无推送历史时返回空列表。"""
        resp = client.get("/api/git/pushes")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["pushes"] == []
        assert data["total"] == 0
        assert data["page"] == 1
        assert data["per_page"] == 20

    def test_pushes_returns_recorded(self, client, temp_repo_with_db):
        """记录推送后能在列表中读到（倒序）。"""
        _, db = temp_repo_with_db
        db.record_push(
            remote="origin",
            branch="main",
            from_sha="",
            to_sha="aaa",
            commit_count=1,
            commit_message="first",
            success=True,
            message="ok",
        )
        db.record_push(
            remote="origin",
            branch="main",
            from_sha="aaa",
            to_sha="bbb",
            commit_count=1,
            commit_message="second",
            success=False,
            message="boom",
        )

        resp = client.get("/api/git/pushes")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["total"] == 2
        # 倒序：second 在前
        assert data["pushes"][0]["commit_message"] == "second"
        assert data["pushes"][0]["success"] is False
        assert data["pushes"][1]["commit_message"] == "first"

    def test_pushes_pagination_clamped(self, client, temp_repo_with_db):
        """page/per_page 被钳制并正确分页。"""
        _, db = temp_repo_with_db
        for i in range(3):
            db.record_push(
                remote="origin",
                branch="main",
                from_sha="",
                to_sha=f"s{i}",
                commit_count=0,
                commit_message=f"m{i}",
                success=True,
                message="ok",
            )

        resp = client.get("/api/git/pushes?page=1&per_page=2")
        data = resp.get_json()
        assert data["total"] == 3
        assert len(data["pushes"]) == 2
        assert data["total_pages"] == 2

        # 越界参数被钳制
        resp2 = client.get("/api/git/pushes?page=0&per_page=999")
        data2 = resp2.get_json()
        assert data2["page"] == 1
        assert data2["per_page"] == 100
        assert len(data2["pushes"]) == 3

    def test_pushes_missing_database(self, client):
        """registry 未注入 database 时返回显式错误而非通用 500。"""
        services = app_module.registry._services
        had_database = "database" in services
        orig = services.get("database")
        # 模拟 database 未初始化: 临时移除 key
        if "database" in services:
            del services["database"]
        try:
            resp = client.get("/api/git/pushes")
            assert resp.status_code == 500
            data = resp.get_json()
            assert data["success"] is False
            assert "数据库未初始化" in data["message"]
        finally:
            if had_database:
                services["database"] = orig
            else:
                services.pop("database", None)

    # ---------- /api/git/commits (enriched) ----------

    def test_commits_have_refs_and_stats(self, client, temp_repo_with_db):
        """/api/git/commits 返回的提交包含 refs 与 stats。"""
        repo, _ = temp_repo_with_db
        (repo / "b.txt").write_text("bb")
        subprocess.run(["git", "add", "-A"], cwd=repo, check=True)
        subprocess.run(["git", "commit", "-qm", "second"], cwd=repo, check=True)

        resp = client.get("/api/git/commits?count=5")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert len(data["commits"]) >= 1
        c = data["commits"][0]
        assert "refs" in c
        assert "stats" in c
        # 旧字段仍在
        for key in ("hash", "author", "email", "date", "message"):
            assert key in c
        assert c["stats"]["files"] >= 1
        assert c["stats"]["insertions"] >= 1
