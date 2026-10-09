# coding: utf-8
"""Posts 只读 API 的 HTTP 契约测试。

fixture 文件在 setup 时写入 ``contracts/fixtures/posts/``（内容固定，
Go 侧契约回放读取同一批文件构造 content 目录），并 monkeypatch
``registry.post_service`` 指向该目录，避免依赖真实博客仓库。
``RECORD_CONTRACT=1`` 时可把请求/响应录制进契约样本库。
"""

import shutil
import sys
from pathlib import Path

import frontmatter
import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

import app as app_module  # noqa: E402
from services.post_service import PostService  # noqa: E402

FIXTURE_DIR = (
    Path(__file__).resolve().parent.parent / "contracts" / "fixtures" / "posts"
)

# (相对路径, 标题, 日期, 分类, 标签, 正文) —— 与 Go 契约回放共享，勿随意改动
FIXTURES = [
    (
        "post/go-01.md",
        "Go 重写之一",
        "2026-03-01",
        ["go"],
        ["重构", "go"],
        "这是第一篇，讲迁移。",
    ),
    (
        "post/go-02.md",
        "Go 重写之二",
        "2026-03-05",
        ["go"],
        ["重构"],
        "第二篇，讲契约。",
    ),
    (
        "post/flask-01.md",
        "Flask 旧事",
        "2025-12-01",
        ["python"],
        ["flask"],
        "旧服务回忆。",
    ),
    ("post/no-date.md", "无日期文章", "", ["misc"], [], "没有日期的文章。"),
]


def _write_fixtures():
    if FIXTURE_DIR.exists():
        shutil.rmtree(FIXTURE_DIR)
    FIXTURE_DIR.mkdir(parents=True)
    for rel, title, date, categories, tags, body in FIXTURES:
        fm = frontmatter.Post(
            f"# {title}\n\n{body}\n",
            title=title,
            date=date,
            draft=False,
            categories=categories,
            tags=tags,
        )
        path = FIXTURE_DIR / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(frontmatter.dumps(fm), encoding="utf-8")


@pytest.fixture
def posts_client(auth_store, login):
    _write_fixtures()
    original = app_module.registry.post_service
    app_module.registry.post_service = PostService(str(FIXTURE_DIR), use_cache=False)
    client = app_module.app.test_client()
    login(client)
    try:
        yield client
    finally:
        app_module.registry.post_service = original


class TestPostsHTTPAPI:
    def test_list_sorted_by_date_desc(self, posts_client):
        resp = posts_client.get("/api/posts")
        assert resp.status_code == 200
        data = resp.get_json()
        assert data["total"] == 4
        assert data["page"] == 1 and data["per_page"] == 20
        assert data["total_pages"] == 1
        assert data["has_next"] is False and data["has_prev"] is False
        titles = [p["title"] for p in data["posts"]]
        assert titles == ["Go 重写之二", "Go 重写之一", "Flask 旧事", "无日期文章"]

    def test_pagination_params(self, posts_client):
        data = posts_client.get("/api/posts?per_page=2&page=2").get_json()
        assert data["total"] == 4
        assert data["total_pages"] == 2
        assert data["has_next"] is False and data["has_prev"] is True
        assert [p["title"] for p in data["posts"]] == ["Flask 旧事", "无日期文章"]

    def test_filter_by_tag(self, posts_client):
        data = posts_client.get("/api/posts?tag=重构").get_json()
        assert data["total"] == 2

    def test_filter_by_category(self, posts_client):
        data = posts_client.get("/api/posts?category=go").get_json()
        assert data["total"] == 2

    def test_search_query(self, posts_client):
        data = posts_client.get("/api/posts?q=契约").get_json()
        assert data["total"] == 1
        assert data["posts"][0]["title"] == "Go 重写之二"

    def test_tags_aggregate(self, posts_client):
        data = posts_client.get("/api/posts/tags").get_json()
        assert data["tags"] == [
            {"name": "重构", "count": 2},
            {"name": "go", "count": 1},
            {"name": "flask", "count": 1},
        ]

    def test_categories_aggregate(self, posts_client):
        data = posts_client.get("/api/posts/categories").get_json()
        assert data["categories"] == [
            {"name": "go", "count": 2},
            {"name": "python", "count": 1},
            {"name": "misc", "count": 1},
        ]
