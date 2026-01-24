#!/usr/bin/env python3
# coding: utf-8
"""
调试文章解析问题
"""

import sys
from pathlib import Path

# 添加父目录到路径
sys.path.insert(0, str(Path(__file__).parent.parent))

print("=" * 60)
print("调试文章解析问题")
print("=" * 60)

# 测试 1: 导入模块
print("\n1. 测试导入模块...")
try:
    from services.post_service import PostService

    print("   ✓ PostService 导入成功")
except Exception as e:
    print(f"   ✗ PostService 导入失败: {e}")
    sys.exit(1)

# 测试 2: 创建服务实例
print("\n2. 创建 PostService 实例...")
try:
    content_dir = Path(__file__).parent.parent / "content"
    post_service = PostService(content_dir)
    print("   ✓ 实例创建成功")
    print(f"   内容目录: {content_dir}")
    print(f"   文章目录: {post_service.post_dir}")
except Exception as e:
    print(f"   ✗ 实例创建失败: {e}")
    import traceback

    traceback.print_exc()
    sys.exit(1)

# 测试 3: 获取文章列表
print("\n3. 测试获取文章列表...")
try:
    result = post_service.get_posts(per_page=10)
    print("   ✓ 获取成功")
    print(f"   总文章数: {result['total']}")
    print(f"   当前返回: {len(result['posts'])} 篇")
    print(f"   总页数: {result['total_pages']}")
except Exception as e:
    print(f"   ✗ 获取失败: {e}")
    import traceback

    traceback.print_exc()
    sys.exit(1)

# 测试 4: 显示前 5 篇文章
print("\n4. 显示前 5 篇文章...")
try:
    for i, post in enumerate(result["posts"][:5], 1):
        print(f"\n   [{i}] {post['title']}")
        print(f"       路径: {post['path']}")
        print(f"       日期: {post['date']}")
        tags_str = ", ".join(post["tags"][:3])
        suffix = "..." if len(post["tags"]) > 3 else ""
        print(f"       标签: {tags_str}{suffix}")
except Exception as e:
    print(f"   ✗ 显示失败: {e}")
    import traceback

    traceback.print_exc()

# 测试 5: 测试标签和分类
print("\n5. 测试标签和分类...")
try:
    tags = post_service.get_all_tags()
    categories = post_service.get_all_categories()
    print(f"   ✓ 标签数: {len(tags)}")
    print(f"   ✓ 分类数: {len(categories)}")

    if tags:
        print(f"   前 5 个标签: {', '.join([t['name'] for t in tags[:5]])}")
    if categories:
        print(f"   所有分类: {', '.join([c['name'] for c in categories])}")
except Exception as e:
    print(f"   ✗ 获取标签/分类失败: {e}")
    import traceback

    traceback.print_exc()

# 测试 6: 测试文件读取
print("\n6. 测试文件读取...")
try:
    if result["posts"]:
        first_post_path = result["posts"][0]["full_path"]
        success, content = post_service.read_file(first_post_path)

        if success:
            print("   ✓ 文件读取成功")
            print(f"   文件路径: {first_post_path}")
            print(f"   内容长度: {len(content)} 字符")
            print(f"   前 200 字符:\n   {content[:200]}...")
        else:
            print(f"   ✗ 文件读取失败: {content}")
except Exception as e:
    print(f"   ✗ 文件读取异常: {e}")
    import traceback

    traceback.print_exc()

print("\n" + "=" * 60)
print("调试完成!")
print("=" * 60)
