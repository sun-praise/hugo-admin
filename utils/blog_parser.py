# coding: utf-8
"""
Hugo 博客文章解析器
用于解析 Hugo 博客的 Markdown 文件和 frontmatter
独立于特定项目，可在任何 Hugo 博客中使用
"""

import pathlib
import re
from datetime import datetime, date
from pathlib import Path

try:
    import frontmatter
except ImportError:
    frontmatter = None


class BlogPost:
    """博客文章数据类"""

    def __init__(self, file_path):
        self.file_path = pathlib.Path(file_path)
        self.relative_path = None
        self.title = ""
        self.date = None
        self.description = ""
        self.tags = []
        self.categories = []
        self.draft = False
        self.content = ""
        self.excerpt = ""
        self.mod_time = None  # 文件修改时间

        # 解析文章
        self._parse()

    def _parse(self):
        """解析 Markdown 文件"""
        if not self.file_path.exists():
            return

        if self.file_path.is_dir():
            # 如果是目录，跳过解析但保留默认值
            self.mod_time = 0
            return

        try:
            # 获取文件修改时间
            self.mod_time = self.file_path.stat().st_mtime

            with open(self.file_path, "r", encoding="utf-8") as f:
                content = f.read()

            # 使用 frontmatter 库解析
            if frontmatter:
                post = frontmatter.loads(content)
                metadata = post.metadata
                self.content = post.content
            else:
                # 简单的 frontmatter 解析作为后备
                metadata, self.content = self._parse_frontmatter_simple(content)

            # 提取元数据
            self.title = metadata.get("title", "")
            self.description = metadata.get("description", "")
            self.draft = metadata.get("draft", False)

            # 处理日期
            date_value = metadata.get("date")
            if date_value:
                if isinstance(date_value, str):
                    # 尝试解析日期字符串
                    try:
                        self.date = datetime.fromisoformat(
                            date_value.replace("Z", "+00:00")
                        )
                    except:
                        try:
                            self.date = datetime.strptime(date_value, "%Y-%m-%d")
                        except:
                            self.date = None
                else:
                    self.date = date_value

            # 处理标签和分类
            self.tags = metadata.get("tags", [])
            if isinstance(self.tags, str):
                self.tags = [self.tags]

            self.categories = metadata.get("categories", [])
            if isinstance(self.categories, str):
                self.categories = [self.categories]

            # 生成摘要
            self.excerpt = self._generate_excerpt()

        except Exception as e:
            print(f"Error parsing {self.file_path}: {e}")

    def _parse_frontmatter_simple(self, content):
        """简单的 frontmatter 解析（后备方案）"""
        metadata = {}
        body = content

        # 检查是否有 frontmatter
        if content.startswith("---"):
            parts = content.split("---", 2)
            if len(parts) >= 3:
                frontmatter_text = parts[1]
                body = parts[2].strip()

                # 简单解析 YAML
                for line in frontmatter_text.split("\n"):
                    line = line.strip()
                    if ":" in line:
                        key, value = line.split(":", 1)
                        key = key.strip()
                        value = value.strip().strip("\"'")

                        # 处理数组
                        if value.startswith("[") and value.endswith("]"):
                            value = [
                                v.strip().strip("\"'") for v in value[1:-1].split(",")
                            ]

                        metadata[key] = value

        return metadata, body

    def _generate_excerpt(self):
        """生成文章摘要"""
        if not self.content:
            return ""

        # 移除 Markdown 语法
        text = re.sub(r"#+ ", "", self.content)  # 移除标题
        text = re.sub(r"\[([^\]]+)\]\([^\)]+\)", r"\1", text)  # 移除链接
        text = re.sub(r"[*_`]", "", text)  # 移除格式标记

        # 获取前200个字符
        excerpt = text.strip()[:200]
        if len(text) > 200:
            excerpt += "..."

        return excerpt

    def to_dict(self):
        """转换为字典"""
        return {
            "file_path": str(self.file_path),
            "relative_path": str(self.relative_path) if self.relative_path else None,
            "title": self.title,
            "date": self.date.isoformat() if self.date else None,
            "description": self.description,
            "tags": self.tags,
            "categories": self.categories,
            "draft": self.draft,
            "excerpt": self.excerpt,
            "content": self.content,
        }


