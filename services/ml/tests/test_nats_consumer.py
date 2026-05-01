"""Tests for the analytics dispatch in nats_consumer._process_analytics_message.

Covers:
- Mock NATS message → readiness cache row written.
- Blank user → refused:no_data envelope still ends up in analytics_cache.
- Module-failure isolation: cluster_activities raising does not block readiness.
- Pref-race → publisher.publish_analytics_invalidate is invoked.
"""
from __future__ import annotations

import contextlib
import json
import sys
import types

import pytest

# nats_consumer pulls aioboto3 + nats at import-time; the dev venv is minimal
# (asyncpg + pytest only). Stub the heavy optional deps before importing the
# module — the consumer code paths exercised here never touch S3 or NATS.
for _name in ("aioboto3", "nats", "nats.js", "nats.js.api", "nats.js.errors"):
    sys.modules.setdefault(_name, types.ModuleType(_name))
# nats.js.api needs ConsumerConfig/DeliverPolicy/AckPolicy attrs
_api = sys.modules["nats.js.api"]
for _attr in ("ConsumerConfig", "DeliverPolicy", "AckPolicy"):
    setattr(_api, _attr, type(_attr, (), {}))
# nats.js.errors needs NotFoundError
sys.modules["nats.js.errors"].NotFoundError = type("NotFoundError", (Exception,), {})
# nats.NATS is referenced as a type annotation in _process_analytics_message
sys.modules["nats"].NATS = type("NATS", (), {})
sys.modules["nats"].js = sys.modules["nats.js"]
sys.modules["nats.js"].errors = sys.modules["nats.js.errors"]
sys.modules["nats.js"].api = sys.modules["nats.js.api"]

# enrichment.document_vlm pulls in optional deps too — stub if missing.
try:  # pragma: no cover
    import enrichment.document_vlm  # noqa: F401
except Exception:  # noqa: BLE001
    enr = types.ModuleType("enrichment")
    doc_vlm = types.ModuleType("enrichment.document_vlm")
    doc_vlm.convert_to_markdown = lambda *a, **kw: None
    doc_vlm.process_document = lambda *a, **kw: None
    doc_vlm.upload_to_s3 = lambda *a, **kw: None
    enr_lifts = types.ModuleType("enrichment")
    sys.modules.setdefault("enrichment", enr)
    sys.modules["enrichment.document_vlm"] = doc_vlm
    # `from enrichment import enrich_lifts` — provide a stub
    sys.modules["enrichment"].enrich_lifts = lambda *a, **kw: None

import nats_consumer  # noqa: E402
import analytics.readiness as readiness_mod  # noqa: E402
from tests.conftest import seed_user  # noqa: E402


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class _FakeMsg:
    def __init__(self, payload: dict):
        self.data = json.dumps(payload).encode()
        self.metadata = type("Meta", (), {"num_delivered": 1})()
        self.acked = 0
        self.naked = 0

    async def ack(self):
        self.acked += 1

    async def nak(self, delay: int = 0):  # noqa: D401
        self.naked += 1


class _FakePool:
    """Pool stand-in whose acquire() yields a pre-built connection."""

    def __init__(self, conn):
        self._conn = conn

    def acquire(self):
        conn = self._conn

        @contextlib.asynccontextmanager
        async def _cm():
            yield conn

        return _cm()


class _FakePub:
    def __init__(self):
        self.calls: list[str] = []

    async def publish_analytics_invalidate(self, user_id: str) -> None:
        self.calls.append(user_id)


@pytest.fixture
def patch_pool(monkeypatch, conn):
    """Make nats_consumer.get_pool return a pool that hands out the test conn.

    Also stubs cluster_activities by default — it queries production tables
    (`user_profiles`) that aren't part of the readiness fixture surface, and a
    failed query inside the test's outer transaction would poison the rest.
    Tests that want to exercise cluster failure paths re-patch this attribute.
    """
    pool = _FakePool(conn)

    async def _get_pool():
        return pool

    async def _noop_clusters(_c, _u):
        return {"clusters": []}

    monkeypatch.setattr(nats_consumer, "get_pool", _get_pool)
    monkeypatch.setattr(nats_consumer, "cluster_activities", _noop_clusters)
    return pool


# ---------------------------------------------------------------------------
# Seed helpers (mirror patterns from test_readiness.py — kept local to this
# file so it stays self-contained).
# ---------------------------------------------------------------------------


async def _insert_hrv_night_history(conn, uid: str, n_days: int = 30):
    """Seed n_days of nightly RMSSD ending yesterday in health_hrv_night_values."""
    for i in range(1, n_days + 1):
        rmssd = 50.0 + (5.0 if i % 2 == 0 else -5.0)
        await conn.execute(
            """
            INSERT INTO health_hrv_night_values (user_id, ts, rmssd_ms)
            VALUES ($1::uuid,
                    (CURRENT_DATE - $2::int)::timestamptz - INTERVAL '6 hours',
                    $3)
            ON CONFLICT (user_id, ts) DO NOTHING
            """,
            uid, i, rmssd,
        )


