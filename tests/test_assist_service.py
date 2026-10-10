# coding: utf-8 -*-
"""Unit tests for services/assist_service.py (prompts + normalization)."""

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).parent.parent))

from services.assist_service import (
    AssistNormalizeError,
    build_messages,
    extract_json,
    normalize_for,
    normalize_polish,
    normalize_review,
    normalize_translate,
)

# ---------------------------------------------------------------------------
# build_messages
# ---------------------------------------------------------------------------


class TestBuildMessages:
    def test_translate_includes_contexts_not_document(self):
        system, user = build_messages(
            "translate",
            selected_text="选区",
            context_before="前文",
            context_after="后文",
        )
        assert "翻译" in system
        assert "<selection>\n选区\n</selection>" in user
        assert "<context_before>\n前文\n</context_before>" in user
        assert "<context_after>\n后文\n</context_after>" in user
        assert "<document>" not in user

    def test_polish_omits_empty_contexts(self):
        system, user = build_messages("polish", selected_text="片段")
        assert "3 个候选" in system
        assert "<context_before>" not in user
        assert "<context_after>" not in user

    def test_review_includes_document_and_selection(self):
        system, user = build_messages(
            "review", selected_text="片段", full_text="全文内容"
        )
        assert "overclaim" in system
        assert "<document>\n全文内容\n</document>" in user
        assert "<selection>\n片段\n</selection>" in user

    def test_unsupported_task_raises(self):
        with pytest.raises(ValueError):
            build_messages("custom", selected_text="x")


# ---------------------------------------------------------------------------
# extract_json
# ---------------------------------------------------------------------------


class TestExtractJson:
    def test_plain_json(self):
        assert extract_json('{"a": 1}') == {"a": 1}

    def test_fenced_json(self):
        raw = '```json\n{"a": 1}\n```'
        assert extract_json(raw) == {"a": 1}

    def test_json_with_surrounding_prose(self):
        raw = '以下是结果：{"a": {"b": 2}} 希望有帮助'
        assert extract_json(raw) == {"a": {"b": 2}}

    def test_trailing_comma_repair(self):
        raw = '{"a": 1, "b": [1, 2,],}'
        assert extract_json(raw) == {"a": 1, "b": [1, 2]}

    def test_no_json_returns_none(self):
        assert extract_json("抱歉，我无法完成该任务。") is None
        assert extract_json("") is None

    def test_unrepairable_json_returns_none(self):
        assert extract_json('{"a": [1, 2') is None


# ---------------------------------------------------------------------------
# normalize_translate
# ---------------------------------------------------------------------------


class TestNormalizeTranslate:
    def test_happy_path(self):
        raw = {
            "translation": "The translation",
            "hard_words": [
                {"word": "translation", "source": "翻译", "gloss": "n. 翻译"}
            ],
        }
        result = normalize_translate(raw)
        assert result["translation"] == "The translation"
        assert result["hard_words"] == [
            {"word": "translation", "source": "翻译", "gloss": "n. 翻译"}
        ]

    def test_field_fallbacks(self):
        raw = {
            "zh": "译文",
            "terms": [{"label": "词", "en": "word", "note": "释义"}],
        }
        result = normalize_translate(raw)
        assert result["translation"] == "译文"
        assert result["hard_words"][0] == {
            "word": "词",
            "source": "word",
            "gloss": "释义",
        }

    def test_caps_hard_words_at_eight(self):
        raw = {
            "translation": "t",
            "hard_words": [{"word": f"w{i}"} for i in range(20)],
        }
        assert len(normalize_translate(raw)["hard_words"]) == 8

    def test_missing_translation_returns_none(self):
        assert normalize_translate({"hard_words": []}) is None
        assert normalize_translate("not a dict") is None


# ---------------------------------------------------------------------------
# normalize_polish
# ---------------------------------------------------------------------------


