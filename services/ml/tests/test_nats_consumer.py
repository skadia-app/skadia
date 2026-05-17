"""Tests for the analytics dispatch in nats_consumer._process_analytics_message.

Covers:
- Mock NATS message → readiness cache row written.
- Blank user → refused:no_data envelope still ends up in analytics_cache.
- Module-failure isolation: cluster_activities raising does not block readiness.
- Pref-race → publisher.publish_analytics_invalidate is invoked.
- _BoundedSemaphoreMap: LRU-Cap, TTL eviction, acquired-semaphore safety.
"""
from __future__ import annotations

import asyncio
import contextlib
import json
import sys
import time
import types

import pytest

# nats_consumer pulls aioboto3 + nats + sklearn (via analytics) at import-time;
# the dev venv is minimal (asyncpg + pytest only). Stub heavy optional deps
# before importing the module — the consumer code paths exercised here never
# touch S3, NATS, or ML inference.
for _name in ("aioboto3", "nats", "nats.js", "nats.js.api", "nats.js.errors"):
    sys.modules.setdefault(_name, types.ModuleType(_name))

# sklearn is pulled by analytics.clustering; stub it out so the import chain
# resolves without the wheel installed.
for _sk in ("sklearn", "sklearn.cluster", "sklearn.preprocessing", "sklearn.metrics"):
    sys.modules.setdefault(_sk, types.ModuleType(_sk))
sys.modules["sklearn.cluster"].KMeans = type("KMeans", (), {"__init__": lambda s, *a, **kw: None})
sys.modules["sklearn.preprocessing"].StandardScaler = type("StandardScaler", (), {"__init__": lambda s, *a, **kw: None})
sys.modules["sklearn.metrics"].silhouette_score = lambda *a, **kw: 0.0
# ruptures and numpy are used by analytics sub-modules (changepoints, dfa_alpha1
# etc.).  Stub them with just enough surface area so the import chain resolves.
for _mod_name in ("ruptures", "numpy"):
    sys.modules.setdefault(_mod_name, types.ModuleType(_mod_name))

_rpt_mod = sys.modules["ruptures"]
_rpt_mod.Pelt = type(
    "Pelt", (),
    {
        "__init__": lambda s, *a, **kw: None,
        "fit": lambda s, *a, **kw: s,
        "predict": lambda s, *a, **kw: [],
    }
)

_np = sys.modules["numpy"]


class _NdArray:
    pass


_np.ndarray = _NdArray
_np.bool_ = bool
_np.isscalar = lambda obj: not isinstance(obj, _NdArray)
_np.array = lambda *a, **kw: _NdArray()
_np.arange = lambda *a, **kw: _NdArray()
_np.cumsum = lambda *a, **kw: _NdArray()
_np.mean = lambda *a, **kw: 0.0
_np.std = lambda *a, **kw: 0.0
_np.nan = float("nan")
_np.inf = float("inf")
_np.log = lambda *a, **kw: 0.0
_np.polyfit = lambda *a, **kw: [0.0, 0.0]
_np.polyval = lambda *a, **kw: 0.0
_np.isfinite = lambda *a, **kw: True
_np.isnan = lambda *a, **kw: False
_np.maximum = lambda *a, **kw: 0.0
_np.float64 = float
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
    doc_vlm.convert_document_to_markdown = lambda *a, **kw: None
    doc_vlm.process_document = lambda *a, **kw: None
    sys.modules.setdefault("enrichment", enr)
    sys.modules["enrichment.document_vlm"] = doc_vlm

import nats_consumer  # noqa: E402
import analytics.readiness as readiness_mod  # noqa: E402
from tests.conftest import seed_user  # noqa: E402

DOC_ID = "11111111-1111-1111-1111-111111111111"
USER_ID = "22222222-2222-2222-2222-222222222222"
FILE_ID = "33333333-3333-3333-3333-333333333333"


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class _FakeMsg:
    def __init__(self, payload: dict):
        self.data = json.dumps(payload).encode()
        self.metadata = type("Meta", (), {"num_delivered": 1})()
        self.acked = 0
        self.naked = 0
        self.in_progress_calls = 0

    async def ack(self):
        self.acked += 1

    async def nak(self, delay: int = 0):  # noqa: D401
        self.naked += 1

    async def in_progress(self):
        self.in_progress_calls += 1


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


