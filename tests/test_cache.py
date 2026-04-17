#!/usr/bin/env python
# coding: utf-8
"""
测试缓存系统
"""

import os
import sys
import tempfile
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))


def _create_md_file(
    parent: Path, name: str, title: str = "Test", body: str = ""
) -> Path:
    parent.mkdir(parents=True, exist_ok=True)
    p = parent / name
    content = f"---\ntitle: {title}\ndate: 2025-01-01\n---\n\n{body}\n"
    p.write_text(content, encoding="utf-8")
    return p


def _cache_service_with_tmp(content_dir: Path):
    fd, db_path = tempfile.mkstemp(suffix=".db")
    os.close(fd)
    from services.cache_service import CacheService

    svc = CacheService(str(content_dir), db_path)
    return svc, db_path


def test_incremental_no_change():
    """增量更新：文件未修改时不应重新解析"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()
            all_paths_1 = set(svc.db.get_all_file_paths())

            svc._initialized = False
            svc.initialize()
            all_paths_2 = set(svc.db.get_all_file_paths())

            assert all_paths_1 == all_paths_2
        finally:
            os.unlink(db_path)


def test_incremental_new_file():
    """增量更新：新增文件应被缓存"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()

            time.sleep(0.05)
            _create_md_file(post_dir, "b.md", "Beta")

            svc._initialized = False
            svc.initialize()

            titles = [p["title"] for p in svc.db.get_all_posts()]
            assert "Alpha" in titles
            assert "Beta" in titles
        finally:
            os.unlink(db_path)


def test_incremental_deleted_file():
    """增量更新：已删除文件应从缓存移除"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        f1 = _create_md_file(post_dir, "a.md", "Alpha")
        _create_md_file(post_dir, "b.md", "Beta")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()
            assert len(svc.db.get_all_posts()) == 2

            os.unlink(f1)

            svc._initialized = False
            svc.initialize()

            titles = [p["title"] for p in svc.db.get_all_posts()]
            assert "Alpha" not in titles
            assert "Beta" in titles
        finally:
            os.unlink(db_path)


def test_incremental_modified_file():
    """增量更新：mtime 变化时应重新解析"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        f1 = _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()

            time.sleep(0.05)
            f1.write_text(
                "---\ntitle: Alpha Updated\ndate: 2025-01-01\n---\n\nupdated\n",
                encoding="utf-8",
            )

            svc._initialized = False
            svc.initialize()

            posts = svc.db.get_all_posts()
            assert len(posts) == 1
            assert posts[0]["title"] == "Alpha Updated"
        finally:
            os.unlink(db_path)


def test_incremental_empty_db_triggers_full_rebuild():
    """空数据库应走全量扫描路径"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()
            assert svc._initialized
            assert len(svc.db.get_all_posts()) == 1
        finally:
            os.unlink(db_path)


def test_force_rebuild():
    """强制重建应重新扫描所有文件"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()
            time.sleep(0.05)
            _create_md_file(post_dir, "b.md", "Beta")

            svc.initialize(force_rebuild=True)

            titles = [p["title"] for p in svc.db.get_all_posts()]
            assert "Alpha" in titles
            assert "Beta" in titles
        finally:
            os.unlink(db_path)


def test_path_consistency():
    """_full_rebuild 和 _incremental_update 存储的路径格式应一致"""
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        post_dir = content_dir / "post"
        _create_md_file(post_dir, "a.md", "Alpha")

        svc, db_path = _cache_service_with_tmp(content_dir)
        try:
            svc.initialize()
            full_paths = set(svc.db.get_all_file_paths())

            svc._initialized = False
            svc.initialize()
            inc_paths = set(svc.db.get_all_file_paths())

            assert full_paths == inc_paths
        finally:
            os.unlink(db_path)


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
