# coding: utf-8
"""
PostService 发布功能测试
"""

import tempfile
from pathlib import Path

import frontmatter
import pytest

from services.post_service import PostService


class TestPostServicePublish:
    @pytest.fixture
    def temp_content_dir(self):
        """临时内容目录"""
        with tempfile.TemporaryDirectory() as temp_dir:
            yield Path(temp_dir)

    @pytest.fixture
    def temp_article(self, temp_content_dir):
        """临时测试文章"""
        article_path = temp_content_dir / "test-article.md"
        article_content = """---
title: Test Article
draft: true
date: 2025-11-14
categories: ["tech"]
tags: ["test"]
---

This is test content for the article.
"""
        article_path.write_text(article_content)
        yield article_path

    @pytest.fixture
    def post_service(self, temp_content_dir):
        """PostService 实例"""
        return PostService(temp_content_dir, use_cache=False)

    def test_publish_article_service(self, post_service, temp_article):
        """测试 PostService 发布文章"""
        success, message, operation_id = post_service.publish_article(str(temp_article))

        assert success is True
        assert "发布成功" in message
        assert operation_id is not None

        # 验证文件被更改
        post = frontmatter.load(str(temp_article))
        assert post.get("draft") is False
        assert "publishDate" in post.metadata

    def test_publish_already_published_service(self, post_service, temp_article):
        """测试发布已发布的文章"""
        # 首先发布一次
        post_service.publish_article(str(temp_article))

        # 再次尝试发布
        success, message, operation_id = post_service.publish_article(str(temp_article))

        assert success is False
        assert "已经发布" in message
        assert operation_id is not None

    def test_get_publish_status_draft(self, post_service, temp_article):
        """测试获取草稿文章状态"""
        status = post_service.get_publish_status(str(temp_article))

        assert status["is_draft"] is True
        assert status["is_publishable"] is True
        assert status["last_published"] is None
        assert status["frontmatter"]["title"] == "Test Article"

    def test_get_publish_status_published(self, post_service, temp_article):
        """测试获取已发布文章状态"""
        # 先发布文章
        post_service.publish_article(str(temp_article))

        status = post_service.get_publish_status(str(temp_article))

        assert status["is_draft"] is False
        assert status["is_publishable"] is False
        assert status["last_published"] is not None

    def test_bulk_publish_articles(self, post_service, temp_content_dir):
        """测试批量发布文章"""
        # 创建多个测试文章
        article1 = temp_content_dir / "article1.md"
        article2 = temp_content_dir / "article2.md"

        article1.write_text(
            """---
title: Article 1
draft: true
---

Content 1
"""
        )

        article2.write_text(
            """---
title: Article 2
draft: true
---

Content 2
"""
        )

        result = post_service.bulk_publish_articles([str(article1), str(article2)])

        assert result["success"] is True
        assert result["total_count"] == 2
        assert result["published_count"] == 2
        assert result["failed_count"] == 0
        assert len(result["results"]) == 2

        # 验证所有文章都被发布
        post1 = frontmatter.load(str(article1))
        post2 = frontmatter.load(str(article2))
        assert post1.get("draft") is False
        assert post2.get("draft") is False
