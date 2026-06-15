# coding: utf-8
"""
Tests for the inline-edit API endpoint and its post-processing helpers.
"""

import json
import sys
from pathlib import Path
from unittest.mock import MagicMock

from flask import Flask

sys.path.insert(0, str(Path(__file__).parent.parent))

from routes.inline_edit_routes import (
    _looks_like_markdown,
    _post_process,
    _unwrap_single_fence,
    register_inline_edit_routes,
)
from services.ai_service import InlineEditEmptyResultError, InlineEditTimeoutError

# ---------------------------------------------------------------------------
# Post-processing helper tests (pure functions, no Flask)
# ---------------------------------------------------------------------------


class TestUnwrapSingleFence:
    def test_unwraps_markdown_fence(self):
        raw = "```markdown\n# Title\n\nBody.\n```"
        out, fired = _unwrap_single_fence(raw)
        assert fired is True
        assert "# Title" in out
        assert "```" not in out

    def test_unwraps_plain_fence(self):
        raw = "```\nhello world\n```"
        out, fired = _unwrap_single_fence(raw)
        assert fired is True
        assert out == "hello world"

    def test_no_change_when_not_fenced(self):
        out, fired = _unwrap_single_fence("plain text")
        assert fired is False
        assert out == "plain text"

    def test_no_change_when_fence_has_inner_fence(self):
        raw = "```\n```python\nx = 1\n```\n```"
        out, fired = _unwrap_single_fence(raw)
        assert fired is False
        assert out == raw

    def test_no_change_when_fence_not_closed(self):
        raw = "```markdown\n# Title"
        out, fired = _unwrap_single_fence(raw)
        assert fired is False


class TestPostProcess:
    def test_unwrap_then_no_strip(self):
        raw = "```markdown\n# Title\n```"
        out, reasons = _post_process(raw)
        assert "unwrapped_single_fence" in reasons
        assert "# Title" in out

    def test_no_reasons_on_pure_markdown(self):
        raw = "## Heading\n\nBody."
        out, reasons = _post_process(raw)
        assert reasons == []

    def test_keeps_plain_prose(self):
        # Chinese plain-prose responses must NOT be stripped — the inline-edit
        # result is allowed to be plain prose, and stripping the first line
        # would discard the model's actual answer.
        raw = "为什么选择 MiniMax M3？M3 的智能水平不逊于 GLM-5.1。"
        out, reasons = _post_process(raw)
        assert reasons == []
        assert out == raw


class TestLooksLikeMarkdown:
    def test_heading(self):
        assert _looks_like_markdown("# heading")
        assert _looks_like_markdown("## heading")

    def test_list(self):
        assert _looks_like_markdown("- item")
        assert _looks_like_markdown("* item")
        assert _looks_like_markdown("1. item")

    def test_blockquote(self):
        assert _looks_like_markdown("> quote")

    def test_code(self):
        assert _looks_like_markdown("`code`")

    def test_bold(self):
        assert _looks_like_markdown("**bold**")

    def test_link(self):
        assert _looks_like_markdown("[text](url)")

    def test_plain_text_not_markdown(self):
        assert not _looks_like_markdown("好的，这是改写后的版本")
        assert not _looks_like_markdown("Here is the rewrite:")

    def test_blank_line_is_markdown_neutral(self):
        assert _looks_like_markdown("")
        assert _looks_like_markdown("   ")


# ---------------------------------------------------------------------------
# Endpoint tests (Flask test client + mocked AI service)
# ---------------------------------------------------------------------------


def _make_app(ai_service):
    """Build a Flask app with the inline-edit blueprint and a fake AI service."""
    app = Flask(__name__)
    app.config["TESTING"] = True
    bp = register_inline_edit_routes(lambda: ai_service)
    app.register_blueprint(bp)
    return app


