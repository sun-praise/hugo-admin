# coding: utf-8
"""
POST /api/git/push 路由的最小化测试。

覆盖三种响应：
  - 成功 -> 200，``{ success: True, ... }``
  - 失败 -> 400，``{ success: False, ... }``
  - 非 git 仓库 -> 400，``{ success: False, message: "...不是有效的 git 仓库" }``
"""

from unittest.mock import patch

import pytest

import app as app_module


class TestGitPushAPI:
    @pytest.fixture
    def client(self, login):
        app_module.app.config["TESTING"] = True
        with app_module.app.test_client() as c:
            login(c)
            yield c

    def test_push_success(self, client):
        """mock push 返回 (True, 'ok') 时，路由返回 200 + success。"""
        with (
            patch.object(
                app_module.registry.git_service,
                "is_git_repo",
                return_value=True,
            ),
            patch.object(
                app_module.registry.git_service,
                "push",
                return_value=(True, "推送成功到 origin/main"),
            ),
        ):
            resp = client.post(
                "/api/git/push",
                json={"remote": "origin", "branch": "main", "set_upstream": False},
            )
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        assert data["remote"] == "origin"
        assert data["branch"] == "main"
        assert "推送成功" in data["message"]

    def test_push_failure(self, client):
        """mock push 返回 (False, 'boom') 时，路由返回 400 + success=false。"""
        with (
            patch.object(
                app_module.registry.git_service,
                "is_git_repo",
                return_value=True,
            ),
            patch.object(
                app_module.registry.git_service,
                "push",
                return_value=(False, "推送失败: rejected"),
            ),
        ):
            resp = client.post("/api/git/push", json={"branch": "main"})
        assert resp.status_code == 400
        data = resp.get_json()
        assert data["success"] is False
        assert "推送失败" in data["message"]
        assert data["remote"] == "origin"  # 默认值
        assert data["branch"] == "main"

    def test_push_non_git_repo(self, client):
        """is_git_repo 为 False 时，路由直接 400。"""
        with patch.object(
            app_module.registry.git_service, "is_git_repo", return_value=False
        ):
            resp = client.post("/api/git/push", json={"branch": "main"})
        assert resp.status_code == 400
        data = resp.get_json()
        assert data["success"] is False
        assert "不是有效的 git 仓库" in data["message"]

    def test_push_no_body(self, client):
        """无 body 时使用默认 remote=origin，branch=''（push() 内部取当前分支）。"""
        with (
            patch.object(
                app_module.registry.git_service,
                "is_git_repo",
                return_value=True,
            ),
            patch.object(
                app_module.registry.git_service,
                "push",
                return_value=(True, "ok"),
            ) as mock_push,
        ):
            resp = client.post("/api/git/push")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["success"] is True
        # 关键：路由把 default remote 透传给 service 层，branch 留空
        kwargs = mock_push.call_args.kwargs
        assert kwargs["remote"] == "origin"
        assert kwargs["branch"] is None
        assert kwargs["set_upstream"] is False
