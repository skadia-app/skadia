"""Tests for lift enrichment sport taxonomy handling."""

import importlib.util
import sys
import types
from pathlib import Path


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
    assert "sport_type, sub_type" in sql
    assert "a.sport_type, COALESCE(NULLIF(a.sub_type, ''), 'generic')" in sql