class _NCNoop:
    async def publish(self, *args):
        return None


def test_s3_bucket_for_file_type_defaults_to_shadowfs_rules(monkeypatch):
    monkeypatch.delenv("S3_BUCKET", raising=False)
    monkeypatch.delenv("S3_BUCKET_MEDIA", raising=False)
    monkeypatch.delenv("S3_BUCKET_ACTIVITY", raising=False)

    assert nats_consumer._s3_bucket_for_file_type("media") == "skadia-originals"
    assert nats_consumer._s3_bucket_for_file_type("activity") == "skadia-originals"

    monkeypatch.setenv("S3_BUCKET", "default-bucket")
    monkeypatch.setenv("S3_BUCKET_MEDIA", "media-bucket")
    monkeypatch.setenv("S3_BUCKET_ACTIVITY", "activity-bucket")

    assert nats_consumer._s3_bucket_for_file_type("media") == "media-bucket"
    assert nats_consumer._s3_bucket_for_file_type("document") == "media-bucket"
    assert nats_consumer._s3_bucket_for_file_type("activity") == "activity-bucket"
    assert nats_consumer._s3_bucket_for_file_type("other") == "default-bucket"


def test_document_timeout_envelopes_cover_bounded_page_work():
    page_markdown_budget = (
        nats_consumer.MAX_SCANNED_PDF_PAGES
        * nats_consumer.DOCUMENT_PAGE_MARKDOWN_TIMEOUT_SECONDS
    )
    structured_budget = (
        nats_consumer.MAX_SCANNED_PDF_PAGES
        * nats_consumer.DOCUMENT_VISION_TIMEOUT_SECONDS
    )

    assert nats_consumer.DOCUMENT_MARKDOWN_TIMEOUT_SECONDS > page_markdown_budget
    assert nats_consumer.DOCUMENT_PROCESS_TIMEOUT_SECONDS > structured_budget


@pytest.mark.asyncio
async def test_load_document_file_metadata_uses_file_id_user_and_doc_scope():
    class _Conn:
        def __init__(self):
            self.calls: list[tuple] = []

        async def fetchrow(self, query, *args):
            self.calls.append((query, *args))
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-invoice",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 1234,
            }

    conn = _Conn()
    metadata = await nats_consumer._load_document_file_metadata(_FakePool(conn), FILE_ID, USER_ID, DOC_ID)

    assert metadata == {
        "storage_backend": "s3",
        "storage_path": "documents/synthetic-invoice",
        "file_type": "document",
        "content_type": "application/pdf",
        "size_bytes": 1234,
    }
    assert "user_files" in conn.calls[0][0]
    assert "gear_documents" in conn.calls[0][0]
    assert conn.calls[0][1:] == (FILE_ID, USER_ID, DOC_ID)


@pytest.mark.asyncio
async def test_process_document_message_passes_user_file_metadata_not_legacy_s3(monkeypatch):
    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            return "UPDATE 1"

    pool = _FakePool(_Conn())

    async def fake_get_pool():
        return pool

    calls: list[dict] = []

    async def fake_process_document(**kwargs):
        calls.append(kwargs)
        return False

    async def fake_markdown(**kwargs):
        return None

    class _NC:
        async def publish(self, *args):
            return None

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)

    msg = _FakeMsg(
        {
            "doc_id": DOC_ID,
            "user_id": USER_ID,
            "file_id": FILE_ID,
            "s3_bucket": "legacy-bucket",
            "s3_key": "legacy-key",
        }
    )

    await nats_consumer._process_document_message(msg, _NC())

    assert msg.acked == 1
    assert calls[0]["file_id"] == FILE_ID
    assert calls[0]["storage_backend"] == "s3"
    assert calls[0]["storage_path"] == "documents/synthetic-current-document"
    assert calls[0]["file_type"] == "document"
    assert calls[0]["content_type"] == "application/pdf"
    assert "s3_bucket" not in calls[0]
    assert "s3_key" not in calls[0]