def get_blog_posts(content_dir="content"):
    """
    获取所有博客文章

    Args:
        content_dir: Hugo 内容目录路径

    Returns:
        list: BlogPost 对象列表
    """
    posts = []
    post_dir = pathlib.Path(content_dir) / "post"

    if not post_dir.exists():
        print(f"Warning: Post directory {post_dir} does not exist")
        return posts

    # 递归查找所有 Markdown 文件
    for md_file in post_dir.rglob("*.md"):
        try:
            # 跳过目录
            if md_file.is_dir():
                continue

            post = BlogPost(md_file)

            # 跳过没有标题的文章（解析失败）
            if not post.title and not post.content:
                continue

            # 计算相对路径
            try:
                post.relative_path = md_file.relative_to(content_dir)
            except ValueError:
                # 如果文件不在 content_dir 下，使用绝对路径
                post.relative_path = md_file

            posts.append(post)
        except Exception as e:
            print(f"Error processing {md_file}: {e}")

    # 按日期排序（最新的在前）
    # 处理时区感知和时区naive的datetime对象
    def get_sort_key(post):
        if not post.date:
            return datetime.min.replace(tzinfo=None)

        # 如果是 date 对象而不是 datetime，转换为 datetime
        if isinstance(post.date, date) and not isinstance(post.date, datetime):
            return datetime.combine(post.date, datetime.min.time())

        # 如果是 datetime 且有时区信息，转换为naive datetime
        if hasattr(post.date, "tzinfo") and post.date.tzinfo is not None:
            return post.date.replace(tzinfo=None)

        return post.date

    posts.sort(key=get_sort_key, reverse=True)

    return posts


def filter_posts_by_search(posts, search_query, search_fields=None):
    """
    根据搜索条件过滤文章

    Args:
        posts: BlogPost 对象列表
        search_query: 搜索关键词
        search_fields: 要搜索的字段列表，默认为 ['all']（搜索所有字段）

    Returns:
        list: 过滤后的 BlogPost 列表
    """
    if not search_query:
        return posts

    if search_fields is None:
        search_fields = ["all"]

    search_query = search_query.lower()
    filtered_posts = []

    for post in posts:
        # 如果搜索所有字段
        if "all" in search_fields:
            searchable_text = " ".join(
                [
                    post.title,
                    post.description,
                    post.content,
                    " ".join(post.tags),
                    " ".join(post.categories),
                ]
            ).lower()

            if search_query in searchable_text:
                filtered_posts.append(post)
        else:
            # 搜索指定字段
            match = False

            if "title" in search_fields and search_query in post.title.lower():
                match = True
            elif "content" in search_fields and search_query in post.content.lower():
                match = True
            elif "tags" in search_fields:
                if any(search_query in tag.lower() for tag in post.tags):
                    match = True
            elif "categories" in search_fields:
                if any(search_query in cat.lower() for cat in post.categories):
                    match = True

            if match:
                filtered_posts.append(post)

    return filtered_posts


def get_all_tags(posts):
    """
    获取所有标签及其计数

    Args:
        posts: BlogPost 对象列表

    Returns:
        list: 标签字典列表 [{'name': 'tag', 'count': n}, ...]
    """
    tag_count = {}

    for post in posts:
        for tag in post.tags:
            tag_count[tag] = tag_count.get(tag, 0) + 1

    # 转换为列表并按计数排序
    tags = [{"name": tag, "count": count} for tag, count in tag_count.items()]
    tags.sort(key=lambda x: x["count"], reverse=True)

    return tags


def get_all_categories(posts):
    """
    获取所有分类及其计数

    Args:
        posts: BlogPost 对象列表

    Returns:
        list: 分类字典列表 [{'name': 'category', 'count': n}, ...]
    """
    category_count = {}

    for post in posts:
        for category in post.categories:
            category_count[category] = category_count.get(category, 0) + 1

    # 转换为列表并按计数排序
    categories = [
        {"name": cat, "count": count} for cat, count in category_count.items()
    ]
    categories.sort(key=lambda x: x["count"], reverse=True)

    return categories
