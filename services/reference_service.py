# coding: utf-8
"""
文章引用关系服务
扫描文章中的 Hugo ref shortcode，构建双向引用索引
"""

import re
from pathlib import Path

REF_PATTERN = re.compile(r'\{\{<\s*ref\s+"([^"]+)"\s*>\}\}', re.DOTALL)


class ReferenceService:
    def __init__(self, content_dir, db: "Database"):  # noqa: F821
        self.content_dir = Path(content_dir)
        if db is None:
            raise ValueError("ReferenceService requires a valid database instance")
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
        """扫描 content 目录下所有 .md 文件，重建引用索引"""
        if not self.content_dir.exists():
            return

        for md_file in self.content_dir.rglob("*.md"):
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
        results = self.db.search_posts(query)
        return [{"path": p["relative_path"], "title": p["title"]} for p in results]
