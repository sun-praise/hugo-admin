# coding: utf-8
"""
文章引用关系服务
扫描文章中的 Hugo ref shortcode，构建双向引用索引
"""

import re
from pathlib import Path

REF_PATTERN = re.compile(r'\{\{<\s*ref\s+"([^"]+)"\s*>\}\}', re.DOTALL)


class ReferenceService:
    def __init__(self, content_dir, db):
        self.content_dir = Path(content_dir)
        self.db = db

    def scan_file(self, file_path: str) -> list:
        """扫描单个文件的引用，返回 [{target_path, context}]"""
        path = Path(file_path)
        if not path.is_absolute():
            path = self.content_dir / path

        if not path.exists():
            return []

        try:
            text = path.read_text(encoding="utf-8")
        except OSError:
            return []

        refs = []
        for m in REF_PATTERN.finditer(text):
            target = m.group(1)
            # 提取匹配位置前后的上下文（最多 60 字符）
            start = max(0, m.start() - 30)
            end = min(len(text), m.end() + 30)
            ctx = text[start:end].replace("\n", " ").strip()
            refs.append({"target_path": target, "context": ctx})
        return refs

    def scan_all(self):
        """扫描 content/post 下所有 .md 文件，重建引用索引"""
        post_dir = self.content_dir / "post"
        if not post_dir.exists():
            return

        for md_file in post_dir.rglob("*.md"):
            if md_file.is_dir():
                continue
            refs = self.scan_file(str(md_file))
            self.db.upsert_references(str(md_file), refs)

    def update_file(self, file_path: str):
        """增量更新单个文件的引用"""
        refs = self.scan_file(file_path)
        self.db.upsert_references(file_path, refs)

    def get_backlinks(self, file_path: str):
        """获取反向链接（哪些文章引用了当前文章）"""
        # file_path 可能是 absolute 或 relative
        return self.db.get_backlinks(file_path)

    def search_posts(self, query: str):
        """模糊搜索文章（标题、路径、摘要、描述）"""
        all_posts = self.db.get_all_posts(order_by="title ASC")
        q = query.lower()
        return [
            {"path": p["relative_path"], "title": p["title"]}
            for p in all_posts
            if q in p["title"].lower()
            or q in p["relative_path"].lower()
            or q in (p.get("excerpt") or "").lower()
            or q in (p.get("description") or "").lower()
        ]
