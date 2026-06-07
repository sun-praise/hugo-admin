# coding: utf-8
"""
EmailService get_post_by_url URL 匹配测试
覆盖: 完整 URL、path-only、裸 slug、index.html 后缀、www 变体、HTTP/HTTPS、多匹配兜底
"""


from unittest.mock import MagicMock, patch

import pytest

from services.email_service import (
    EmailService,
    _normalize_url_for_match,
    _strip_html_suffix,
)

# ── _strip_html_suffix 单元测试 ──


@pytest.mark.parametrize(
    "path, expected",
    [
        ("/p/post-name", "/p/post-name"),
        ("/p/post-name/index.html", "/p/post-name"),
        ("/p/post-name/index.htm", "/p/post-name"),
        ("/p/post-name.html", "/p/post-name"),
        ("/p/post-name.htm", "/p/post-name"),
        ("/p/post-name/", "/p/post-name/"),  # trailing slash NOT stripped
    ],
)
def test_strip_html_suffix(path, expected):
    assert _strip_html_suffix(path) == expected


# ── _normalize_url_for_match 单元测试 ──


@pytest.mark.parametrize(
    "raw_url, expected",
    [
        # 裸 slug → 拒绝
        ("正则表达式", None),
        ("", None),
        ("   ", None),
        # path-only
        ("/p/post-name/", ("", "/p/post-name")),
        ("/p/post-name", ("", "/p/post-name")),
        ("/p/post-name/index.html", ("", "/p/post-name")),
        ("/p/post-name.html", ("", "/p/post-name")),
        # 完整 URL
        (
            "https://svtter.cn/p/post-name/",
            ("svtter.cn", "/p/post-name"),
        ),
        (
            "https://www.svtter.cn/p/post-name/",
            ("svtter.cn", "/p/post-name"),
        ),
        (
            "http://svtter.cn/p/post-name/",
            ("svtter.cn", "/p/post-name"),
        ),
        (
            "https://svtter.cn/p/post-name/index.html",
            ("svtter.cn", "/p/post-name"),
        ),
        # 无 scheme，有 domain 的输入
        (
            "svtter.cn/p/post-name/",
            ("svtter.cn", "/p/post-name"),
        ),
    ],
)
def test_normalize_url_for_match(raw_url, expected):
    assert _normalize_url_for_match(raw_url) == expected


# ── get_post_by_url 集成测试（mock feedparser） ──


def _make_entry(title, link):
    """构造一个 mock feedparser entry"""
    entry = MagicMock()
    entry.title = title
    entry.link = link
    entry.published = "2025-01-01"
    entry.description = ""
    entry.summary = ""
    entry.tags = []
    return entry


def _make_service(rss_url="https://svtter.cn/index.xml"):
    """构造一个 EmailService 实例（跳过真实初始化）"""
    svc = EmailService.__new__(EmailService)
    svc.rss_url = rss_url
    return svc


# 模拟 RSS 条目列表
ENTRIES = [
    _make_entry("正则表达式", "https://svtter.cn/p/正则表达式/"),
    _make_entry("xx正则表达式", "https://svtter.cn/p/xx正则表达式/"),
    _make_entry("我的正则表达式笔记", "https://svtter.cn/p/我-的-正则表达式-笔记/"),
    _make_entry("Clean URL article", "https://svtter.cn/post/hello-world/"),
    _make_entry("Index HTML article", "https://svtter.cn/p/with-index/index.html"),
]


@pytest.fixture
def mock_feed():
    """返回一个 mock feed，包含 ENTRYIES"""
    feed = MagicMock()
    feed.entries = ENTRIES
    feed.bozo = False
    return feed


@pytest.mark.parametrize(
    "input_url, expected_title",
    [
        # 完整 URL — 精确匹配
        ("https://svtter.cn/p/正则表达式/", "正则表达式"),
        # path-only
        ("/p/正则表达式/", "正则表达式"),
        ("/p/正则表达式", "正则表达式"),
        # 裸 slug → 拒绝
        ("正则表达式", None),
        # index.html / .html 后缀
        ("https://svtter.cn/p/正则表达式/index.html", "正则表达式"),
        ("https://svtter.cn/p/with-index/index.html", "Index HTML article"),
        # www 变体
        ("https://www.svtter.cn/p/正则表达式/", "正则表达式"),
        # HTTP scheme（非 HTTPS）
        ("http://svtter.cn/p/正则表达式/", "正则表达式"),
        # 不同文章 path-only
        ("/post/hello-world/", "Clean URL article"),
        ("/post/hello-world", "Clean URL article"),
        # 无 scheme + domain
        ("svtter.cn/p/正则表达式/", "正则表达式"),
        # 无匹配
        ("https://svtter.cn/p/nonexistent/", None),
        ("/p/nonexistent/", None),
    ],
)
def test_get_post_by_url(input_url, expected_title, mock_feed):
    svc = _make_service()
    with patch("feedparser.parse", return_value=mock_feed):
        result = svc.get_post_by_url(input_url)

    if expected_title is None:
        assert result is None, f"expected None for {input_url!r}, got {result}"
    else:
        assert (
            result is not None
        ), f"expected {expected_title!r} for {input_url!r}, got None"
        assert result["title"] == expected_title, f"wrong match for {input_url!r}"


def test_get_post_by_url_multiple_matches_returns_none():
    """当多个条目匹配同一路径时，应返回 None（要求用户更精确）"""
    dup_entries = [
        _make_entry("Article A", "https://svtter.cn/p/same-path/"),
        _make_entry("Article B", "https://svtter.cn/p/same-path/"),
    ]
    feed = MagicMock()
    feed.entries = dup_entries
    feed.bozo = False

    svc = _make_service()
    with patch("feedparser.parse", return_value=feed):
        result = svc.get_post_by_url("/p/same-path/")
    assert result is None


def test_get_post_by_url_empty_feed_returns_none():
    feed = MagicMock()
    feed.entries = []
    feed.bozo = False

    svc = _make_service()
    with patch("feedparser.parse", return_value=feed):
        result = svc.get_post_by_url("/p/anything/")
    assert result is None
