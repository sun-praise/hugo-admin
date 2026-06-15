# coding: utf-8
"""
Unit tests for ``AIService.quick_rewrite``.

The Claude Agent SDK is mocked at the ``ClaudeSDKClient`` boundary so the
tests don't hit the network and don't depend on a real LLM.
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


# ---------------------------------------------------------------------------
# quick_rewrite
# ---------------------------------------------------------------------------


class TestQuickRewrite:
    @pytest.mark.asyncio
    async def test_returns_aggregated_text(self):
        svc = _make_service()

        mock_client = MagicMock()
        mock_client.query = AsyncMock()
        mock_client.receive_response = MagicMock(
            return_value=_StubAsyncIter(
                [_assistant_message("hello "), _assistant_message("world")]
            )
        )

        with patch(
            "services.ai_service.ClaudeSDKClient",
            return_value=MagicMock(
                __aenter__=AsyncMock(return_value=mock_client),
                __aexit__=AsyncMock(return_value=False),
            ),
        ):
            result = await svc.quick_rewrite("sys", "usr")

        assert result == "hello world"
        mock_client.query.assert_awaited_once_with("usr")

    @pytest.mark.asyncio
    async def test_raises_on_empty_result(self):
        svc = _make_service()

        mock_client = MagicMock()
        mock_client.query = AsyncMock()
        mock_client.receive_response = MagicMock(return_value=_StubAsyncIter([]))

        with patch(
            "services.ai_service.ClaudeSDKClient",
            return_value=MagicMock(
                __aenter__=AsyncMock(return_value=mock_client),
                __aexit__=AsyncMock(return_value=False),
            ),
        ):
            with pytest.raises(InlineEditEmptyResultError):
                await svc.quick_rewrite("sys", "usr")

    @pytest.mark.asyncio
    async def test_raises_on_whitespace_only_result(self):
        svc = _make_service()

        mock_client = MagicMock()
        mock_client.query = AsyncMock()
        mock_client.receive_response = MagicMock(
            return_value=_StubAsyncIter([_assistant_message("   \n\t  ")])
        )

        with patch(
            "services.ai_service.ClaudeSDKClient",
            return_value=MagicMock(
                __aenter__=AsyncMock(return_value=mock_client),
                __aexit__=AsyncMock(return_value=False),
            ),
        ):
            with pytest.raises(InlineEditEmptyResultError):
                await svc.quick_rewrite("sys", "usr")

    @pytest.mark.asyncio
    async def test_raises_on_timeout(self):
        svc = _make_service()

        async def _slow_iter():
            await asyncio.sleep(0.5)
            yield _assistant_message("never")
            return

        mock_client = MagicMock()
        mock_client.query = AsyncMock()
        mock_client.receive_response = MagicMock(return_value=_slow_iter())

        with patch(
            "services.ai_service.ClaudeSDKClient",
            return_value=MagicMock(
                __aenter__=AsyncMock(return_value=mock_client),
                __aexit__=AsyncMock(return_value=False),
            ),
        ):
            with pytest.raises(InlineEditTimeoutError):
                await svc.quick_rewrite("sys", "usr", timeout_s=0.1)

    @pytest.mark.asyncio
    async def test_raises_when_disabled(self):
        svc = _make_service(enabled=False)
        with pytest.raises(RuntimeError):
            await svc.quick_rewrite("sys", "usr")

    @pytest.mark.asyncio
    async def test_ignores_non_text_blocks(self):
        svc = _make_service()

        text_block = MagicMock(spec=TextBlock)
        text_block.text = "kept"
        other_block = MagicMock()
        other_block.text = None  # simulate non-text / empty block
        msg = MagicMock(spec=AssistantMessage)
        msg.content = [text_block, other_block]

        mock_client = MagicMock()
        mock_client.query = AsyncMock()
        mock_client.receive_response = MagicMock(return_value=_StubAsyncIter([msg]))

        with patch(
            "services.ai_service.ClaudeSDKClient",
            return_value=MagicMock(
                __aenter__=AsyncMock(return_value=mock_client),
                __aexit__=AsyncMock(return_value=False),
            ),
        ):
            result = await svc.quick_rewrite("sys", "usr")

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
