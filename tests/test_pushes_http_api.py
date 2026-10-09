# coding: utf-8
"""Git 推送历史 API 的 HTTP 契约测试。

复用 test_git_http_api 的固定日期仓库序列，GitService 注入真实的
temp 数据库（Database），push 两次后经 /api/git/pushes 查询。
pushed_at/pushed_at_iso 由 Go 侧回放归一化。
"""

import os
import subprocess
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from models.database import Database  # noqa: E402
from services.git_service import GitService  # noqa: E402

C1_DATE = {
    "GIT_AUTHOR_DATE": "2026-01-01T12:00:00+08:00",
    "GIT_COMMITTER_DATE": "2026-01-01T12:00:00+08:00",
}
C2_DATE = {
    "GIT_AUTHOR_DATE": "2026-01-02T08:30:00+08:00",
    "GIT_COMMITTER_DATE": "2026-01-02T08:30:00+08:00",
}


def _git(cwd, env, *args):
    full_env = {
        **os.environ,
        "GIT_CONFIG_GLOBAL": "/dev/null",
        "GIT_CONFIG_SYSTEM": "/dev/null",
        **env,
    }
    subprocess.run(
        ["git", *args], cwd=cwd, env=full_env, check=True, capture_output=True
    )


@pytest.fixture
def pushes_client(auth_store, login, tmp_path):
    repo = tmp_path / "repo"
    repo.mkdir()
    bare = str(tmp_path / "remote.git")
    post_dir = repo / "content" / "post"
    post_dir.mkdir(parents=True)

    _git(repo, {}, "init", "-b", "main")
    _git(repo, {}, "config", "user.name", "契约测试")
    _git(repo, {}, "config", "user.email", "contract@test.local")

    (post_dir / "a.md").write_text("a\n", encoding="utf-8")
    _git(repo, C1_DATE, "add", "-A")
    _git(repo, C1_DATE, "commit", "-m", "init: 第一提交")
    (post_dir / "b.md").write_text("b\n", encoding="utf-8")
    _git(repo, C2_DATE, "add", "-A")
    _git(repo, C2_DATE, "commit", "-m", "feat: 第二提交（含|竖线）")

    _git(tmp_path, {}, "init", "-b", "main", "--bare", bare)
    _git(repo, {}, "remote", "add", "origin", bare)

    database = Database(str(tmp_path / "cache.db"))
    original_git = app_module.registry.git_service
    original_db = app_module.registry.database
    app_module.registry.git_service = GitService(str(repo), database=database)
    app_module.registry.database = database
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.registry.git_service = original_git
        app_module.registry.database = original_db


class TestPushesHTTPAPI:
    def test_push_records_and_pagination(self, pushes_client):
        # 两次推送（首推 + up-to-date），都成功且各记录一条历史
        first = pushes_client.post("/api/git/push", json={})
        assert first.status_code == 200
        second = pushes_client.post("/api/git/push", json={})
        assert second.status_code == 200

        data = pushes_client.get("/api/git/pushes").get_json()
        assert data["success"] is True
        assert data["total"] == 2
        assert data["page"] == 1 and data["per_page"] == 20
        assert data["total_pages"] == 1

        # 倒序：最近一次在前（up-to-date：from==to、commit_count=0）
        latest = data["pushes"][0]
        assert latest["success"] is True
        assert latest["commit_count"] == 0
        assert latest["commit_message"] == "feat: 第二提交（含|竖线）"
        assert len(latest["to_sha"]) == 40

        first_push = data["pushes"][1]
        assert first_push["from_sha"] == ""  # 首推无 remote head
        # 首推 from 缺失时无法界定范围，Python 语义返回 0（UI 隐藏该字段）
        assert first_push["commit_count"] == 0

        # 分页（同一 fixture 内追加查询，回放按序可重放）
        data = pushes_client.get("/api/git/pushes?per_page=1&page=2").get_json()
        assert data["total"] == 2
        assert data["total_pages"] == 2
        assert len(data["pushes"]) == 1
        assert data["pushes"][0]["from_sha"] == ""  # 第一条（首推）