async def _insert_hrv_today(conn, uid: str, rmssd: float = 60.0):
    await conn.execute(
        """
        INSERT INTO health_snapshots (user_id, measured_at_utc, rmssd_ms, source_type)
        VALUES ($1::uuid, NOW() - INTERVAL '2 hours', $2, 'garmin_fit')
        """,
        uid, rmssd,
    )


async def _insert_sleep(conn, uid: str, score: int = 82):
    await conn.execute(
        """
        INSERT INTO health_sleep
            (user_id, calendar_date, sleep_start_utc, sleep_end_utc,
             deep_seconds, light_seconds, rem_seconds, score_overall, source_type)
        VALUES ($1::uuid,
                CURRENT_DATE,
                NOW() - INTERVAL '10 hours',
                NOW() - INTERVAL '2 hours',
                $2, $2, $2, $3, 'garmin_fit')
        """,
        uid, 7 * 3600 // 3, score,
    )


async def _insert_pref(conn, uid: str, pref: str = "skadia"):
    await conn.execute(
        """
        INSERT INTO user_readiness_preferences (user_id, preferred_source)
        VALUES ($1::uuid, $2)
        ON CONFLICT (user_id) DO UPDATE
            SET preferred_source = EXCLUDED.preferred_source, updated_at = NOW()
        """,
        uid, pref,
    )


async def _fetch_readiness_cache(conn, uid: str):
    return await conn.fetchrow(
        """
        SELECT data, expires_at
          FROM analytics_cache
         WHERE user_id = $1::uuid AND cache_key = 'readiness'
        """,
        uid,
    )


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


async def test_message_writes_readiness_cache(conn, patch_pool):
    """End-to-end: a NATS message for a seeded user produces a readiness
    cache row with expires_at set."""
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid)
    await _insert_hrv_today(conn, uid)
    await _insert_sleep(conn, uid)
    await _insert_pref(conn, uid)

    msg = _FakeMsg({"user_id": uid})
    await nats_consumer._process_analytics_message(msg, nc=None, publisher=_FakePub())

    row = await _fetch_readiness_cache(conn, uid)
    assert row is not None, "readiness cache row missing"
    assert row["expires_at"] is not None
    data = row["data"]
    if isinstance(data, str):
        data = json.loads(data)
    # Either fully scored or refused — both are valid envelopes; the point of
    # this test is that the row was written by the dispatch loop.
    assert "refused" in data
    assert msg.acked == 1


async def test_blank_user_writes_no_data_refused(conn, patch_pool):
    """A user with no health data still gets a refused envelope written."""
    uid = await seed_user(conn)

    msg = _FakeMsg({"user_id": uid})
    await nats_consumer._process_analytics_message(msg, nc=None, publisher=_FakePub())

    row = await _fetch_readiness_cache(conn, uid)
    assert row is not None
    data = row["data"]
    if isinstance(data, str):
        data = json.loads(data)
    assert data["refused"] is True
    assert data["refusal_reason"] == "no_data"


async def test_module_failure_isolation(conn, patch_pool, monkeypatch):
    """If cluster_activities raises, readiness still gets written."""
    uid = await seed_user(conn)

    async def _boom(_conn, _uid):
        raise RuntimeError("clusters exploded")

    monkeypatch.setattr(nats_consumer, "cluster_activities", _boom)

    msg = _FakeMsg({"user_id": uid})
    await nats_consumer._process_analytics_message(msg, nc=None, publisher=_FakePub())

    row = await _fetch_readiness_cache(conn, uid)
    assert row is not None, "readiness cache missing despite cluster_activities failure"
    # And the clusters row must NOT exist (its module raised before upsert).
    clusters_row = await conn.fetchrow(
        "SELECT 1 FROM analytics_cache WHERE user_id=$1::uuid AND cache_key='clusters'",
        uid,
    )
    assert clusters_row is None
    assert msg.acked == 1


async def test_pref_race_triggers_publisher(conn, patch_pool):
    """Force the pref-race path; the dispatch publisher must be called once."""
    uid = await seed_user(conn)
    await _insert_hrv_night_history(conn, uid)
    await _insert_hrv_today(conn, uid)
    await _insert_sleep(conn, uid)
    await _insert_pref(conn, uid, "skadia")

    orig = readiness_mod._read_pref
    call_count = {"n": 0}

    async def racy_read(c, user_id):
        call_count["n"] += 1
        if call_count["n"] == 1:
            return await orig(c, user_id)
        await c.execute(
            """
            UPDATE user_readiness_preferences
               SET preferred_source='manufacturer',
                   updated_at = NOW() + INTERVAL '1 second'
             WHERE user_id=$1::uuid
            """,
            user_id,
        )
        return await orig(c, user_id)

    pub = _FakePub()
    msg = _FakeMsg({"user_id": uid})

    readiness_mod._read_pref = racy_read
    try:
        await nats_consumer._process_analytics_message(msg, nc=None, publisher=pub)
    finally:
        readiness_mod._read_pref = orig

    assert pub.calls == [uid], f"expected one re-publish, got {pub.calls!r}"
