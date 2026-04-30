"""Tests for lift enrichment sport taxonomy handling."""

import importlib.util
import asyncio
import sys
import types
from pathlib import Path
from uuid import uuid4


if "asyncpg" not in sys.modules:
    asyncpg_stub = types.ModuleType("asyncpg")
    asyncpg_stub.Pool = object
    sys.modules["asyncpg"] = asyncpg_stub

spec = importlib.util.spec_from_file_location(
    "lift_module_under_test",
    Path(__file__).parents[1] / "enrichment" / "lifts.py",
)
lifts = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(lifts)


def test_lift_relevance_accepts_physical_bikes_and_excludes_indoor_virtual():
    assert lifts._is_lift_relevant_activity("cycling", None, False, False)
    assert lifts._is_lift_relevant_activity("cycling", "mountain", False, False)
    assert lifts._is_lift_relevant_activity("cycling", "road", False, False)
    assert lifts._is_lift_relevant_activity("cycling", "gravel_cycling", False, False)
    assert lifts._is_lift_relevant_activity("cycling", "mountain", False, True)

    assert not lifts._is_lift_relevant_activity("cycling", "indoor_cycling", False, False)
    assert not lifts._is_lift_relevant_activity("cycling", "spin", False, False)
    assert not lifts._is_lift_relevant_activity("cycling", "mountain", True, False)
    assert not lifts._is_lift_relevant_activity("cycling", "mountain", False, False, is_virtual=True)
    assert not lifts._is_lift_relevant_activity("swimming", "open_water", False, False)


def test_lift_effort_insert_persists_activity_sub_type():
    sql = lifts._segment_effort_insert_sql()

    assert "user_id" in sql
    assert "sport_type, sub_type, in_leaderboard" in sql
    assert "a.sport_type, COALESCE(NULLIF(a.sub_type, ''), 'generic'), false" in sql
    assert "a.id = $1 AND a.user_id = $8" in _compact(sql)


def test_osm_lift_candidates_sql_uses_existing_lift_id_column():
    sql = lifts._osm_lift_candidates_sql()

    assert "l.id AS id" in sql
    assert "a.user_id = $4" in _compact(sql)
    assert "way_id" not in sql


def test_lift_enrichment_activity_lookup_is_user_scoped():
    activity_id = str(uuid4())
    user_id = str(uuid4())
    conn = OwnerCheckingConn(activity_id, user_id)

    result = asyncio.run(lifts.enrich_lifts(FakePool(conn), activity_id, user_id))

    assert result == 0
    assert conn.checked_activity


def test_lift_activity_scoped_queries_bind_user_id():
    source = Path(__file__).parents[1] / "enrichment" / "lifts.py"
    text = source.read_text()

    assert "FROM activities WHERE id = $1 AND user_id = $2" in text
    assert "SELECT track_full FROM activity_tracks at" in text
    assert "a.user_id = $2" in text
    assert "DELETE FROM activity_lift_candidates" in text
    assert "AND a.user_id = $2" in text
    assert "UPDATE activities SET" in text
    assert "WHERE id = $1 AND user_id = $2" in text


def _compact(sql: str) -> str:
    return " ".join(sql.split())


class OwnerCheckingConn:
    def __init__(self, activity_id: str, user_id: str):
        self.activity_id = activity_id
        self.user_id = user_id
        self.checked_activity = False

    async def fetchrow(self, query, *params):
        if "FROM activities" in query and "sport_type" in query:
            self.checked_activity = True
            assert "user_id = $2" in _compact(query)
            assert params == (self.activity_id, self.user_id)
            return {
                "sport_type": "swimming",
                "sub_type": "open_water",
                "is_indoor": False,
                "is_electric": False,
                "is_virtual": False,
            }
        raise AssertionError(f"unexpected fetchrow: {query}")

    async def fetchval(self, query, *params):
        raise AssertionError(f"unexpected fetchval: {query}")

    async def fetch(self, query, *params):
        raise AssertionError(f"unexpected fetch: {query}")

    async def execute(self, query, *params):
        raise AssertionError(f"unexpected execute: {query}")


class FakePool:
    def __init__(self, conn):
        self.conn = conn

    def acquire(self):
        return FakeAcquire(self.conn)


class FakeAcquire:
    def __init__(self, conn):
        self.conn = conn

    async def __aenter__(self):
        return self.conn

    async def __aexit__(self, exc_type, exc, tb):
        return False
