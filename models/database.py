# coding: utf-8
"""
数据库模型定义
使用 SQLite 存储文章缓存数据
"""

import json
import sqlite3
import uuid
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional


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
        self._migrate_db()

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
                cover TEXT DEFAULT '',
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

        # 聊天会话表
        cursor.execute(
            """
            CREATE TABLE IF NOT EXISTS chat_sessions (
                id TEXT PRIMARY KEY,
                title TEXT NOT NULL,
                created_at REAL NOT NULL,
                updated_at REAL NOT NULL
            )
        """
        )

        # 聊天消息表
        cursor.execute(
            """
            CREATE TABLE IF NOT EXISTS chat_messages (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                session_id TEXT NOT NULL,
                role TEXT NOT NULL,
                content TEXT NOT NULL,
                message_type TEXT DEFAULT 'text',
                created_at REAL NOT NULL,
                FOREIGN KEY (session_id) REFERENCES chat_sessions(id)
            )
        """
        )

        # 文章引用关系表
        cursor.execute(
            """
            CREATE TABLE IF NOT EXISTS post_references (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                source_path TEXT NOT NULL,
                target_path TEXT NOT NULL,
                context TEXT DEFAULT '',
                UNIQUE(source_path, target_path)
            )
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_ref_source ON post_references(source_path)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_ref_target ON post_references(target_path)
        """
        )

        # 创建聊天相关索引
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_session_id ON chat_messages(session_id)
        """
        )
        cursor.execute(
            """
            CREATE INDEX IF NOT EXISTS idx_updated_at ON chat_sessions(updated_at)
        """
        )

        conn.commit()
        conn.close()

    def _migrate_db(self):
        """数据库迁移：为已有表添加新字段"""
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("PRAGMA table_info(posts)")
        columns = {row[1] for row in cursor.fetchall()}

        if "cover" not in columns:
            cursor.execute("ALTER TABLE posts ADD COLUMN cover TEXT DEFAULT ''")
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
            (file_path, relative_path, title, date, description, excerpt, cover,
             tags, categories, mod_time, cached_at)
            VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        """,
            (
                post_data["file_path"],
                post_data["relative_path"],
                post_data["title"],
                post_data.get("date", ""),
                post_data.get("description", ""),
                post_data.get("excerpt", ""),
                post_data.get("cover", ""),
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
            sql += " AND (title LIKE ? OR description LIKE ?"
            sql += " OR excerpt LIKE ? OR relative_path LIKE ?)"
            search_term = f"%{query}%"
            params.extend([search_term, search_term, search_term, search_term])

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

    def create_chat_session(self, title: str) -> Dict[str, Any]:
        """
        创建聊天会话

        Args:
            title: 会话标题

        Returns:
            会话数据字典
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        session_id = uuid.uuid4().hex
        now = datetime.now().timestamp()

        cursor.execute(
            """
            INSERT INTO chat_sessions (id, title, created_at, updated_at)
            VALUES (?, ?, ?, ?)
        """,
            (session_id, title, now, now),
        )

        conn.commit()
        conn.close()

        return {
            "id": session_id,
            "title": title,
            "created_at": now,
            "updated_at": now,
        }

    def add_chat_message(
        self,
        session_id: str,
        role: str,
        content: str,
        message_type: str = "text",
    ) -> Dict[str, Any]:
        """
        添加聊天消息

        Args:
            session_id: 会话 ID
            role: 消息角色 (user/assistant)
            content: 消息内容
            message_type: 消息类型，默认为 'text'

        Returns:
            消息数据字典
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        now = datetime.now().timestamp()

        cursor.execute(
            """
            INSERT INTO chat_messages
            (session_id, role, content, message_type, created_at)
            VALUES (?, ?, ?, ?, ?)
        """,
            (session_id, role, content, message_type, now),
        )

        message_id = cursor.lastrowid

        cursor.execute(
            """
            UPDATE chat_sessions SET updated_at = ? WHERE id = ?
        """,
            (now, session_id),
        )

        conn.commit()
        conn.close()

        return {
            "id": message_id,
            "session_id": session_id,
            "role": role,
            "content": content,
            "message_type": message_type,
            "created_at": now,
        }

    def get_chat_session(self, session_id: str) -> Optional[Dict[str, Any]]:
        """
        获取聊天会话

        Args:
            session_id: 会话 ID

        Returns:
            会话数据字典或 None
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT * FROM chat_sessions WHERE id = ?", (session_id,))
        row = cursor.fetchone()
        conn.close()

        if row:
            return dict(row)
        return None

    def list_chat_sessions(self) -> List[Dict[str, Any]]:
        """
        列出所有聊天会话

        Returns:
            会话列表，按 updated_at 降序排列
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("SELECT * FROM chat_sessions ORDER BY updated_at DESC")
        rows = cursor.fetchall()
        conn.close()

        return [dict(row) for row in rows]

    def get_chat_messages(self, session_id: str) -> List[Dict[str, Any]]:
        """
        获取聊天会话的所有消息

        Args:
            session_id: 会话 ID

        Returns:
            消息列表，按 created_at 升序排列
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute(
            "SELECT * FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC",
            (session_id,),
        )
        rows = cursor.fetchall()
        conn.close()

        return [dict(row) for row in rows]

    def delete_chat_session(self, session_id: str):
        """
        删除聊天会话及其所有消息

        Args:
            session_id: 会话 ID
        """
        conn = self._get_connection()
        cursor = conn.cursor()

        cursor.execute("DELETE FROM chat_messages WHERE session_id = ?", (session_id,))
        cursor.execute("DELETE FROM chat_sessions WHERE id = ?", (session_id,))

        conn.commit()
        conn.close()

    # ---- 引用关系 ----

    def upsert_references(self, source_path: str, refs: list):
        """
        替换某篇文章的所有引用关系

        Args:
            source_path: 源文章文件路径
            refs: [{"target_path": "...", "context": "..."}] 列表
        """
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute(
            "DELETE FROM post_references WHERE source_path = ?", (source_path,)
        )
        for ref in refs:
            cursor.execute(
                """
                INSERT OR IGNORE INTO post_references
                (source_path, target_path, context)
                VALUES (?, ?, ?)
                """,
                (source_path, ref["target_path"], ref.get("context", "")),
            )
        conn.commit()
        conn.close()

    def batch_upsert_references(self, all_refs: dict):
        """
        批量替换多篇文章的引用关系（单次事务）

        Args:
            all_refs: {source_path: [refs...], ...}
        """
        conn = self._get_connection()
        cursor = conn.cursor()
        for source_path, refs in all_refs.items():
            cursor.execute(
                "DELETE FROM post_references WHERE source_path = ?",
                (source_path,),
            )
            for ref in refs:
                cursor.execute(
                    """
                    INSERT OR IGNORE INTO post_references
                    (source_path, target_path, context)
                    VALUES (?, ?, ?)
                    """,
                    (source_path, ref["target_path"], ref.get("context", "")),
                )
        conn.commit()
        conn.close()

    def get_backlinks(self, target_path: str) -> List[Dict[str, Any]]:
        """获取指向 target_path 的所有反向引用"""
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute(
            """
            SELECT p.relative_path, p.title, pr.context
            FROM post_references pr
            JOIN posts p ON pr.source_path = p.file_path
            WHERE pr.target_path = ?
            ORDER BY p.date DESC
            """,
            (target_path,),
        )
        rows = cursor.fetchall()
        conn.close()
        return [
            {
                "path": row["relative_path"],
                "title": row["title"] or "",
                "context": row["context"] or "",
            }
            for row in rows
        ]

    def get_all_references(self) -> Dict[str, List[str]]:
        """获取所有引用关系"""
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute("SELECT source_path, target_path FROM post_references")
        rows = cursor.fetchall()
        conn.close()
        refs: Dict[str, List[str]] = {}
        for row in rows:
            refs.setdefault(row["source_path"], []).append(row["target_path"])
        return refs

    def update_chat_session_title(self, session_id: str, title: str):
        """更新聊天会话标题"""
        conn = self._get_connection()
        cursor = conn.cursor()
        cursor.execute(
            "UPDATE chat_sessions SET title = ? WHERE id = ?", (title, session_id)
        )

        conn.commit()
        conn.close()
