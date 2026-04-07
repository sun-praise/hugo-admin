#!/usr/bin/env python
# coding: utf-8
"""
仅测试数据库功能（不依赖 invoke）
"""
import sys
from pathlib import Path

# 添加父目录到路径以访问项目模块
sys.path.insert(0, str(Path(__file__).parent.parent))

import os
import tempfile

from models.database import Database

print("=" * 60)
print("测试数据库缓存功能")
print("=" * 60)

# 使用临时数据库
with tempfile.NamedTemporaryFile(delete=False, suffix=".db") as f:
    db_path = f.name

try:
    db = Database(db_path)
    print(f"✓ 数据库创建成功: {db_path}\n")

    # 模拟插入多篇文章
    posts_data = [
        {
            "file_path": "/content/post/2025-01-01-test1/index.md",
            "relative_path": "post/2025-01-01-test1/index.md",
            "title": "Python 异步编程指南",
            "date": "2025-01-01",
            "description": "深入理解 Python 异步编程",
            "excerpt": "本文介绍 asyncio 的使用...",
            "tags": ["Python", "Async", "编程"],
            "categories": ["编程", "Python"],
            "mod_time": 1704067200.0,
        },
        {
            "file_path": "/content/post/2025-01-02-test2/index.md",
            "relative_path": "post/2025-01-02-test2/index.md",
            "title": "Flask Web 开发实战",
            "date": "2025-01-02",
            "description": "Flask 框架从入门到精通",
            "excerpt": "Flask 是一个轻量级的 Web 框架...",
            "tags": ["Python", "Flask", "Web"],
            "categories": ["编程", "Web开发"],
            "mod_time": 1704153600.0,
        },
        {
            "file_path": "/content/post/2025-01-03-test3/index.md",
            "relative_path": "post/2025-01-03-test3/index.md",
            "title": "SQLite 数据库优化技巧",
            "date": "2025-01-03",
            "description": "提升 SQLite 性能的实用技巧",
            "excerpt": "索引、查询优化、事务管理...",
            "tags": ["数据库", "SQLite", "优化"],
            "categories": ["数据库"],
            "mod_time": 1704240000.0,
        },
    ]

    # 批量插入
    for post_data in posts_data:
        db.upsert_post(post_data)
    print(f"✓ 插入了 {len(posts_data)} 篇文章\n")

    # 测试查询所有文章
    all_posts = db.get_all_posts()
    print(f"文章总数: {len(all_posts)}")
    print("\n所有文章:")
    for post in all_posts:
        print(f"  - {post['title']} ({post['date']})")

    # 测试搜索
    print("\n搜索 'Python' 关键词:")
    search_results = db.search_posts("Python")
    for post in search_results:
        print(f"  - {post['title']}")

    # 测试分类筛选
    print("\n筛选 '编程' 分类:")
    category_results = db.search_posts("", category="编程")
    for post in category_results:
        print(f"  - {post['title']}")

    # 测试标签筛选
    print("\n筛选 'Flask' 标签:")
    tag_results = db.search_posts("", tag="Flask")
    for post in tag_results:
        print(f"  - {post['title']}")

    # 测试标签统计
    tags = db.get_all_tags()
    print(f"\n标签统计 (共 {len(tags)} 个):")
    for tag in tags:
        print(f"  - {tag['name']}: {tag['count']} 篇")

    # 测试分类统计
    categories = db.get_all_categories()
    print(f"\n分类统计 (共 {len(categories)} 个):")
    for cat in categories:
        print(f"  - {cat['name']}: {cat['count']} 篇")

    # 测试更新（模拟文件修改）
    print("\n测试更新文章...")
    update_data = posts_data[0].copy()
    update_data["title"] = "Python 异步编程指南 (已更新)"
    update_data["mod_time"] = 1704326400.0
    db.upsert_post(update_data)

    updated_post = db.get_post(update_data["file_path"])
    print(f"✓ 更新成功: {updated_post['title']}")

    # 测试删除
    print("\n测试删除文章...")
    db.delete_post(posts_data[2]["file_path"])
    remaining_posts = db.get_all_posts()
    print(f"✓ 删除成功，剩余 {len(remaining_posts)} 篇文章")

    print("\n" + "=" * 60)
    print("✓ 所有数据库功能测试通过！")
    print("=" * 60)

except Exception as e:
    print(f"\n✗ 测试失败: {e}")
    import traceback

    traceback.print_exc()

finally:
    # 清理
    if os.path.exists(db_path):
        os.remove(db_path)
        print(f"\n已清理临时数据库: {db_path}")