@pytest.mark.asyncio
async def test_process_document_message_binds_status_updates_to_document_owner(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    pool = _FakePool(_Conn())

    async def fake_get_pool():
        return pool

    async def fake_process_document(**kwargs):
        return True

    async def fake_markdown(**kwargs):
        return None

    class _NC:
        async def publish(self, *args):
            return None

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)

    await nats_consumer._process_document_message(
        _FakeMsg({"doc_id": DOC_ID, "user_id": USER_ID, "file_id": FILE_ID}),
        _NC(),
    )

    status_updates = [call for call in executed if "processing_status" in call[0]]
    assert status_updates
    assert all("user_id" in call[0] and "file_id" in call[0] for call in status_updates)
    assert all(call[-2:] == (USER_ID, FILE_ID) for call in status_updates)


@pytest.mark.asyncio
async def test_process_document_message_acks_malformed_or_legacy_document_ids(monkeypatch):
    async def fail_get_pool():  # pragma: no cover - must not run for invalid doc_id
        raise AssertionError("malformed messages must not touch DB")

    monkeypatch.setattr(nats_consumer, "get_pool", fail_get_pool)
    invalid = _FakeMsg({"doc_id": "not-a-uuid", "user_id": USER_ID, "file_id": FILE_ID})

    await nats_consumer._process_document_message(invalid, nc=None)

    assert invalid.acked == 1
    assert invalid.naked == 0

    executed: list[tuple] = []

    class _Conn:
        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    legacy = _FakeMsg({"doc_id": DOC_ID, "user_id": USER_ID})

    await nats_consumer._process_document_message(legacy, nc=None)

    assert legacy.acked == 1
    assert legacy.naked == 0
    assert executed
    assert "file_id" not in executed[0][0]


@pytest.mark.asyncio
async def test_process_document_message_acks_when_processed_event_publish_fails(monkeypatch):
    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    async def fake_process_document(**kwargs):
        return True

    async def fake_markdown(**kwargs):
        return None

    class _NC:
        async def publish(self, *args):
            raise RuntimeError("publish failed")

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)

    msg = _FakeMsg(
        {
            "doc_id": DOC_ID,
            "user_id": USER_ID,
            "file_id": FILE_ID,
            "extracted_text": "Rechnungstext " * 20,
        }
    )

    await nats_consumer._process_document_message(msg, _NC())

    assert msg.acked == 1
    assert msg.naked == 0


@pytest.mark.asyncio
async def test_process_document_message_converts_markdown_before_structured_extraction(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    calls: list[tuple[str, object]] = []

    async def fake_markdown(**kwargs):
        calls.append(("markdown", kwargs))
        return "# Invoice\n\nBrake Pads"

    async def fake_process_document(**kwargs):
        calls.append(("process", kwargs))
        return True

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)

    msg = _FakeMsg(
        {
            "doc_id": DOC_ID,
            "user_id": USER_ID,
            "file_id": FILE_ID,
            "is_text_pdf": False,
            "extracted_text": "q\n/R7 gs\n0 0 1 rg\n1 i\n1 2 3 4 re\nf\nQ\nq\nf\nQ",
        }
    )

    await nats_consumer._process_document_message(msg, _NCNoop())

    assert [name for name, _ in calls] == ["markdown", "process"]
    assert calls[0][1]["storage_path"] == "documents/synthetic-current-document"
    assert calls[1][1]["is_text_pdf"] is True
    assert calls[1][1]["extracted_text"] == "# Invoice\n\nBrake Pads"
    markdown_updates = [call for call in executed if "extracted_markdown" in call[0]]
    assert markdown_updates
    assert markdown_updates[0][1] == "# Invoice\n\nBrake Pads"


