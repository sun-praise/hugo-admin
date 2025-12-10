# coding: utf-8
"""
文章管理服务
负责文章的读取、保存、搜索等操作
复用 tasks.py 中的 BlogPost 类
"""
import os
import sys
from pathlib import Path
from datetime import datetime, timezone, timedelta
import yaml
import uuid
import fcntl
import time
import frontmatter

# 导入内部模块
from utils.blog_parser import BlogPost, get_blog_posts, filter_posts_by_search
from services.cache_service import CacheService


class PostService:
    """文章管理服务"""

    def __init__(self, content_dir, use_cache=True):
        """
        初始化文章服务

        Args:
            content_dir: 内容目录路径
            use_cache: 是否使用缓存
        """
        self.content_dir = Path(content_dir)
        self.post_dir = self.content_dir / 'post'
        self.use_cache = use_cache

        # 初始化缓存服务
        if use_cache:
            self.cache_service = CacheService(content_dir)
        else:
            self.cache_service = None

    def publish_article(self, file_path):
        """
        发布文章 - 将 draft 状态从 true 改为 false

        Args:
            file_path: 文章文件路径（相对于 content 目录或绝对路径）

        Returns:
            tuple: (success, message, operation_id)
        """
        operation_id = str(uuid.uuid4())

        try:
            # 处理路径
            if not Path(file_path).is_absolute():
                file_path = self.content_dir / file_path

            file_path = Path(file_path)

            # 安全检查
            if not self._is_safe_path(file_path):
                return False, "访问被拒绝:文件不在允许的目录中", operation_id

            # 检查文件是否存在
            if not file_path.exists():
                return False, f"文件不存在: {file_path}", operation_id

            # 使用文件锁进行并发控制
            def publish_operation(file_handle):
                try:
                    # 加载文章
                    post = frontmatter.load(file_path)

                    # 检查是否已经是发布状态
                    if not post.get('draft', False):
                        return False, "文章已经发布", False

                    # 更新 draft 状态
                    post.metadata['draft'] = False

                    # 如果没有 publishDate，添加发布时间（使用东八区时区）
                    if 'publishDate' not in post.metadata:
                        tz_cn = timezone(timedelta(hours=8))
                        now = datetime.now(tz_cn)
                        post.metadata['publishDate'] = now.strftime('%Y-%m-%dT%H:%M:%S+08:00')

                    # 保存文件
                    frontmatter.dump(post, file_handle.name)
                    return True, "文章发布成功", True

                except Exception as e:
                    return False, f"发布操作失败: {str(e)}", False

            # 执行带锁的发布操作
            result, message, status_changed = self._safe_file_operation(str(file_path), publish_operation)

            # 如果发布成功，更新缓存
            if result and self.use_cache and self.cache_service:
                self.cache_service.invalidate_post(str(file_path))

            return result, message, operation_id

        except Exception as e:
            return False, f"发布操作失败: {str(e)}", operation_id

    def bulk_publish_articles(self, file_paths):
        """
        批量发布文章

        Args:
            file_paths: 文章文件路径列表

        Returns:
            dict: 批量操作结果
        """
        operation_id = str(uuid.uuid4())
        results = []
        published_count = 0
        failed_count = 0

        for file_path in file_paths:
            success, message, _ = self.publish_article(file_path)

            # 使用东八区时区
            tz_cn = timezone(timedelta(hours=8))
            published_at = datetime.now(tz_cn).strftime('%Y-%m-%dT%H:%M:%S+08:00') if success else None

            result = {
                'file_path': file_path,
                'success': success,
                'message': message if not success else None,
                'published_at': published_at
            }
            results.append(result)

            if success:
                published_count += 1
            else:
                failed_count += 1

        return {
            'success': failed_count == 0,
            'total_count': len(file_paths),
            'published_count': published_count,
            'failed_count': failed_count,
            'operation_id': operation_id,
            'results': results,
            'duration_ms': 0  # 可以添加计时逻辑
        }

    def get_publish_status(self, file_path):
        """
        获取文章发布状态

        Args:
            file_path: 文章文件路径（相对于 content 目录或绝对路径）

        Returns:
            dict: 发布状态信息
        """
        try:
            # 处理路径
            if not Path(file_path).is_absolute():
                file_path = self.content_dir / file_path

            file_path = Path(file_path)

            # 安全检查
            if not self._is_safe_path(file_path):
                return {
                    'error': '访问被拒绝:文件不在允许的目录中',
                    'file_path': str(file_path)
                }

            # 检查文件是否存在
            if not file_path.exists():
                return {
                    'error': '文件不存在',
                    'file_path': str(file_path)
                }

            # 加载文章 frontmatter
            post = frontmatter.load(str(file_path))
            is_draft = post.get('draft', True)  # 默认为 draft

            # 检查是否可以发布
            is_publishable = is_draft  # 简化逻辑，后续可以扩展
            publish_errors = []

            # 验证必要的 frontmatter 字段
            if not post.get('title'):
                publish_errors.append('缺少标题')

            return {
                'file_path': str(file_path),
                'is_draft': is_draft,
                'is_publishable': is_publishable,
                'last_published': post.get('publishDate') if not is_draft else None,
                'publish_errors': publish_errors,
                'frontmatter': dict(post.metadata)  # 返回 frontmatter 的副本
            }

        except Exception as e:
            return {
                'error': f'状态检查失败: {str(e)}',
                'file_path': str(file_path)
            }

    def get_posts(self, query='', category='', tag='', page=1, per_page=20):
        """
        获取文章列表

        Args:
            query: 搜索关键词
            category: 分类筛选
            tag: 标签筛选
            page: 页码
            per_page: 每页数量

        Returns:
            dict: 包含文章列表和分页信息
        """
        # 使用缓存或直接扫描
        if self.use_cache and self.cache_service:
            return self.cache_service.get_posts(query, category, tag, page, per_page)

        # 回退到原有的直接扫描方式
        # 获取所有文章
        all_posts = get_blog_posts(str(self.content_dir))

        # 应用搜索过滤
        if query:
            all_posts = filter_posts_by_search(all_posts, query, search_fields=['all'])

        # 按分类过滤
        if category:
            all_posts = [p for p in all_posts if category in p.categories]

        # 按标签过滤
        if tag:
            all_posts = [p for p in all_posts if tag in p.tags]

        # 计算分页
        total = len(all_posts)
        total_pages = (total + per_page - 1) // per_page
        start = (page - 1) * per_page
        end = start + per_page

        # 获取当前页文章
        page_posts = all_posts[start:end]

        # 转换为 JSON 可序列化格式
        posts_data = []
        for post in page_posts:
            # BlogPost 类已经统一处理了所有字段类型，这里直接使用即可
            posts_data.append({
                'title': post.title,
                'path': str(post.relative_path),
                'full_path': str(post.file_path),
                'date': post.date[:10] if post.date else '',  # date 已经是字符串
                'description': post.description,
                'excerpt': post.excerpt,
                'tags': post.tags,  # 已经是列表
                'categories': post.categories,  # 已经是列表
                'mod_time': datetime.fromtimestamp(post.mod_time).strftime("%Y-%m-%d %H:%M")
            })

        return {
            'posts': posts_data,
            'total': total,
            'page': page,
            'per_page': per_page,
            'total_pages': total_pages,
            'has_next': page < total_pages,
            'has_prev': page > 1
        }

    def get_all_tags(self):
        """
        获取所有标签

        Returns:
            list: 标签列表(包含文章数量)
        """
        # 使用缓存或直接扫描
        if self.use_cache and self.cache_service:
            return self.cache_service.get_all_tags()

        # 回退到原有的直接扫描方式
        all_posts = get_blog_posts(str(self.content_dir))
        tag_count = {}

        for post in all_posts:
            # BlogPost 已经保证 tags 是列表
            for tag in post.tags:
                tag_count[tag] = tag_count.get(tag, 0) + 1

        # 按文章数量排序
        tags = [{'name': tag, 'count': count} for tag, count in tag_count.items()]
        tags.sort(key=lambda x: x['count'], reverse=True)

        return tags

    def get_all_categories(self):
        """
        获取所有分类

        Returns:
            list: 分类列表(包含文章数量)
        """
        # 使用缓存或直接扫描
        if self.use_cache and self.cache_service:
            return self.cache_service.get_all_categories()

        # 回退到原有的直接扫描方式
        all_posts = get_blog_posts(str(self.content_dir))
        category_count = {}

        for post in all_posts:
            # BlogPost 已经保证 categories 是列表
            for category in post.categories:
                category_count[category] = category_count.get(category, 0) + 1

        # 按文章数量排序
        categories = [{'name': cat, 'count': count} for cat, count in category_count.items()]
        categories.sort(key=lambda x: x['count'], reverse=True)

        return categories

    def read_file(self, file_path):
        """
        读取文件内容

        Args:
            file_path: 文件路径(相对于 content 目录或绝对路径)

        Returns:
            (success, content): 成功标志和文件内容
        """
        try:
            # 处理路径
            if not Path(file_path).is_absolute():
                file_path = self.content_dir / file_path

            file_path = Path(file_path)

            # 安全检查:确保文件在 content 目录下
            if not self._is_safe_path(file_path):
                return False, "访问被拒绝:文件不在允许的目录中"

            # 检查文件是否存在
            if not file_path.exists():
                return False, f"文件不存在: {file_path}"

            # 读取文件
            with open(file_path, 'r', encoding='utf-8') as f:
                content = f.read()

            return True, content

        except Exception as e:
            return False, f"读取文件失败: {str(e)}"

    def save_file(self, file_path, content):
        """
        保存文件内容

        Args:
            file_path: 文件路径
            content: 文件内容

        Returns:
            (success, message): 成功标志和消息
        """
        try:
            # 处理路径
            if not Path(file_path).is_absolute():
                file_path = self.content_dir / file_path

            file_path = Path(file_path)

            # 安全检查
            if not self._is_safe_path(file_path):
                return False, "访问被拒绝:文件不在允许的目录中"

            # 确保父目录存在
            file_path.parent.mkdir(parents=True, exist_ok=True)

            # 保存文件
            with open(file_path, 'w', encoding='utf-8') as f:
                f.write(content)

            # 更新缓存
            if self.use_cache and self.cache_service:
                self.cache_service.invalidate_post(str(file_path))

            return True, "文件保存成功"

        except Exception as e:
            return False, f"保存文件失败: {str(e)}"

    def create_post(self, title):
        """
        创建新文章

        Args:
            title: 文章标题

        Returns:
            (success, result): 成功标志和文件路径或错误消息
        """
        try:
            # 生成文件名
            post_name = str(datetime.now().date()) + f"-{title}"
            post_name = post_name.replace(" ", "-")

            # 创建文章目录
            post_folder = self.post_dir / post_name
            post_folder.mkdir(exist_ok=True)

            # 创建文章文件
            post_file = post_folder / "index.md"

            # 生成 frontmatter（使用东八区时区）
            tz_cn = timezone(timedelta(hours=8))
            now = datetime.now(tz_cn)
            # 格式化为 RFC3339 格式，不带引号
            date_str = now.strftime('%Y-%m-%dT%H:%M:%S+08:00')

            frontmatter = {
                'title': title,
                'date': date_str,
                'draft': True,
                'categories': [],
                'tags': []
            }

            # 手动构造 frontmatter，确保日期格式正确且不加引号
            content = "---\n"
            content += f"title: {title}\n"
            content += f"date: {date_str}\n"
            content += f"draft: true\n"
            content += "categories: []\n"
            content += "tags: []\n"
            content += "---\n\n"
            content += "在这里编写你的文章内容...\n"

            with open(post_file, 'w', encoding='utf-8') as f:
                f.write(content)

            # 返回相对路径
            rel_path = post_file.relative_to(self.content_dir)

            # 更新缓存
            if self.use_cache and self.cache_service:
                self.cache_service.invalidate_post(str(post_file))

            return True, str(rel_path)

        except Exception as e:
            return False, f"创建文章失败: {str(e)}"

    def _is_safe_path(self, file_path):
        """
        检查路径是否安全(在 content 目录下)

        Args:
            file_path: 文件路径

        Returns:
            bool: 是否安全
        """
        try:
            file_path = Path(file_path).resolve()
            content_dir = self.content_dir.resolve()

            # 检查是否在 content 目录下
            return str(file_path).startswith(str(content_dir))

        except Exception:
            return False

    def _safe_file_operation(self, file_path, operation, timeout=10):
        """
        安全的文件操作，使用文件锁防止并发访问

        Args:
            file_path: 文件路径
            operation: 操作函数，接收文件对象并返回结果
            timeout: 超时时间（秒）

        Returns:
            tuple: (success, result)
        """
        start_time = time.time()

        while time.time() - start_time < timeout:
            try:
                with open(file_path, 'r+', encoding='utf-8') as f:
                    # 尝试获取文件锁
                    try:
                        fcntl.flock(f.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                    except (ImportError, AttributeError):
                        # 在 Windows 或非 Unix 系统上跳过文件锁
                        pass

                    # 执行操作
                    return operation(f)

            except (IOError, OSError) as e:
                if "Resource temporarily unavailable" in str(e) or "already locked" in str(e).lower():
                    # 文件被锁定，等待后重试
                    time.sleep(0.1)
                    continue
                else:
                    # 其他 IO 错误
                    return False, f"文件访问错误: {str(e)}"
            except Exception as e:
                return False, f"操作执行失败: {str(e)}"

        # 超时
        return False, f"无法在 {timeout} 秒内获取文件锁"

    def _validate_frontmatter(self, post):
        """
        验证文章 frontmatter

        Args:
            post: frontmatter.Post 对象

        Returns:
            tuple: (is_valid, errors)
        """
        errors = []

        # 检查必要字段
        if not post.get('title'):
            errors.append('缺少标题')

        # 检查 draft 字段类型
        draft_value = post.get('draft')
        if draft_value is not None and not isinstance(draft_value, bool):
            errors.append('draft 字段必须是布尔值')

        # 检查日期格式
        date_value = post.get('date')
        if date_value:
            try:
                # 尝试解析日期
                if isinstance(date_value, str):
                    datetime.fromisoformat(date_value.replace('Z', '+00:00'))
            except (ValueError, AttributeError):
                errors.append('日期格式无效')

        return len(errors) == 0, errors

    def _validate_file_path(self, file_path):
        """
        验证文件路径的安全性

        Args:
            file_path: 文件路径

        Returns:
            tuple: (is_valid, error_message)
        """
        try:
            path = Path(file_path)

            # 检查路径是否尝试进行目录遍历攻击
            if '..' in path.parts:
                return False, "路径包含目录遍历字符"

            # 转换为绝对路径
            if not path.is_absolute():
                path = (self.content_dir / path).resolve()
            else:
                path = path.resolve()

            # 检查是否在允许的目录内
            content_dir = self.content_dir.resolve()
            if not str(path).startswith(str(content_dir)):
                return False, "路径不在允许的内容目录内"

            # 检查文件扩展名
            allowed_extensions = {'.md', '.markdown'}
            if path.suffix.lower() not in allowed_extensions:
                return False, f"不支持的文件扩展名: {path.suffix}"

            return True, None

        except Exception as e:
            return False, f"路径验证失败: {str(e)}"

    def save_image(self, article_path, file):
        """
        保存图片到文章目录

        Args:
            article_path: 文章路径(相对于 content 目录)
            file: 上传的文件对象

        Returns:
            (success, result): 成功标志和图片URL或错误消息
        """
        try:
            # 构建文章目录路径
            if not Path(article_path).is_absolute():
                article_file = self.content_dir / article_path
            else:
                article_file = Path(article_path)

            # 获取文章所在目录
            article_dir = article_file.parent

            # 创建 pics 目录
            pics_dir = article_dir / 'pics'
            pics_dir.mkdir(exist_ok=True)

            # 生成安全的文件名
            filename = file.filename
            # 移除特殊字符
            safe_filename = "".join(c for c in filename if c.isalnum() or c in '.-_')

            # 保存文件
            file_path = pics_dir / safe_filename
            file.save(str(file_path))

            # 返回相对URL（相对于文章）
            relative_url = f"pics/{safe_filename}"

            return True, relative_url

        except Exception as e:
            return False, f"保存图片失败: {str(e)}"

    def list_images(self, article_path):
        """
        列出文章目录下的所有图片

        Args:
            article_path: 文章路径(相对于 content 目录)

        Returns:
            (success, result): 成功标志和图片列表或错误消息
        """
        try:
            # 构建文章目录路径
            if not Path(article_path).is_absolute():
                article_file = self.content_dir / article_path
            else:
                article_file = Path(article_path)

            # 获取文章所在目录
            article_dir = article_file.parent
            pics_dir = article_dir / 'pics'

            if not pics_dir.exists():
                return True, []

            # 支持的图片格式
            image_extensions = {'.png', '.jpg', '.jpeg', '.gif', '.svg', '.webp'}

            # 列出所有图片
            images = []
            for img_path in pics_dir.iterdir():
                if img_path.is_file() and img_path.suffix.lower() in image_extensions:
                    images.append({
                        'name': img_path.name,
                        'url': f"pics/{img_path.name}",
                        'size': img_path.stat().st_size,
                        'modified': img_path.stat().st_mtime
                    })

            # 按修改时间倒序排列
            images.sort(key=lambda x: x['modified'], reverse=True)

            return True, images

        except Exception as e:
            return False, f"获取图片列表失败: {str(e)}"
