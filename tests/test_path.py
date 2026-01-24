#!/usr/bin/env python3
# coding: utf-8
"""测试路径解析"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent))
from services.post_service import PostService

# 初始化服务
content_dir = Path(__file__).parent.parent / "content"
post_service = PostService(content_dir)

# 测试路径
test_paths = [
    "post/2025-11-02-一个划算的kilocode-使用方法/index.md",
    "post/2025-11-02-data-extraction-task/index.md",
]

print("Content dir:", content_dir)
print("=" * 60)

for path in test_paths:
    print(f"\n测试路径: {path}")
    success, content = post_service.read_file(path)
    if success:
        print(f"✓ 成功读取，内容长度: {len(content)}")
        print(f"  前 100 字符: {content[:100]}")
    else:
        print(f"✗ 读取失败: {content}")