@pytest.mark.asyncio
async def test_process_document_message_reuses_existing_markdown_on_retry(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def fetchrow(self, query, *args):
            if "extracted_markdown" in query:
                return {
                    "extracted_text": "raw text",
                    "extracted_markdown": "# Existing\n\nMarkdown",
                }
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    async def fail_markdown(**kwargs):  # pragma: no cover - must not run
        raise AssertionError("existing extracted_markdown must be reused")

    calls: list[dict] = []

    async def fake_process_document(**kwargs):
        calls.append(kwargs)
        return True

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fail_markdown)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)

    msg = _FakeMsg({"doc_id": DOC_ID, "user_id": USER_ID, "file_id": FILE_ID})
    await nats_consumer._process_document_message(msg, _NCNoop())

    assert msg.acked == 1
    assert calls[0]["extracted_text"] == "# Existing\n\nMarkdown"
    assert not [call for call in executed if "extracted_markdown" in call[0]]


@pytest.mark.asyncio
async def test_process_document_message_naks_retryable_document_errors_without_marking_failed(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    async def fake_process_document(**kwargs):
        raise nats_consumer.RetryableDocumentError("broker unavailable")

    async def fake_markdown(**kwargs):
        return "# Invoice\n\nBrake Pads"

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)

    msg = _FakeMsg({"doc_id": DOC_ID, "user_id": USER_ID, "file_id": FILE_ID})
    await nats_consumer._process_document_message(msg, _NCNoop())

    assert msg.acked == 0
    assert msg.naked == 1
    assert not [call for call in executed if "processing_status" in call[0]]


@pytest.mark.asyncio
async def test_process_document_message_marks_failed_before_final_ack(monkeypatch):
    executed: list[tuple] = []

    class _Conn:
        async def fetchrow(self, query, *args):
            return {
                "storage_backend": "s3",
                "storage_path": "documents/synthetic-current-document",
                "file_type": "document",
                "content_type": "application/pdf",
                "size_bytes": 2048,
            }

        async def execute(self, *args):
            executed.append(args)
            return "UPDATE 1"

    async def fake_get_pool():
        return _FakePool(_Conn())

    async def fake_process_document(**kwargs):
        raise nats_consumer.RetryableDocumentError("broker unavailable")

    async def fake_markdown(**kwargs):
        return "# Invoice\n\nBrake Pads"

    monkeypatch.setattr(nats_consumer, "get_pool", fake_get_pool)
    monkeypatch.setattr(nats_consumer, "convert_document_to_markdown", fake_markdown)
    monkeypatch.setattr(nats_consumer, "process_document", fake_process_document)

    msg = _FakeMsg({"doc_id": DOC_ID, "user_id": USER_ID, "file_id": FILE_ID})
    msg.metadata.num_delivered = nats_consumer.MAX_RETRIES
    await nats_consumer._process_document_message(msg, _NCNoop())

    assert msg.acked == 1
    assert msg.naked == 0
    status_updates = [call for call in executed if "processing_status" in call[0]]
    assert status_updates
    assert status_updates[-1][1] == "failed"
    import_log_updates = [call for call in executed if "document_processed" in call[0]]
    assert import_log_updates
    assert import_log_updates[-1][-1] == "completed"


@pytest.mark.asyncio
async def test_document_processing_sends_in_progress_heartbeat(monkeypatch):
    monkeypatch.setattr(nats_consumer, "DOCUMENT_IN_PROGRESS_INTERVAL_SECONDS", 0.001)
    msg = _FakeMsg({})

    async def slow_work():
        await asyncio.sleep(0.01)
        return "done"

    assert await nats_consumer._run_with_in_progress_heartbeat(msg, slow_work(), 1.0) == "done"
    assert msg.in_progress_calls > 0


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


# ---------------------------------------------------------------------------
# _BoundedSemaphoreMap tests
# ---------------------------------------------------------------------------


def test_bounded_semaphore_map_cap():
    """Inserting more than cap entries keeps map size at or below cap.

    Only idle (not-acquired) semaphores may be evicted.  With cap=10 and
    10 001 distinct keys (all idle), the map must not exceed 10 000.
    """
    from nats_consumer import _BoundedSemaphoreMap

    cap = 10_000
    sm = _BoundedSemaphoreMap(initial_value=2, cap=cap, ttl=3600)
    for i in range(cap + 1):
        sm[f"user-{i}"]

    assert len(sm) <= cap, f"map grew to {len(sm)}, expected <= {cap}"


