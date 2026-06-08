# coding: utf-8
"""
分页测试 (issue #104)

后端分页逻辑此前已通过 `services.post_service.PostService.get_posts` 暴露，
但前端从未传递 `page` 参数，UI 也没有翻页控件。此处的测试固定住后端契约，
避免后续回归；并证明 `page` / `per_page` 在服务层能正确分页。

不分层到 HTTP：避免把 Flask app 的整条导入链拖进测试套件。
"""
import sys
import tempfile
from pathlib import Path

import frontmatter
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from services.post_service import PostService  # noqa: E402


def _make_post(path: Path, title: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fm = frontmatter.Post(
        f"# {title}\n",
        title=title,
        date="2025-01-01",
        draft=False,
        categories=["demo"],
        tags=["pagination"],
    )
    path.write_text(frontmatter.dumps(fm), encoding="utf-8")


@pytest.fixture
def temp_content_dir():
    with tempfile.TemporaryDirectory() as tmp:
        content_dir = Path(tmp) / "content"
        content_dir.mkdir()
        yield content_dir


@pytest.fixture
def post_service(temp_content_dir):
    return PostService(temp_content_dir, use_cache=False)


def _seed(content_dir: Path, n: int) -> None:
    for i in range(n):
        _make_post(content_dir / f"post/p{i:02d}.md", f"P{i:02d}")


class TestPostsPagination:
    """`PostService.get_posts` 的分页契约"""

    def test_default_returns_first_window(self, post_service, temp_content_dir):
        _seed(temp_content_dir, 25)
        data = post_service.get_posts(page=1, per_page=10)
        assert data["total"] == 25
        assert data["page"] == 1
        assert data["per_page"] == 10
        assert data["total_pages"] == 3
        assert data["has_next"] is True
        assert data["has_prev"] is False
        assert len(data["posts"]) == 10

    def test_explicit_page_2_returns_next_window(self, post_service, temp_content_dir):
        _seed(temp_content_dir, 25)
        page1 = post_service.get_posts(page=1, per_page=10)
        page2 = post_service.get_posts(page=2, per_page=10)
        assert page2["page"] == 2
        assert page2["has_next"] is True
        assert page2["has_prev"] is True
        assert len(page2["posts"]) == 10
        # 翻页结果应与第 1 页互不重叠。
        first_paths = {p["path"] for p in page1["posts"]}
        second_paths = {p["path"] for p in page2["posts"]}
        assert first_paths.isdisjoint(second_paths)

    def test_last_page_has_partial_window(self, post_service, temp_content_dir):
        _seed(temp_content_dir, 25)
        data = post_service.get_posts(page=3, per_page=10)
        assert data["page"] == 3
        assert data["has_next"] is False
        assert data["has_prev"] is True
        assert len(data["posts"]) == 5

    def test_total_pages_zero_for_empty(self, post_service, temp_content_dir):
        data = post_service.get_posts(page=1, per_page=20)
        assert data["total"] == 0
        assert data["page"] == 1
        assert data["total_pages"] == 0
        assert data["has_next"] is False
        assert data["has_prev"] is False
        assert data["posts"] == []
