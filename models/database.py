# coding: utf-8
"""
数据库模型定义
使用 SQLite 存储文章缓存数据
"""
import sqlite3
from pathlib import Path
from typing import Optional, List, Dict, Any
from datetime import datetime
import json


class Database:
    """数据库管理类"""

    def __init__(self, db_path: str):
        """
        初始化数据库连接

        Args:
            db_path: 数据库文件路径
        """
        self.db_path = Path(db_path)
        self.db_path.parent.mkdir(parents=True, exist_ok=True)
        self._init_db()

    def _get_connection(self):
        """获取数据库连接"""
        conn = sqlite3.connect(str(self.db_path))
        conn.row_factory = sqlite3.Row  # 使用 Row 工厂，可以通过列名访问
        return conn

    def _init_db(self):
        """初始化数据库表"""
        conn = self._get_connection()
        cursor = conn.cursor()

        # 文章表
        cursor.execute(
            """
            CREATE TABLE IF NOT EXISTS posts (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                file_path TEXT UNIQUE NOT NULL,
                relative_path TEXT NOT NULL,
                title TEXT NOT NULL,
                date TEXT,
                description TEXT,
                excerpt TEXT,
                tags TEXT,
                categories TEXT,
                mod_time REAL NOT NULL,
                cached_at REAL NOT NULL
            )
        """
        )

        # 创建索引
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_file_path ON posts(file_path)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_mod_time ON posts(mod_time)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_date ON posts(date)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_tags ON posts(tags)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_categories ON posts(categories)
        """
        )

        conn.commit()
        conn.close()

    def get_post(self, file_path: str) -> Optional[Dict[str, Any]]:
        """
        获取单个文章

        Args:
            file_path: 文件路径

        Returns:
            文章数据字典或 None
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT * FROM posts WHERE file_path = ?", (file_path,))
        row = cursor.fetchone()
        conn.close()

        if row:
            return self._row_to_dict(row)
        return None

    def upsert_post(self, post_data: Dict[str, Any]):
        """
        插入或更新文章

        Args:
            post_data: 文章数据字典
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        # 将列表转换为 JSON 字符串存储
        tags_json = json.dumps(post_data.get("tags", []), ensure_ascii=False)
        categories_json = json.dumps(
            post_data.get("categories", []), ensure_ascii=False
        )

        cursor.execute(
            """
            INSERT OR REPLACE INTO posts
            (file_path, relative_path, title, date, description, excerpt,
             tags, categories, mod_time, cached_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
            (
                post_data["file_path"],
                post_data["relative_path"],
                post_data["title"],
                post_data.get("date", ""),
                post_data.get("description", ""),
                post_data.get("excerpt", ""),
                tags_json,
                categories_json,
                post_data["mod_time"],
                datetime.now().timestamp(),
            ),
        )

        conn.commit()
        conn.close()

    def delete_post(self, file_path: str):
        """
        删除文章

        Args:
            file_path: 文件路径
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("DELETE FROM posts WHERE file_path = ?", (file_path,))

        conn.commit()
        conn.close()

    def get_all_posts(self, order_by: str = "date DESC") -> List[Dict[str, Any]]:
        """
        获取所有文章

        Args:
            order_by: 排序方式

        Returns:
            文章列表
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute(f"SELECT * FROM posts ORDER BY {order_by}")
        rows = cursor.fetchall()
        conn.close()

        return [self._row_to_dict(row) for row in rows]

    def search_posts(
        self, query: str, category: str = "", tag: str = ""
    ) -> List[Dict[str, Any]]:
        """
        搜索文章

        Args:
            query: 搜索关键词
            category: 分类筛选
            tag: 标签筛选

        Returns:
            文章列表
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        sql = "SELECT * FROM posts WHERE 1=1"
        params = []

        if query:
            sql += " AND (title LIKE ? OR description LIKE ? OR excerpt LIKE ?)"
            search_term = f"%{query}%"
            params.extend([search_term, search_term, search_term])

        if category:
            sql += " AND categories LIKE ?"
            params.append(f'%"{category}"%')

        if tag:
            sql += " AND tags LIKE ?"
            params.append(f'%"{tag}"%')

        sql += " ORDER BY date DESC"

        cursor.execute(sql, params)
        rows = cursor.fetchall()
        conn.close()

        return [self._row_to_dict(row) for row in rows]

    def get_all_tags(self) -> List[Dict[str, Any]]:
        """
        获取所有标签及其计数

        Returns:
            标签列表 [{'name': 'tag', 'count': 10}, ...]
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT tags FROM posts")
        rows = cursor.fetchall()
        conn.close()

        # 统计标签
        tag_count = {}
        for row in rows:
            tags = json.loads(row["tags"])
            if tags is not None:
                for tag in tags:
                    tag_count[tag] = tag_count.get(tag, 0) + 1

        # 按数量排序
        tags = [{"name": tag, "count": count} for tag, count in tag_count.items()]
        tags.sort(key=lambda x: x["count"], reverse=True)

        return tags

    def get_all_categories(self) -> List[Dict[str, Any]]:
        """
        获取所有分类及其计数

        Returns:
            分类列表 [{'name': 'category', 'count': 10}, ...]
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT categories FROM posts")
        rows = cursor.fetchall()
        conn.close()

        # 统计分类
        category_count = {}
        for row in rows:
            categories = json.loads(row["categories"])
            if categories is not None:
                for category in categories:
                    category_count[category] = category_count.get(category, 0) + 1

        # 按数量排序
        categories = [
            {"name": cat, "count": count} for cat, count in category_count.items()
        ]
        categories.sort(key=lambda x: x["count"], reverse=True)

        return categories

    def get_all_file_paths(self) -> List[str]:
        """
        获取所有文件路径

        Returns:
            文件路径列表
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT file_path FROM posts")
        rows = cursor.fetchall()
        conn.close()

        return [row["file_path"] for row in rows]

    def _row_to_dict(self, row: sqlite3.Row) -> Dict[str, Any]:
        """
        将数据库行转换为字典

        Args:
            row: 数据库行

        Returns:
            字典
        """
        data = dict(row)
        # 将 JSON 字符串转换回列表
        data["tags"] = json.loads(data["tags"])
        data["categories"] = json.loads(data["categories"])
        return data
