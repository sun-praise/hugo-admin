# coding: utf-8
"""
发布功能集成测试
"""

import pytest
import tempfile
from pathlib import Path
import frontmatter

import app as app_module
from services.post_service import PostService


class TestPublishIntegration:
    @pytest.fixture
    def temp_content_dir(self):
        """临时内容目录"""
        with tempfile.TemporaryDirectory() as temp_dir:
            content_dir = Path(temp_dir) / "content"
            content_dir.mkdir()
            yield content_dir

    @pytest.fixture
    def post_service(self, temp_content_dir):
        """PostService 实例"""
        return PostService(temp_content_dir, use_cache=False)

    @pytest.fixture
    def client(self, temp_content_dir):
        """测试客户端 - 配置 app 使用临时目录"""
        app_module.app.config["TESTING"] = True
        original_post_service = app_module.post_service
        app_module.post_service = PostService(temp_content_dir, use_cache=False)
        with app_module.app.test_client() as client:
            yield client
        app_module.post_service = original_post_service

    def test_full_publish_workflow(self, client, temp_content_dir):
        """测试完整的发布工作流程"""
        # 创建测试文章
        article_path = temp_content_dir / "workflow-test.md"
        article_content = """---
title: Workflow Test
draft: true
date: 2025-11-14
tags: ["integration"]
---

This is a workflow test.
"""
        article_path.write_text(article_content)

        # 步骤 1: 检查初始状态
        response = client.get(f"/api/article/status?file_path={str(article_path)}")
        assert response.status_code == 200
        data = response.get_json()
        assert data["status"]["is_draft"] is True

        # 步骤 2: 发布文章
        response = client.post(
            "/api/article/publish", json={"file_path": str(article_path)}
        )
        assert response.status_code == 200
        data = response.get_json()
        assert data["success"] is True
        _operation_id = data["operation_id"]  # noqa: F841

        # 步骤 3: 验证更新后的状态
        response = client.get(f"/api/article/status?file_path={str(article_path)}")
        assert response.status_code == 200
        data = response.get_json()
        assert data["status"]["is_draft"] is False
        assert data["status"]["last_published"] is not None

        # 步骤 4: 验证文件内容
        post = frontmatter.load(str(article_path))
        assert post.get("draft") is False
        assert "publishDate" in post.metadata

    def test_publish_error_handling(self, client, temp_content_dir):
        """测试发布错误处理"""
        invalid_article = temp_content_dir / "invalid.md"
        invalid_article.write_text(
            """---
draft: true
---

No title here.
"""
        )

        response = client.post(
            "/api/article/publish", json={"file_path": str(invalid_article)}
        )

        assert response.status_code in [200, 400, 409]

    def test_concurrent_publish_attempt(self, client, post_service, temp_content_dir):
        """测试并发发布尝试"""
        # 创建测试文章
        article_path = temp_content_dir / "concurrent-test.md"
        article_path.write_text(
            """---
title: Concurrent Test
draft: true
---

Content for concurrent test.
"""
        )

        # 这个测试主要验证文件锁机制不会导致死锁
        # 在实际的并发环境中，这需要更复杂的测试设置
        success, message, operation_id = post_service.publish_article(str(article_path))
        assert success is True

        # 再次尝试应该返回已发布
        success, message, operation_id = post_service.publish_article(str(article_path))
        assert success is False
        assert "已经发布" in message