class TestInlineEditEndpoint:
    def test_400_when_selected_text_missing(self):
        app = _make_app(_FakeAIService(revised="ok"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"instruction": "polish"}),
            content_type="application/json",
        )
        assert resp.status_code == 400
        body = resp.get_json()
        assert body["success"] is False

    def test_400_when_instruction_missing(self):
        app = _make_app(_FakeAIService(revised="ok"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "hello"}),
            content_type="application/json",
        )
        assert resp.status_code == 400

    def test_503_when_ai_disabled(self):
        disabled = MagicMock()
        disabled.enabled = False
        app = _make_app(disabled)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 503
        assert "not configured" in resp.get_json()["message"].lower()

    def test_200_on_success(self):
        ai = _FakeAIService(revised="revised text")
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps(
                {
                    "selected_text": "hello",
                    "instruction": "polish",
                    "context_before": "before",
                    "context_after": "after",
                }
            ),
            content_type="application/json",
        )
        assert resp.status_code == 200
        body = resp.get_json()
        assert body["success"] is True
        assert body["revised_text"] == "revised text"
        assert "model" in body

    def test_500_on_empty_result(self):
        ai = MagicMock()
        ai.enabled = True
        ai.model_name = "test-model"
        ai.quick_rewrite = MagicMock(side_effect=InlineEditEmptyResultError("empty"))
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 500

    def test_504_on_timeout(self):
        ai = MagicMock()
        ai.enabled = True
        ai.model_name = "test-model"
        ai.quick_rewrite = MagicMock(side_effect=InlineEditTimeoutError("timed out"))
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 504

    def test_post_process_keeps_plain_prose(self):
        # The pre-strip heuristic used to drop the first "non-Markdown" line,
        # which is unsafe for plain-prose selections (Chinese sentences
        # without any markdown marker). Plain prose must be returned as-is.
        ai = _FakeAIService(revised="好的，这是改写：\n这是真正的内容。")
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 200
        body = resp.get_json()
        assert "好的" in body["revised_text"]
        assert "这是真正的内容" in body["revised_text"]

    def test_post_process_unwraps_single_fence(self):
        ai = _FakeAIService(revised="```markdown\n## Heading\n\nbody\n```")
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 200
        body = resp.get_json()
        assert "```" not in body["revised_text"]
        assert "## Heading" in body["revised_text"]

    # ---- security: per-field size caps ---------------------------------

    def test_400_when_selected_text_too_long(self):
        app = _make_app(_FakeAIService(revised="ok"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x" * 5001, "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 400

    def test_400_when_instruction_too_long(self):
        app = _make_app(_FakeAIService(revised="ok"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y" * 1001}),
            content_type="application/json",
        )
        assert resp.status_code == 400

    def test_400_when_context_too_long(self):
        app = _make_app(_FakeAIService(revised="ok"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps(
                {
                    "selected_text": "x",
                    "instruction": "y",
                    "context_before": "a" * 1001,
                }
            ),
            content_type="application/json",
        )
        assert resp.status_code == 400

    # ---- security: error message hygiene ------------------------------

    def test_500_uses_generic_message_on_runtime_error(self):
        ai = MagicMock()
        ai.enabled = True
        ai.model_name = "test-model"
        ai.quick_rewrite = MagicMock(
            side_effect=RuntimeError("internal stack trace from SDK")
        )
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 500
        body = resp.get_json()
        assert "stack trace" not in body["message"]
        assert "SDK" not in body["message"]

    def test_500_uses_generic_message_on_unexpected_error(self):
        ai = MagicMock()
        ai.enabled = True
        ai.model_name = "test-model"
        ai.quick_rewrite = MagicMock(side_effect=ValueError("leaky internal detail"))
        app = _make_app(ai)
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 500
        body = resp.get_json()
        assert "leaky" not in body["message"]

    # ---- security: dangerous response payload -------------------------

    def test_500_when_response_contains_script_tag(self):
        app = _make_app(_FakeAIService(revised="polished <script>alert(1)</script>"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 500

    def test_500_when_response_contains_javascript_url(self):
        app = _make_app(_FakeAIService(revised="see [link](javascript:alert(1))"))
        client = app.test_client()
        resp = client.post(
            "/api/ai/inline-edit",
            data=json.dumps({"selected_text": "x", "instruction": "y"}),
            content_type="application/json",
        )
        assert resp.status_code == 500


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class _FakeAIService:
    """Synchronous fake of the async quick_rewrite contract for the route.

    The route calls ``anyio.run(ai_service.quick_rewrite, ...)`` so the fake
    must be a sync callable that returns the revised text directly.
    """

    def __init__(self, revised: str = "ok"):
        self.revised = revised
        self.enabled = True
        self.model_name = "fake-model"

    def quick_rewrite(
        self, system_prompt: str, user_prompt: str
    ) -> str:  # noqa: ARG002
        return self.revised