def test_bounded_semaphore_map_ttl_eviction():
    """Accessing a key after TTL expiry returns a fresh (unused) semaphore.

    We cannot inject wall-clock time directly, so we use a tiny TTL and
    monkeypatch time.monotonic to simulate passage of time.
    """
    import nats_consumer as nc_mod
    from nats_consumer import _BoundedSemaphoreMap

    ttl = 10  # 10 s TTL for the test
    sm = _BoundedSemaphoreMap(initial_value=2, cap=10_000, ttl=ttl)

    real_mono = time.monotonic

    # Step 1 – prime user-A at t=0
    t_base = real_mono()
    mono_calls = [t_base]

    def fake_mono():
        return mono_calls[0]

    original = nc_mod.time.monotonic
    nc_mod.time.monotonic = fake_mono
    try:
        sem_a_first = sm["user-A"]
        assert sem_a_first._value == 2

        # Step 2 – advance fake clock beyond TTL
        mono_calls[0] = t_base + ttl + 1

        # Add a second key to trigger _maybe_evict (cap not exceeded yet, but
        # TTL-expired stale entries should be cleaned up)
        sm["user-B"]

        # At this point user-A should have been evicted (it was idle + stale).
        # Accessing user-A again must create a brand-new semaphore.
        sem_a_second = sm["user-A"]
    finally:
        nc_mod.time.monotonic = original

    # A fresh Semaphore has _value == initial_value (2).
    assert sem_a_second._value == 2
    # The two objects must be different instances (eviction happened).
    assert sem_a_second is not sem_a_first, (
        "Expected a new Semaphore after TTL eviction, got the same object"
    )


def test_bounded_semaphore_map_acquired_never_evicted():
    """An acquired (held) semaphore must not be evicted even when over cap.

    Safety requirement: evicting an acquired semaphore would leave waiting
    coroutines permanently blocked on a semaphore that no-one will release.
    """
    from nats_consumer import _BoundedSemaphoreMap

    cap = 5
    sm = _BoundedSemaphoreMap(initial_value=1, cap=cap, ttl=3600)

    # Acquire user-protected's semaphore synchronously (no await needed for
    # Semaphore._value manipulation in a unit test — we set _value directly to
    # simulate an active acquire without running the event loop).
    protected_sem = sm["user-protected"]
    protected_sem._value = 0  # simulate acquired (makes locked() return True)

    # Now flood with cap+1 additional idle users to trigger eviction.
    for i in range(cap + 1):
        sm[f"user-idle-{i}"]

    # The protected semaphore must still be present and identical.
    assert "user-protected" in sm, "acquired semaphore was evicted — safety violation"
    retrieved = sm["user-protected"]
    assert retrieved is protected_sem, "acquired semaphore was replaced — safety violation"


def test_bounded_semaphore_map_logs_warning_above_double_cap(caplog):
    """When the map exceeds 2x cap, a warning is emitted.

    This acts as a watchdog for leaked (stuck-acquired) semaphores: normal
    eviction keeps the map at or near cap, so exceeding 2x cap indicates that
    many semaphores are permanently locked and cannot be freed.
    """
    import asyncio
    import logging
    import time
    from nats_consumer import _BoundedSemaphoreMap

    cap = 3
    sm = _BoundedSemaphoreMap(initial_value=1, cap=cap, ttl=3600)

    # Directly inject 2*cap+1 locked (stuck) semaphores into the internal store
    # without going through __getitem__ so that eviction is not triggered
    # piecemeal during setup.  This simulates the stuck-worker scenario where
    # semaphores are acquired but never released.
    now = time.monotonic()
    for i in range(2 * cap + 1):
        sem = asyncio.Semaphore(1)
        sem._value = 0  # simulate permanently acquired (locked() == True)
        sm._store[f"user-stuck-{i}"] = (sem, now)

    # Now trigger one normal __getitem__ access; _maybe_evict will see
    # len(store) == 2*cap+1 > 2*cap and must emit the watchdog warning.
    with caplog.at_level(logging.WARNING, logger="nats_consumer"):
        sm._maybe_evict(time.monotonic())

    assert any(
        "2x cap" in record.message or "above 2x cap" in record.message
        for record in caplog.records
    ), f"Expected 2x-cap warning, got: {[r.message for r in caplog.records]}"
