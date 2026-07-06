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

    def test_no_blank_line_accumulation_on_save_cycle(
        self, post_service, temp_content_dir
    ):
        """多次保存不应累积 frontmatter 与正文之间的空行"""
        file_path = temp_content_dir / "no-accum.md"

        # 初始文件：frontmatter 后无空行
        initial = "---\ntitle: Test\n---\nhello world"
        file_path.write_text(initial)

        for _ in range(5):
            success, body, fm, _mtime = post_service.read_file_with_frontmatter(
                str(file_path)
            )
            assert success
            success, msg = post_service.save_file(
                str(file_path), body, frontmatter_data=fm
            )
            assert success

        # 检查最终文件：--- 结束标记与正文之间应只有 1 个空行（dumps 模板固定插入）
        content = file_path.read_text()
        parts = content.split("---\n", 2)
        assert len(parts) >= 3
        # dumps 模板固定插入 \n\n，所以正文前始终有 1 个空行
        # 关键断言：5 次保存后空行数目 ≤ 1（不会累积增长）
        body_section = parts[2]
        leading_newlines = len(body_section) - len(body_section.lstrip("\n"))
        assert (
            leading_newlines <= 1
        ), f"Expected ≤1 leading newlines after 5 saves, got {leading_newlines}"


class TestStripLeadingFrontmatter:
    """_strip_leading_frontmatter 回归测试"""

    def test_strips_leading_blank_lines(self):
        """正文开头空行应被剥除，避免与 frontmatter.dumps 不对称累积"""
        fn = PostService._strip_leading_frontmatter

        # 无开头空行 — 保持不变
        assert fn("hello world") == "hello world"

        # 单个开头空行
        assert fn("\nhello world") == "hello world"

        # 多个开头空行
        assert fn("\n\n\nhello world") == "hello world"

        # Windows 换行符 (\r\n)
        assert fn("\r\n\r\nhello world") == "hello world"

        # 仅空行
        assert fn("\n\n") == ""

        # 空字符串
        assert fn("") == ""

        # None
        assert fn(None) is None
