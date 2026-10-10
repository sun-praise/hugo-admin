# coding: utf-8 -*-
"""Tests for the selection-assist API endpoints (SSE streaming + validation)."""

import json
import sys
from pathlib import Path

from flask import Flask

sys.path.insert(0, str(Path(__file__).parent.parent))

from routes.assist_routes import register_assist_routes
from services.assist_service import AssistNormalizeError

TRANSLATE_MODEL_JSON = json.dumps(
    {
        "translation": "Hello world",
        "hard_words": [{"word": "Hello", "source": "你好", "gloss": "int. 打招呼"}],
    },
    ensure_ascii=False,
)

POLISH_MODEL_JSON = json.dumps(
    {
        "candidates": [
            {"style": "formal", "note": "n1", "text": "A"},
            {"style": "concise", "note": "n2", "text": "B"},
            {"style": "restructure", "note": "n3", "text": "C"},
        ]
    },
    ensure_ascii=False,
)

REVIEW_MODEL_JSON = json.dumps(
    {
        "issues": [
            {
                "type": "overclaim",
                "severity": "high",
                "comment": "断言过强",
                "before": "所有人都知道",
                "after": "多数读者认为",
            }
        ]
    },
    ensure_ascii=False,
)


class _FakeAIService:
    """Async-gen quick_stream stand-in mirroring the AIService contract."""

    def __init__(self, chunks=None, error=None, enabled=True):
        self.enabled = enabled
        self.model_name = "fake-model"
        self._chunks = chunks or []
        self._error = error
        self.calls = []

    async def quick_stream(self, system_prompt, user_prompt, idle_timeout_s=None):
        self.calls.append((system_prompt, user_prompt))
        if self._error is not None:
            raise self._error
        for chunk in self._chunks:
            yield chunk


def _make_app(ai_service):
    app = Flask(__name__)
    app.config["TESTING"] = True
    app.register_blueprint(register_assist_routes(lambda: ai_service))
    return app


def _events(resp):
    """Parse an SSE response body into a list of event payload dicts."""
    events = []
    for frame in resp.get_data(as_text=True).split("\n\n"):
        if frame.startswith("data: "):
            events.append(json.loads(frame[len("data: ") :]))
    return events


def _post(client, task, **body):
    return client.post(
        f"/api/ai/assist/{task}",
        data=json.dumps(body),
        content_type="application/json",
    )


class TestAssistHappyPaths:
    def test_translate_streams_deltas_then_done(self):
        ai = _FakeAIService(
            chunks=['{"translation": "Hel', 'lo world", "hard_words": []}']
        )
        client = _make_app(ai).test_client()

        resp = _post(
            client,
            "translate",
            selected_text="你好",
            context_before="前",
            context_after="后",
        )
        assert resp.status_code == 200
        assert resp.mimetype == "text/event-stream"

        events = _events(resp)
        assert [e["type"] for e in events] == ["start", "delta", "delta", "done"]
        assert events[0]["task"] == "translate"
        assert events[0]["model"] == "fake-model"
        assert events[1]["text"] == '{"translation": "Hel'
        assert events[3]["result"] == {
            "translation": "Hello world",
            "hard_words": [],
        }

        system_prompt, user_prompt = ai.calls[0]
        assert "翻译" in system_prompt
        assert "<selection>\n你好\n</selection>" in user_prompt

    def test_polish_done_carries_three_candidates(self):
        ai = _FakeAIService(chunks=[POLISH_MODEL_JSON])
        client = _make_app(ai).test_client()

        resp = _post(client, "polish", selected_text="片段")
        events = _events(resp)

        done = events[-1]
        assert done["type"] == "done"
        assert [c["text"] for c in done["result"]["candidates"]] == ["A", "B", "C"]

    def test_review_receives_full_text_as_document(self):
        ai = _FakeAIService(chunks=[REVIEW_MODEL_JSON])
        client = _make_app(ai).test_client()

        resp = _post(client, "review", selected_text="片段", full_text="整篇文档")
        assert resp.status_code == 200

        events = _events(resp)
        assert events[-1]["result"]["issues"][0]["before"] == "所有人都知道"

        _, user_prompt = ai.calls[0]
        assert "<document>\n整篇文档\n</document>" in user_prompt


class TestAssistValidation:
    def test_unknown_task_404(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(client, "summarize", selected_text="x")
        assert resp.status_code == 404

    def test_missing_selected_text_400(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(client, "translate")
        assert resp.status_code == 400

    def test_oversized_selected_text_400(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(client, "translate", selected_text="x" * 5001)
        assert resp.status_code == 400

    def test_oversized_context_400(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(
            client,
            "translate",
            selected_text="x",
            context_before="y" * 2001,
        )
        assert resp.status_code == 400

    def test_review_without_full_text_400(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(client, "review", selected_text="x")
        assert resp.status_code == 400

    def test_review_with_oversized_full_text_400(self):
        client = _make_app(_FakeAIService()).test_client()
        resp = _post(client, "review", selected_text="x", full_text="y" * 30001)
        assert resp.status_code == 400

    def test_ai_not_configured_503(self):
        client = _make_app(_FakeAIService(enabled=False)).test_client()
        resp = _post(client, "translate", selected_text="x")
        assert resp.status_code == 503


class TestAssistStreamErrors:
    def test_unparseable_output_ends_with_error_event(self):
        ai = _FakeAIService(chunks=["抱歉，我无法输出 JSON。"])
        client = _make_app(ai).test_client()

        events = _events(_post(client, "translate", selected_text="x"))
        types = [e["type"] for e in events]

        assert types == ["start", "delta", "error"]
        assert "error" in events[-1]
        assert "result" not in events[-1]

    def test_contract_violation_ends_with_error_event(self):
        # Valid JSON, but no usable translation field.
        ai = _FakeAIService(chunks=['{"hard_words": []}'])
        client = _make_app(ai).test_client()

        events = _events(_post(client, "translate", selected_text="x"))
        assert events[-1]["type"] == "error"

    def test_upstream_exception_ends_with_error_event(self):
        ai = _FakeAIService(error=AssistNormalizeError("boom"))
        client = _make_app(ai).test_client()

        events = _events(_post(client, "polish", selected_text="x"))
        assert events[-1]["type"] == "error"
        assert "boom" in events[-1]["error"]

    def test_polish_with_fewer_than_three_candidates_ends_with_error(self):
        raw = json.dumps(
            {
                "candidates": [
                    {"style": "formal", "text": "A"},
                    {"style": "concise", "text": "B"},
                ]
            },
            ensure_ascii=False,
        )
        ai = _FakeAIService(chunks=[raw])
        client = _make_app(ai).test_client()

        events = _events(_post(client, "polish", selected_text="x"))
        assert events[-1]["type"] == "error"
        assert "polish" in events[-1]["error"]

    def test_review_with_empty_issues_returns_done(self):
        ai = _FakeAIService(chunks=['{"issues": []}'])
        client = _make_app(ai).test_client()

        events = _events(
            _post(client, "review", selected_text="x", full_text="整篇文档")
        )
        assert events[-1]["type"] == "done"
        assert events[-1]["result"] == {"issues": []}


class TestAssistAuth:
    def test_unauthenticated_returns_401(self, auth_store):
        import app as app_module

        client = app_module.app.test_client()
        resp = client.post(
            "/api/ai/assist/translate",
            data=json.dumps({"selected_text": "x"}),
            content_type="application/json",
        )
        assert resp.status_code == 401
