#!/usr/bin/env python3
# coding: utf-8
"""
测试 API 和统计功能
"""

import sys
from pathlib import Path

# 添加父目录到路径
sys.path.insert(0, str(Path(__file__).parent.parent))

from services.post_service import PostService

# 初始化服务
content_dir = Path(__file__).parent.parent / "content"
post_service = PostService(content_dir)

# 测试获取文章
print("=" * 50)
print("测试文章统计")
print("=" * 50)

result = post_service.get_posts(per_page=10000)  # 获取所有文章
print(f"总文章数: {result['total']}")
print(f"总页数: {result['total_pages']}")
print(f"当前返回文章数: {len(result['posts'])}")
print()

# 测试标签
print("=" * 50)
print("测试标签统计")
print("=" * 50)
tags = post_service.get_all_tags()
print(f"总标签数: {len(tags)}")
if tags:
    print("前 10 个标签:")
    for tag in tags[:10]:
        print(f"  - {tag['name']}: {tag['count']} 篇")
print()

# 测试分类
print("=" * 50)
print("测试分类统计")
print("=" * 50)
categories = post_service.get_all_categories()
print(f"总分类数: {len(categories)}")
if categories:
    print("所有分类:")
    for cat in categories:
        print(f"  - {cat['name']}: {cat['count']} 篇")
print()

# 显示前几篇文章
print("=" * 50)
print("前 5 篇文章")
print("=" * 50)
for i, post in enumerate(result["posts"][:5], 1):
    print(f"{i}. {post['title']}")
    print(f"   路径: {post['path']}")
    print(f"   日期: {post['date']}")
    print(f"   标签: {', '.join(post['tags'])}")
    print()
