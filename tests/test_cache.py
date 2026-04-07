#!/usr/bin/env python
# coding: utf-8
"""
测试缓存系统
"""

import sys
from pathlib import Path

# 添加父目录到路径
sys.path.insert(0, str(Path(__file__).parent.parent))


def test_database():
    """测试数据库基本功能"""
    import os
    import tempfile

    from models.database import Database

    print("=" * 50)
    print("测试数据库基本功能")
    print("=" * 50)

    # 使用临时数据库
    with tempfile.NamedTemporaryFile(delete=False, suffix=".db") as f:
        db_path = f.name

    try:
        db = Database(db_path)

        # 测试插入
        post_data = {
            "file_path": "/test/post1.md",
            "relative_path": "post/post1.md",
            "title": "测试文章1",
            "date": "2025-01-01",
            "description": "这是一篇测试文章",
            "excerpt": "摘要内容",
            "tags": ["Python", "Flask"],
            "categories": ["编程"],
            "mod_time": 1234567890.0,
        }

        db.upsert_post(post_data)
        print("✓ 插入文章成功")

        # 测试查询
        post = db.get_post("/test/post1.md")
        assert post is not None
        assert post["title"] == "测试文章1"
        assert post["tags"] == ["Python", "Flask"]
        print("✓ 查询文章成功")

        # 测试获取所有文章
        all_posts = db.get_all_posts()
        assert len(all_posts) == 1
        print("✓ 获取所有文章成功")

        # 测试标签统计
        tags = db.get_all_tags()
        assert len(tags) == 2
        print(f"✓ 标签统计成功: {tags}")

        # 测试分类统计
        categories = db.get_all_categories()
        assert len(categories) == 1
        print(f"✓ 分类统计成功: {categories}")

        # 测试删除
        db.delete_post("/test/post1.md")
        post = db.get_post("/test/post1.md")
        assert post is None
        print("✓ 删除文章成功")

        print("\n数据库测试全部通过!")

    finally:
        # 清理
        if os.path.exists(db_path):
            os.remove(db_path)


def test_cache_service():
    """测试缓存服务"""
    print("\n" + "=" * 50)
    print("测试缓存服务")
    print("=" * 50)

    try:
        import os
        import tempfile

        from services.cache_service import CacheService

        # 使用临时数据库
        with tempfile.NamedTemporaryFile(delete=False, suffix=".db") as f:
            db_path = f.name

        content_dir = str(Path(__file__).parent.parent / "content")

        try:
            cache_service = CacheService(content_dir, db_path)
            print(f"内容目录: {content_dir}")

            # 初始化缓存
            print("\n初始化缓存...")
            cache_service.initialize()

            # 获取统计信息
            stats = cache_service.get_stats()
            print("\n缓存统计:")
            print(f"  - 文章总数: {stats['total_posts']}")
            print(f"  - 标签数量: {stats['total_tags']}")
            print(f"  - 分类数量: {stats['total_categories']}")
            print(f"  - 已初始化: {stats['initialized']}")

            # 获取文章列表
            result = cache_service.get_posts(page=1, per_page=5)
            print("\n文章列表 (第1页, 每页5篇):")
            print(f"  - 总数: {result['total']}")
            print(f"  - 总页数: {result['total_pages']}")
            print(f"  - 当前页文章数: {len(result['posts'])}")

            if result["posts"]:
                print("\n前3篇文章:")
                for post in result["posts"][:3]:
                    print(f"  - {post['title']} ({post['date']})")

            # 获取标签
            tags = cache_service.get_all_tags()
            print(f"\n标签统计 (共{len(tags)}个):")
            for tag in tags[:5]:
                print(f"  - {tag['name']}: {tag['count']}篇")

            # 获取分类
            categories = cache_service.get_all_categories()
            print(f"\n分类统计 (共{len(categories)}个):")
            for cat in categories[:5]:
                print(f"  - {cat['name']}: {cat['count']}篇")

            print("\n缓存服务测试完成!")

        finally:
            # 清理
            if os.path.exists(db_path):
                os.remove(db_path)

    except Exception as e:
        print(f"✗ 缓存服务测试失败: {e}")
        import traceback

        traceback.print_exc()


if __name__ == "__main__":
    test_database()
    test_cache_service()
