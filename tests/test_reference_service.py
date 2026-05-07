# coding: utf-8
"""测试 ReferenceService 路径归一化"""
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

        # 插入源文章（JOIN 需要）
        db.upsert_post(
            {
                "file_path": str(content_dir / "post/source.md"),
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
        yield ref_service, content_dir


def test_backlinks_query_normalizes_leading_slash(ref_setup):
    """查询反向链接时去掉前导 /"""
    ref_service, content_dir = ref_setup
    source = str(content_dir / "post/source.md")

    ref_service.db.upsert_references(
        source, [{"target_path": "post/target.md", "context": "引用"}]
    )

    # 带前导 / 查询
    assert len(ref_service.get_backlinks("/post/target.md")) == 1
    # 不带前导 / 查询
    assert len(ref_service.get_backlinks("post/target.md")) == 1


def test_scan_file_normalizes_leading_slash(ref_setup):
    """scan_file 提取引用时去掉前导 /"""
    ref_service, content_dir = ref_setup
    post_dir = content_dir / "post"
    post_dir.mkdir(parents=True, exist_ok=True)

    source = post_dir / "source.md"
    source.write_text(
        '{{< ref "/post/another.md" >}} {{< ref "post/normal.md" >}}',
        encoding="utf-8",
    )

    refs = ref_service.scan_file(str(source))
    assert refs[0]["target_path"] == "post/another.md"
    assert refs[1]["target_path"] == "post/normal.md"
