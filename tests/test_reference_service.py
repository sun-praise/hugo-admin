# coding: utf-8
"""测试 ReferenceService 路径归一化和解析"""
import tempfile
from pathlib import Path

import pytest

from models.database import Database
from services.reference_service import ReferenceService


@pytest.fixture
def ref_setup():
    with tempfile.TemporaryDirectory() as tmpdir:
        content_dir = Path(tmpdir)
        db = Database(str(content_dir / "test.db"))
        ref_service = ReferenceService(content_dir, db)

        # 创建目录结构和文件
        post_dir = content_dir / "post"
        post_dir.mkdir(parents=True, exist_ok=True)

        # 源文章
        source = post_dir / "source.md"
        source.write_text("", encoding="utf-8")

        # 目标文章（各种路径变体需要匹配到这个文件）
        target = post_dir / "target.md"
        target.write_text("", encoding="utf-8")

        # 插入源文章到 posts 表（JOIN 需要）
        db.upsert_post(
            {
                "file_path": str(source),
                "relative_path": "post/source.md",
                "title": "源文章",
                "date": "2026-01-01",
                "description": "",
                "excerpt": "",
                "cover": "",
                "tags": [],
                "categories": [],
                "mod_time": 1704067200.0,
            }
        )
        yield ref_service, content_dir, db


class TestScanFileNormalization:
    """测试 scan_file 对各种路径写法的归一化"""

    def test_full_relative_path(self, ref_setup):
        """完整相对路径 post/target.md 保持不变"""
        ref_service, content_dir, _ = ref_setup
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "post/target.md" >}}', encoding="utf-8")

        refs = ref_service.scan_file(str(source))
        assert refs[0]["target_path"] == "post/target.md"

    def test_leading_slash(self, ref_setup):
        """带前导 / 的路径去掉前导 /"""
        ref_service, content_dir, _ = ref_setup
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "/post/target.md" >}}', encoding="utf-8")

        refs = ref_service.scan_file(str(source))
        assert refs[0]["target_path"] == "post/target.md"

    def test_filename_only(self, ref_setup):
        """仅文件名 target.md 解析为完整相对路径"""
        ref_service, content_dir, _ = ref_setup
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "target.md" >}}', encoding="utf-8")

        refs = ref_service.scan_file(str(source))
        assert refs[0]["target_path"] == "post/target.md"

    def test_filename_not_found(self, ref_setup):
        """仅文件名但文件不存在时保持原样"""
        ref_service, content_dir, _ = ref_setup
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "nonexistent.md" >}}', encoding="utf-8")

        refs = ref_service.scan_file(str(source))
        assert refs[0]["target_path"] == "nonexistent.md"

    def test_relative_with_dot(self, ref_setup):
        """./target.md 解析为完整相对路径"""
        ref_service, content_dir, _ = ref_setup
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "./target.md" >}}', encoding="utf-8")

        refs = ref_service.scan_file(str(source))
        assert refs[0]["target_path"] == "post/target.md"


class TestBacklinksQuery:
    """测试 get_backlinks 查询时的路径匹配"""

    def test_exact_match(self, ref_setup):
        """精确路径匹配"""
        ref_service, content_dir, db = ref_setup
        source = str(content_dir / "post" / "source.md")

        db.upsert_references(
            source, [{"target_path": "post/target.md", "context": "引用"}]
        )

        assert len(ref_service.get_backlinks("post/target.md")) == 1

    def test_query_with_leading_slash(self, ref_setup):
        """查询时带前导 / 也能匹配"""
        ref_service, content_dir, db = ref_setup
        source = str(content_dir / "post" / "source.md")

        db.upsert_references(
            source, [{"target_path": "post/target.md", "context": "引用"}]
        )

        assert len(ref_service.get_backlinks("/post/target.md")) == 1

    def test_filename_only_stored(self, ref_setup):
        """存储的是完整路径，用完整路径查询能匹配"""
        ref_service, content_dir, db = ref_setup
        source = str(content_dir / "post" / "source.md")

        db.upsert_references(
            source, [{"target_path": "post/target.md", "context": "引用"}]
        )

        assert len(ref_service.get_backlinks("post/target.md")) == 1


class TestEndToEnd:
    """端到端测试：scan_all → get_backlinks，验证 JOIN 路径匹配"""

    def test_scan_all_then_get_backlinks(self, ref_setup):
        """完整流程：扫描引用 → 查询反向链接"""
        ref_service, content_dir, db = ref_setup

        # source.md 引用了 target.md
        source = content_dir / "post" / "source.md"
        source.write_text('{{< ref "post/target.md" >}}', encoding="utf-8")

        # 执行全量扫描
        ref_service.scan_all()

        # 查询 target.md 的反向链接，应该找到 source.md
        backlinks = ref_service.get_backlinks("post/target.md")
        assert len(backlinks) == 1
        assert backlinks[0]["title"] == "源文章"
        assert backlinks[0]["path"] == "post/source.md"
