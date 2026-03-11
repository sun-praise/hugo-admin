# coding: utf-8
"""
EmailService 模板变量兼容测试
"""

import sys
import types

sys.modules.setdefault("feedparser", types.SimpleNamespace())

from services.email_service import EmailService


def test_normalize_unsubscribe_url():
    raw = '<a href="{{ .UnsubscribeURL }}">取消订阅</a>'
    normalized = EmailService.normalize_listmonk_template_vars(raw)
    assert normalized == '<a href="{{ UnsubscribeURL }}">取消订阅</a>'


def test_normalize_track_link():
    raw = '<a href="{{ .TrackLink "https://svtter.cn" }}">Read</a>'
    normalized = EmailService.normalize_listmonk_template_vars(raw)
    assert normalized == '<a href="{{ TrackLink "https://svtter.cn" }}">Read</a>'
