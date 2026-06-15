# coding: utf-8
"""
Unit tests for ``AIService.quick_rewrite``.

The Claude Agent SDK is mocked at the ``ClaudeSDKClient`` boundary so the
tests don't hit the network and don't depend on a real LLM.

These tests are written as plain sync functions that drive an async coroutine
via ``asyncio.run(...)``.  The project pins ``pytest==7.4.3`` and does not
depend on ``pytest-asyncio``; using ``@pytest.mark.asyncio`` would require
adding a new test-time dependency just to satisfy pytest's strict-markers
check.  See the route-level tests in ``test_inline_edit_api.py`` for the
end-to-end behavior; the tests here cover ``quick_rewrite`` directly.
"""

import asyncio
import sys
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

sys.path.insert(0, str(Path(__file__).parent.parent))

from claude_agent_sdk import AssistantMessage, TextBlock

from services.ai_service import (
    AIService,
    InlineEditEmptyResultError,
    InlineEditTimeoutError,
)

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _assistant_message(text: str) -> AssistantMessage:
    block = MagicMock(spec=TextBlock)
    block.text = text
    msg = MagicMock(spec=AssistantMessage)
    msg.content = [block]
    return msg


class _StubAsyncIter:
    """Minimal async iterator over a fixed list of messages."""

    def __init__(self, items):
        self._items = list(items)
        self._i = 0

    def __aiter__(self):
        return self

    async def __anext__(self):
        if self._i >= len(self._items):
            raise StopAsyncIteration
        item = self._items[self._i]
        self._i += 1
        return item


def _make_service(enabled: bool = True) -> AIService:
    """Build an AIService without running __init__'s network-touching code."""
    svc = AIService.__new__(AIService)
    svc.enabled = enabled
    svc.model_name = "test-model"
    svc.mcp_server = None
    svc.options = None
    svc.deps = None
    return svc


def _patch_client(svc, receive_iter):
    """Patch ``ClaudeSDKClient`` so ``svc.quick_rewrite`` sees a fake client.

    Returns the ``_patcher`` context manager so the caller can use it via
    ``with _patch_client(...) as ...`` to keep mocking scoped to the test.
    """
    mock_client = MagicMock()
    mock_client.query = AsyncMock()
    mock_client.receive_response = MagicMock(return_value=receive_iter)
    return patch(
        "services.ai_service.ClaudeSDKClient",
        return_value=MagicMock(
            __aenter__=AsyncMock(return_value=mock_client),
            __aexit__=AsyncMock(return_value=False),
        ),
    )


# ---------------------------------------------------------------------------
# quick_rewrite
# ---------------------------------------------------------------------------


class TestQuickRewrite:
    def test_returns_aggregated_text(self):
        svc = _make_service()
        with _patch_client(
            svc,
            _StubAsyncIter([_assistant_message("hello "), _assistant_message("world")]),
        ):
            result = asyncio.run(svc.quick_rewrite("sys", "usr"))

        assert result == "hello world"

    def test_raises_on_empty_result(self):
        svc = _make_service()
        with _patch_client(svc, _StubAsyncIter([])):
            with pytest.raises(InlineEditEmptyResultError):
                asyncio.run(svc.quick_rewrite("sys", "usr"))

    def test_raises_on_whitespace_only_result(self):
        svc = _make_service()
        with _patch_client(svc, _StubAsyncIter([_assistant_message("   \n\t  ")])):
            with pytest.raises(InlineEditEmptyResultError):
                asyncio.run(svc.quick_rewrite("sys", "usr"))

    def test_raises_on_timeout(self):
        svc = _make_service()

        async def _slow_iter():
            await asyncio.sleep(0.5)
            yield _assistant_message("never")

        with _patch_client(svc, _slow_iter()):
            with pytest.raises(InlineEditTimeoutError):
                asyncio.run(svc.quick_rewrite("sys", "usr", timeout_s=0.1))

    def test_raises_when_disabled(self):
        svc = _make_service(enabled=False)
        with pytest.raises(RuntimeError):
            asyncio.run(svc.quick_rewrite("sys", "usr"))

    def test_ignores_non_text_blocks(self):
        svc = _make_service()

        text_block = MagicMock(spec=TextBlock)
        text_block.text = "kept"
        other_block = MagicMock()
        other_block.text = None  # simulate non-text / empty block
        msg = MagicMock(spec=AssistantMessage)
        msg.content = [text_block, other_block]

        with _patch_client(svc, _StubAsyncIter([msg])):
            result = asyncio.run(svc.quick_rewrite("sys", "usr"))

        assert result == "kept"


# ---------------------------------------------------------------------------
# _build_quick_rewrite_options
# ---------------------------------------------------------------------------


class TestBuildQuickRewriteOptions:
    def test_disables_tools(self):
        svc = _make_service()
        opts = svc._build_quick_rewrite_options("system prompt")
        assert opts.allowed_tools == []
        assert opts.include_partial_messages is False
        assert opts.system_prompt == "system prompt"
        assert opts.model == "test-model"

    def test_raises_when_disabled(self):
        svc = _make_service(enabled=False)
        with pytest.raises(RuntimeError):
            svc._build_quick_rewrite_options("system prompt")
