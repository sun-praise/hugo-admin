# coding: utf-8
"""
文章缓存服务
负责管理文章数据的缓存，检测文件变化并增量更新
"""

from pathlib import Path
from typing import Any, Dict, List

from models.database import Database

# 导入内部模块
from utils.blog_parser import BlogPost, get_blog_posts


class CacheService:
    """文章缓存服务"""

    def __init__(self, content_dir: str, db_path: str = None):
        """
        初始化缓存服务

        Args:
            content_dir: 内容目录路径
            db_path: 数据库文件路径，默认为 web_admin/data/cache.db
        """
        self.content_dir = Path(content_dir)

        if db_path is None:
            db_path = Path(__file__).parent.parent / "data" / "cache.db"

        self.db = Database(str(db_path))
        self._initialized = False

    def initialize(self, force_rebuild: bool = False):
        """
        初始化缓存
        在应用启动时调用，扫描所有文件并更新缓存

        Args:
            force_rebuild: 是否强制重建整个缓存
        """
        if self._initialized and not force_rebuild:
            return

        print("正在初始化文章缓存...")
        if force_rebuild:
            print("强制重建缓存...")

        # 获取当前文件系统中的所有文章
        current_posts = get_blog_posts(str(self.content_dir))
        current_paths = {str(post.file_path) for post in current_posts}

        # 获取数据库中的所有文章路径
        cached_paths = set(self.db.get_all_file_paths())

        # 找出需要更新和删除的文章
        to_update = []
        to_delete = cached_paths - current_paths

        for post in current_posts:
            file_path = str(post.file_path)

            if force_rebuild or file_path not in cached_paths:
                # 新文章或强制重建
                to_update.append(post)
            else:
                cached_post = self.db.get_post(file_path)
                if cached_post and (
                    cached_post["mod_time"] != post.mod_time
                    or not cached_post.get("cover")
                ):
                    to_update.append(post)

        # 更新缓存
        update_count = 0
        for post in to_update:
            self._cache_post(post)
            update_count += 1

        # 删除不存在的文章
        delete_count = 0
        for file_path in to_delete:
            self.db.delete_post(file_path)
            delete_count += 1

        self._initialized = True
        print(f"缓存初始化完成: 更新 {update_count} 篇, 删除 {delete_count} 篇")

    def refresh(self):
        """
        刷新缓存
        检查文件变化并更新缓存
        """
        self.initialize(force_rebuild=False)

    @staticmethod
    def _resolve_cover_url(relative_path, cover):
        if not cover:
            return ""
        cover_str = str(cover).strip()
        if not cover_str:
            return ""
        if cover_str.startswith(("http://", "https://", "/")):
            return cover_str
        post_dir = Path(relative_path).parent
        resolved = post_dir / cover_str
        try:
            parts = resolved.parts
            normalized = []
            for part in parts:
                if part == "..":
                    if normalized:
                        normalized.pop()
                elif part != ".":
                    normalized.append(part)
            if not normalized:
                return ""
            return "/content/" + "/".join(normalized)
        except (ValueError, IndexError):
            return f"/content/{post_dir}/{cover_str}"

    def get_posts(
        self,
        query: str = "",
        category: str = "",
        tag: str = "",
        page: int = 1,
        per_page: int = 20,
    ) -> Dict[str, Any]:
        """
        获取文章列表

        Args:
            query: 搜索关键词
            category: 分类筛选
            tag: 标签筛选
            page: 页码
            per_page: 每页数量

        Returns:
            包含文章列表和分页信息的字典
        """
        # 确保缓存已初始化
        if not self._initialized:
            self.initialize()

        # 从数据库查询
        if query or category or tag:
            all_posts = self.db.search_posts(query, category, tag)
        else:
            all_posts = self.db.get_all_posts(order_by="date DESC")

        # 计算分页
        total = len(all_posts)
        total_pages = (total + per_page - 1) // per_page
        start = (page - 1) * per_page
        end = start + per_page

        # 获取当前页文章
        page_posts = all_posts[start:end]

        # 转换为 API 格式
        from datetime import datetime

        posts_data = []
        for post in page_posts:
            posts_data.append(
                {
                    "title": post["title"],
                    "path": post["relative_path"],
                    "full_path": post["file_path"],
                    "date": post["date"][:10] if post["date"] else "",
                    "description": post["description"],
                    "excerpt": post["excerpt"],
                    "tags": post["tags"],
                    "categories": post["categories"],
                    "cover": post.get("cover", ""),
                    "cover_url": self._resolve_cover_url(
                        post["relative_path"], post.get("cover", "")
                    ),
                    "mod_time": datetime.fromtimestamp(post["mod_time"]).strftime(
                        "%Y-%m-%d %H:%M"
                    ),
                }
            )

        return {
            "posts": posts_data,
            "total": total,
            "page": page,
            "per_page": per_page,
            "total_pages": total_pages,
            "has_next": page < total_pages,
            "has_prev": page > 1,
        }

    def get_all_tags(self) -> List[Dict[str, Any]]:
        """
        获取所有标签

        Returns:
            标签列表(包含文章数量)
        """
        if not self._initialized:
            self.initialize()

        return self.db.get_all_tags()

    def get_all_categories(self) -> List[Dict[str, Any]]:
        """
        获取所有分类

        Returns:
            分类列表(包含文章数量)
        """
        if not self._initialized:
            self.initialize()

        return self.db.get_all_categories()

    def invalidate_post(self, file_path: str):
        """
        使特定文章的缓存失效并重新加载

        Args:
            file_path: 文件路径（相对或绝对）
        """
        # 转换为绝对路径
        if not Path(file_path).is_absolute():
            file_path = str(self.content_dir / file_path)

        file_path = str(Path(file_path).resolve())

        # 检查文件是否存在
        if not Path(file_path).exists():
            # 文件已删除，从缓存中移除
            self.db.delete_post(file_path)
            print(f"从缓存中删除: {file_path}")
            return

        # 重新加载文章
        try:
            # 使用 BlogPost 类加载单个文件
            post = BlogPost(file_path)
            # 设置相对路径
            try:
                post.relative_path = Path(file_path).relative_to(self.content_dir)
            except ValueError:
                post.relative_path = Path(file_path)
            self._cache_post(post)
            print(f"更新缓存: {file_path}")
        except Exception as e:
            print(f"无法加载文章 {file_path}: {e}")

    def _cache_post(self, post: BlogPost):
        """
        将文章数据存入缓存

        Args:
            post: BlogPost 实例
        """
        post_data = {
            "file_path": str(post.file_path),
            "relative_path": str(post.relative_path),
            "title": post.title,
            "date": post.date,
            "description": post.description,
            "excerpt": post.excerpt,
            "tags": post.tags,
            "categories": post.categories,
            "cover": post.cover,
            "mod_time": post.mod_time,
        }

        self.db.upsert_post(post_data)

    def get_stats(self) -> Dict[str, Any]:
        """
        获取缓存统计信息

        Returns:
            统计信息字典
        """
        all_posts = self.db.get_all_posts()
        tags = self.db.get_all_tags()
        categories = self.db.get_all_categories()

        return {
            "total_posts": len(all_posts),
            "total_tags": len(tags),
            "total_categories": len(categories),
            "initialized": self._initialized,
        }