class TestNormalizePolish:
    def test_happy_path_three_candidates(self):
        raw = {
            "candidates": [
                {"style": "formal", "note": "a", "text": "A"},
                {"style": "concise", "note": "b", "text": "B"},
                {"style": "restructure", "note": "c", "text": "C"},
                {"style": "formal", "note": "d", "text": "D"},
            ]
        }
        result = normalize_polish(raw)
        assert [c["text"] for c in result["candidates"]] == ["A", "B", "C"]

    def test_style_fallback_by_position(self):
        raw = {
            "candidates": [
                {"text": "A"},
                {"text": "B"},
                {"text": "C"},
            ]
        }
        result = normalize_polish(raw)
        assert [c["style"] for c in result["candidates"]] == [
            "formal",
            "concise",
            "restructure",
        ]

    def test_accepts_top_level_array(self):
        result = normalize_polish(
            [
                {"style": "formal", "text": "A"},
                {"style": "concise", "text": "B"},
                {"style": "restructure", "text": "C"},
            ]
        )
        assert len(result["candidates"]) == 3

    def test_fewer_than_three_candidates_returns_none(self):
        assert (
            normalize_polish(
                [{"style": "concise", "text": "A"}, {"style": "formal", "text": "B"}]
            )
            is None
        )

    def test_no_valid_candidates_returns_none(self):
        assert normalize_polish({"candidates": [{"note": "no text"}]}) is None
        assert normalize_polish(None) is None


# ---------------------------------------------------------------------------
# normalize_review
# ---------------------------------------------------------------------------


class TestNormalizeReview:
    def test_happy_path_with_fix(self):
        raw = {
            "issues": [
                {
                    "type": "overclaim",
                    "severity": "high",
                    "comment": "断言过强",
                    "before": "所有人都知道",
                    "after": "多数读者认为",
                }
            ]
        }
        result = normalize_review(raw)
        assert result["issues"][0]["before"] == "所有人都知道"
        assert result["issues"][0]["after"] == "多数读者认为"

    def test_fix_object_fallback(self):
        raw = {
            "issues": [
                {
                    "category": "terminology",
                    "sev": "low",
                    "detail": "术语不一致",
                    "fix": {"find": "缓存", "replace": "高速缓存"},
                }
            ]
        }
        issue = normalize_review(raw)["issues"][0]
        assert issue["type"] == "terminology"
        assert issue["severity"] == "info"
        assert issue["before"] == "缓存"
        assert issue["after"] == "高速缓存"

    def test_unknown_type_and_severity_fall_back(self):
        raw = {"issues": [{"type": "weird", "severity": "medium", "comment": "c"}]}
        issue = normalize_review(raw)["issues"][0]
        assert issue["type"] == "other"
        assert issue["severity"] == "warn"

    def test_before_without_after_drops_fix(self):
        raw = {"issues": [{"comment": "c", "before": "片段"}]}
        issue = normalize_review(raw)["issues"][0]
        assert issue["before"] is None
        assert issue["after"] is None

    def test_empty_issues_is_valid(self):
        assert normalize_review({"issues": []}) == {"issues": []}
        assert normalize_review({"issues": [{"no_comment": True}]}) == {"issues": []}

    def test_invalid_shape_returns_none(self):
        assert normalize_review({"summary": "no issues key"}) is None
        assert normalize_review({"issues": "not-a-list"}) is None
        assert normalize_review([]) is None


# ---------------------------------------------------------------------------
# normalize_for
# ---------------------------------------------------------------------------


class TestNormalizeFor:
    def test_dispatches_by_task(self):
        assert normalize_for("translate", {"translation": "t"})["translation"] == "t"

    def test_raises_on_contract_violation(self):
        with pytest.raises(AssistNormalizeError):
            normalize_for("polish", {"candidates": []})

    def test_raises_on_unsupported_task(self):
        with pytest.raises(AssistNormalizeError):
            normalize_for("custom", {"text": "x"})
