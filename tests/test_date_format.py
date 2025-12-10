#!/usr/bin/env python3
# coding: utf-8
"""
测试日期格式是否正确
"""
import sys
import tempfile
import shutil
from pathlib import Path

# 添加父目录到路径
sys.path.insert(0, str(Path(__file__).parent.parent))

from services.post_service import PostService
import frontmatter
import re


def test_create_post_date_format():
    """测试创建文章时日期格式是否正确"""
    # 创建临时目录
    with tempfile.TemporaryDirectory() as temp_dir:
        content_dir = Path(temp_dir)
        post_dir = content_dir / 'post'
        post_dir.mkdir()

        # 创建 PostService
        service = PostService(str(content_dir), use_cache=False)

        # 创建文章
        success, result = service.create_post("测试文章")

        assert success, f"创建文章失败: {result}"
        print(f"✓ 创建文章成功: {result}")

        # 读取文章内容
        post_file = content_dir / result
        with open(post_file, 'r', encoding='utf-8') as f:
            content = f.read()

        print(f"\n文章内容:\n{content}")

        # 从原始内容中提取日期行
        date_line = [line for line in content.split('\n') if line.startswith('date:')][0]
        date_value = date_line.split('date:', 1)[1].strip()

        print(f"\n日期值: {date_value}")

        # 验证日期格式：应该是字符串，格式为 YYYY-MM-DDTHH:MM:SS+08:00
        # 使用正则验证格式
        date_pattern = r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+08:00$'
        assert re.match(date_pattern, date_value), f"日期格式不正确: {date_value}，应该匹配 {date_pattern}"

        print(f"✓ 日期格式正确: {date_value}")

        # 验证没有引号
        assert not date_value.startswith("'") and not date_value.startswith('"'), "日期不应该有引号"
        print(f"✓ 日期没有引号")

        # 验证可以被 frontmatter 正确解析
        post = frontmatter.loads(content)
        assert post.get('date') is not None, "日期应该能被解析"
        print(f"✓ 日期可以被 frontmatter 正确解析为: {post.get('date')}")

        # 验证其他字段
        assert post.get('title') == "测试文章", "标题不正确"
        assert post.get('draft') is True, "草稿状态不正确"
        assert post.get('categories') == [], "分类应该是空列表"
        assert post.get('tags') == [], "标签应该是空列表"

        print("\n✓ 所有测试通过！")


if __name__ == "__main__":
    test_create_post_date_format()
