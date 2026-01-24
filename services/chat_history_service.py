# coding: utf-8
"""
聊天历史服务
负责聊天会话和消息的管理，包装数据库操作并添加业务逻辑
"""

from typing import Dict, List, Any, Optional
from datetime import datetime, timezone, timedelta
import uuid
import re

from models.database import Database


class ChatHistoryService:
    """聊天历史管理服务"""

    def __init__(self, db: Database):
        """
        初始化聊天历史服务

        Args:
            db: Database 实例
        """
        self.db = db

    def create_session(self, title: Optional[str] = None) -> Dict[str, Any]:
        """
        创建新的聊天会话

        Args:
            title: 会话标题，如果不提供则为 None，后续可从第一条用户消息自动生成

        Returns:
            包含 session_id 和其他会话信息的字典
        """
        session_title = title if title else "新对话"
        session = self.db.create_chat_session(session_title)

        return {
            "session_id": session["id"],
            "title": session["title"],
            "created_at": session["created_at"],
            "updated_at": session["updated_at"],
            "message_count": 0,
        }

    def add_message(
        self,
        session_id: str,
        role: str,
        content: str,
        message_type: str = "text",
    ) -> Dict[str, Any]:
        """
        向会话中添加消息

        Args:
            session_id: 会话 ID
            role: 消息角色 (user, assistant, system 等)
            content: 消息内容
            message_type: 消息类型，默认为 "text"

        Returns:
            包含消息信息的字典
        """
        now = datetime.now(timezone(timedelta(hours=8))).timestamp()

        msg = self.db.add_chat_message(
            session_id=session_id,
            role=role,
            content=content,
            message_type=message_type,
        )

        if role == "user":
            session = self.db.get_chat_session(session_id)
            if session and session.get("title") == "新对话":
                auto_title = self._generate_title_from_content(content)
                self.db.update_chat_session_title(session_id, auto_title)

        return {
            "message_id": msg["id"],
            "session_id": session_id,
            "role": role,
            "content": content,
            "message_type": message_type,
            "created_at": msg["created_at"],
        }

    def get_session(self, session_id: str) -> Optional[Dict[str, Any]]:
        """
        获取会话及其所有消息

        Args:
            session_id: 会话 ID

        Returns:
            包含会话信息和消息列表的字典，如果会话不存在则返回 None
        """
        session = self.db.get_chat_session(session_id)
        if not session:
            return None

        messages = self.db.get_chat_messages(session_id)

        return {
            "session_id": session_id,
            "title": session.get("title", ""),
            "created_at": session.get("created_at"),
            "updated_at": session.get("updated_at"),
            "message_count": len(messages),
            "messages": messages,
        }

    def list_sessions(self, limit: int = 20) -> List[Dict[str, Any]]:
        """
        列出最近的聊天会话

        Args:
            limit: 返回的最大会话数，默认为 20

        Returns:
            会话列表，按 updated_at 倒序排列
        """
        all_sessions = self.db.list_chat_sessions()
        sessions = all_sessions[:limit]

        result = []
        for session in sessions:
            messages = self.db.get_chat_messages(session["id"])
            result.append(
                {
                    "session_id": session["id"],
                    "title": session.get("title", ""),
                    "created_at": session.get("created_at"),
                    "updated_at": session.get("updated_at"),
                    "message_count": len(messages),
                }
            )

        return result

    def delete_session(self, session_id: str) -> bool:
        """
        删除聊天会话及其所有消息

        Args:
            session_id: 会话 ID

        Returns:
            删除是否成功
        """
        try:
            self.db.delete_chat_session(session_id)
            return True
        except Exception:
            return False

    def update_session_title(self, session_id: str, title: str) -> bool:
        """
        更新会话标题

        Args:
            session_id: 会话 ID
            title: 新标题

        Returns:
            更新是否成功
        """
        try:
            self.db.update_chat_session_title(session_id, title)
            return True
        except Exception:
            return False

    def _generate_title_from_content(self, content: str, max_length: int = 50) -> str:
        """
        从消息内容生成标题

        Args:
            content: 消息内容
            max_length: 标题最大长度

        Returns:
            生成的标题
        """
        # 移除特殊字符和多余空格
        title = re.sub(r"\s+", " ", content.strip())

        # 截断到指定长度
        if len(title) > max_length:
            title = title[: max_length - 3] + "..."

        return title if title else "新对话"
