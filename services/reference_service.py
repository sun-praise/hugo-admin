# coding: utf-8
"""
文章引用关系服务
扫描文章中的 Hugo ref shortcode，构建双向引用索引
"""

import re
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from models.database import Database

REF_PATTERN = re.compile(r'\{\{<\s*ref\s+"([^"]+)"\s*>\}\}', re.DOTALL)


class ReferenceService:
    def __init__(self, content_dir, db: "Database"):
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
            target = m.group(1).lstrip("/")
            # 解析相对路径：尝试找到实际文件的完整相对路径
            target = self._resolve_target(target, path)
            # 提取匹配位置前后的上下文（最多 60 字符）
            start = max(0, m.start() - 30)
            end = min(len(text), m.end() + 30)
            ctx = text[start:end].replace("\n", " ").strip()
            refs.append({"target_path": target, "context": ctx})
        return refs

    def _resolve_target(self, target: str, source_path: Path) -> str:
        """将 target 解析为相对于 content_dir 的完整路径"""
        # 已经是包含目录的路径，直接返回
        if "/" in target and not target.startswith("./"):
            return target
        # 按优先级查找：1) 相对于源文件目录 2) 全局搜索
        candidates = []
        if target.startswith("./"):
            candidates.append(source_path.parent / target)
        else:
            # 先尝试同目录
            candidates.append(source_path.parent / target)
            # 再在 content_dir 下全局搜索文件名
            for found in self.content_dir.rglob(target):
                candidates.append(found)
        for candidate in candidates:
            if candidate.exists() and candidate.is_file():
                try:
                    return str(candidate.relative_to(self.content_dir))
                except ValueError:
                    continue
        return target

    def scan_all(self):
        """扫描 content 目录下所有 .md 文件，重建引用索引"""
        if not self.content_dir.exists():
            return

        all_refs = {}
        for md_file in self.content_dir.rglob("*.md"):
            all_refs[str(md_file)] = self.scan_file(str(md_file))
        self.db.batch_upsert_references(all_refs)

    def update_file(self, file_path: str):
        """增量更新单个文件的引用"""
        refs = self.scan_file(file_path)
        self.db.upsert_references(file_path, refs)

    def get_backlinks(self, file_path: str):
        """获取反向链接（哪些文章引用了当前文章）"""
        return self.db.get_backlinks(file_path.lstrip("/"))

    def search_posts(self, query: str):
        """模糊搜索文章（标题、路径、摘要、描述）"""
        results = self.db.search_posts(query)
        return [{"path": p["relative_path"], "title": p["title"]} for p in results]
