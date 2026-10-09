# coding: utf-8
"""Git API 的 HTTP 契约测试。

fixture 构造与 Go 侧 internal/git/git_test.go 的 setupRepo **完全相同的**
仓库序列（固定 author/committer 日期），commit hash 由此确定，
Go 契约回放可严格比对。publish 的文件改动通过 /api/file/save 制造，
保证回放能完整重放状态流（contentDir 即 repo/content）。
"""

import os
import subprocess
import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.git_service import GitService  # noqa: E402
from services.post_service import PostService  # noqa: E402

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
def git_client(auth_store, login, tmp_path):
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

    # staged / unstaged / untracked 各一个
    (post_dir / "d.md").write_text("d\n", encoding="utf-8")
    _git(repo, {}, "add", "content/post/d.md")
    (post_dir / "b.md").write_text("b changed\n", encoding="utf-8")
    (post_dir / "c.md").write_text("c\n", encoding="utf-8")

    _git(tmp_path, {}, "init", "-b", "main", "--bare", bare)
    _git(repo, {}, "remote", "add", "origin", bare)

    original_git = app_module.registry.git_service
    original_post = app_module.registry.post_service
    app_module.registry.git_service = GitService(str(repo), database=None)
    app_module.registry.post_service = PostService(
        str(repo / "content"), use_cache=False
    )
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.registry.git_service = original_git
        app_module.registry.post_service = original_post


class TestGitHTTPAPI:
    def test_status(self, git_client):
        data = git_client.get("/api/git/status").get_json()
        assert data["success"] is True
        assert data["staged"] == ["content/post/d.md"]
        # Python 既有语义：untracked（"?? "）的 Y 位非空，同样计入 unstaged
        assert data["unstaged"] == ["content/post/b.md", "content/post/c.md"]
        assert data["untracked"] == ["content/post/c.md"]

    def test_commits(self, git_client):
        data = git_client.get("/api/git/commits?count=2").get_json()
        assert data["success"] is True
        assert len(data["commits"]) == 2
        first = data["commits"][0]
        assert first["author"] == "契约测试"
        assert first["refs"] == "HEAD -> main"
        assert first["message"] == "feat: 第二提交（含|竖线）"
        assert first["stats"] == {"files": 1, "insertions": 1, "deletions": 0}
        assert len(first["hash"]) == 40

    def test_push(self, git_client):
        resp = git_client.post("/api/git/push", json={})
        assert resp.status_code == 200
        data = resp.get_json()
        assert data == {
            "success": True,
            "message": "推送成功到 origin/main",
            "remote": "origin",
            "branch": "",
        }

    def test_publish_system(self, git_client):
        # 通过已实现的文件保存端点制造改动，保证契约回放可重放
        save = git_client.post(
            "/api/file/save",
            json={"path": "post/e.md", "content": "e\n"},
        )
        assert save.status_code == 200

        resp = git_client.post(
            "/api/publish/system", json={"message": "release: 测试发布"}
        )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["message"] == "系统发布成功，GitHub Actions 将自动构建站点"
        assert data["steps"]["commit"]["message"] == "提交成功: release: 测试发布"

        # 紧接着再发布：无改动 → 400（与上面共用同一 fixture，回放按序可重放）
        again = git_client.post("/api/publish/system", json={"message": "again"})
        assert again.status_code == 400
        assert again.get_json()["message"] == "没有需要发布的改动"
