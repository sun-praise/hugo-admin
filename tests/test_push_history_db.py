# coding: utf-8
"""
git_push_history 表与 Database.record_push / list_pushes 的测试。
"""

import tempfile
import time
from pathlib import Path

import pytest

from models.database import Database


class TestPushHistoryDB:
    @pytest.fixture
    def db(self):
        with tempfile.TemporaryDirectory() as d:
            yield Database(str(Path(d) / "cache.db"))

    def _record(self, db, **overrides):
        defaults = dict(
            remote="origin",
            branch="main",
            from_sha="",
            to_sha="abc123",
            commit_count=1,
            commit_message="msg",
            success=True,
            message="ok",
        )
        defaults.update(overrides)
        return db.record_push(**defaults)

    def test_table_created(self, db):
        conn = db._get_connection()
        cur = conn.cursor()
        cur.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
            " AND name='git_push_history'"
        )
        assert cur.fetchone()[0] == "git_push_history"
        conn.close()

    def test_record_then_list_newest_first(self, db):
        first = self._record(db, to_sha="aaa", commit_message="first")
        # 确保 pushed_at 有差异，倒序稳定
        time.sleep(0.01)
        second = self._record(db, to_sha="bbb", commit_message="second")

        res = db.list_pushes(limit=10, offset=0)
        assert res["success"] is True
        assert res["total"] == 2
        # 倒序：second 在前
        assert res["pushes"][0]["id"] == second
        assert res["pushes"][1]["id"] == first
        assert res["pushes"][0]["to_sha"] == "bbb"

    def test_empty_sha_round_trips(self, db):
        self._record(db, from_sha="", to_sha="", commit_count=0)
        res = db.list_pushes()
        p = res["pushes"][0]
        assert p["from_sha"] == ""
        assert p["to_sha"] == ""
        assert p["commit_count"] == 0

    def test_pagination(self, db):
        for i in range(5):
            self._record(db, to_sha=f"sha{i}", commit_message=f"m{i}")
        page1 = db.list_pushes(limit=2, offset=0)
        page2 = db.list_pushes(limit=2, offset=2)
        assert page1["total"] == 5
        assert len(page1["pushes"]) == 2
        assert len(page2["pushes"]) == 2
        # 两页不重叠
        ids1 = {p["id"] for p in page1["pushes"]}
        ids2 = {p["id"] for p in page2["pushes"]}
        assert ids1.isdisjoint(ids2)

    def test_success_flag_and_iso(self, db):
        self._record(db, success=False, message="boom")
        p = db.list_pushes()["pushes"][0]
        assert p["success"] is False
        assert p["message"] == "boom"
        assert isinstance(p["pushed_at"], float)
        assert "T" in p["pushed_at_iso"]
