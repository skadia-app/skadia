import asyncio
import contextlib
import json
import logging
import os
import time
import uuid
from collections import OrderedDict

import aioboto3
import nats
from nats.js.api import ConsumerConfig, DeliverPolicy, AckPolicy

from db import get_pool
from config import get_storage_readiness_async, storage_readiness_ready
from progress import publish_progress
from s3_client import s3_client_kwargs
from enrichment.document_vlm import (
    DOCUMENT_PAGE_MARKDOWN_TIMEOUT_SECONDS,
    DOCUMENT_VISION_TIMEOUT_SECONDS,
    MAX_SCANNED_PDF_PAGES,
    RetryableDocumentError,
    convert_document_to_markdown,
    process_document,
)
from enrichment.user_files import download_user_file
from analytics import (
    cluster_activities,
    compute_readiness,
    handle_readiness_message,
)

logger = logging.getLogger(__name__)

MAX_RETRIES = 3
MAX_CONCURRENT_PER_USER = 2
MAX_CONCURRENT_ANALYTICS_PER_USER = 1
MAX_GLOBAL_CONCURRENT = 5
MAX_CONCURRENT_PHOTO_EMBED = 2
DOCUMENT_ACK_WAIT_SECONDS = int(os.getenv("ML_DOCUMENT_ACK_WAIT_SECONDS", "900"))
DOCUMENT_MARKDOWN_TIMEOUT_SECONDS = float(
    os.getenv(
        "ML_DOCUMENT_MARKDOWN_TIMEOUT_SECONDS",
        str((MAX_SCANNED_PDF_PAGES * DOCUMENT_PAGE_MARKDOWN_TIMEOUT_SECONDS) + 120),
    )
)
DOCUMENT_PROCESS_TIMEOUT_SECONDS = float(
    os.getenv(
        "ML_DOCUMENT_PROCESS_TIMEOUT_SECONDS",
        str((MAX_SCANNED_PDF_PAGES * DOCUMENT_VISION_TIMEOUT_SECONDS) + 120),
    )
)
DOCUMENT_IN_PROGRESS_INTERVAL_SECONDS = float(os.getenv("ML_DOCUMENT_IN_PROGRESS_INTERVAL_SECONDS", "60"))
MAX_PHOTO_EMBED_IMAGE_BYTES = int(os.getenv("ML_PHOTO_EMBED_IMAGE_MAX_BYTES", str(50 << 20)))
MAX_PHOTO_EMBED_VIDEO_BYTES = int(os.getenv("ML_PHOTO_EMBED_VIDEO_MAX_BYTES", str(500 << 20)))
FULL_ONLY_STORAGE_NAK_DELAY_SECONDS = int(os.getenv("ML_FULL_ONLY_STORAGE_NAK_DELAY_SECONDS", "30"))

# Global semaphore to prevent resource exhaustion under burst load
_global_semaphore = asyncio.Semaphore(MAX_GLOBAL_CONCURRENT)

# Cap and TTL for per-user semaphore maps.
# At 100k+ users, unbounded defaultdicts would accumulate hundreds of thousands
# of Semaphore objects. These constants bound memory: max 10 000 entries, idle
# entries (unused for >1 h) are evicted lazily at access time.
_SEM_MAP_CAP = 10_000
_SEM_MAP_TTL = 3_600  # seconds


class _BoundedSemaphoreMap:
    """OrderedDict-backed map of per-user asyncio.Semaphore objects.

    Eviction strategy (C — lazy TTL + LRU-Cap):
    - Every __getitem__ moves the key to the end (LRU touch) and records the
      current time as the last-access timestamp.
    - After the touch, if the map exceeds *cap* entries, the oldest 50 % of
      entries that are *not* currently acquired are evicted.  Acquired
      semaphores (Semaphore.locked() == True, i.e. value==0) are never evicted —
      evicting an acquired semaphore would leave awaiting coroutines
      permanently blocked.
    - TTL is checked opportunistically during eviction: entries whose last
      access is older than *ttl* seconds are preferred candidates even when the
      map is below *cap*.

    Thread-safety: single asyncio event-loop required; do not access from
    multiple loops.  No asyncio.Lock needed because mutation only happens in
    the single event loop's coroutines.
    """

    def __init__(self, initial_value: int, cap: int = _SEM_MAP_CAP, ttl: float = _SEM_MAP_TTL) -> None:
        self._initial = initial_value
        self._cap = cap
        self._ttl = ttl
        # Maps user_id → (semaphore, last_access_ts)
        self._store: OrderedDict[str, tuple[asyncio.Semaphore, float]] = OrderedDict()

    def __getitem__(self, key: str) -> asyncio.Semaphore:
        now = time.monotonic()
        if key in self._store:
            sem, _ = self._store[key]
            self._store.move_to_end(key)
            self._store[key] = (sem, now)
        else:
            sem = asyncio.Semaphore(self._initial)
            self._store[key] = (sem, now)
            self._store.move_to_end(key)
            self._maybe_evict(now)
        return sem

    def _maybe_evict(self, now: float) -> None:
        """Evict stale / over-cap entries that are not currently acquired.

        Uses the public asyncio.Semaphore.locked() API (stable since Python 3.10)
        rather than the private _value attribute to determine whether a semaphore
        is safe to evict.  A semaphore is evictable when it is not locked
        (i.e. at least one permit is free and no waiter is blocked on it).
        """
        if len(self._store) > 2 * self._cap:
            logger.warning(
                "BoundedSemaphoreMap above 2x cap (%d entries, cap=%d) — "
                "possibly leaked semaphores from stuck workers",
                len(self._store), self._cap,
            )

        if len(self._store) <= self._cap:
            # Even below cap, evict TTL-expired idle entries to free memory
            # progressively rather than waiting for a burst.
            stale = [
                k for k, (s, ts) in self._store.items()
                if (now - ts) > self._ttl and not s.locked()
            ]
            for k in stale:
                del self._store[k]
            return

        # Over cap: evict oldest 50 % of evictable entries.
        target = len(self._store) - self._cap // 2
        evicted = 0
        to_delete = []
        for k, (s, _) in self._store.items():
            if evicted >= target:
                break
            if not s.locked():  # not currently acquired (public API, no _value)
                to_delete.append(k)
                evicted += 1
        for k in to_delete:
            del self._store[k]

    def __len__(self) -> int:
        return len(self._store)

    def __contains__(self, key: object) -> bool:
        return key in self._store


# Per-user semaphores for rate limiting (bounded: max _SEM_MAP_CAP entries).
# Single asyncio event-loop required; do not access from multiple loops.
_user_semaphores = _BoundedSemaphoreMap(MAX_CONCURRENT_PER_USER)

# Per-user analytics semaphores (bounded: max _SEM_MAP_CAP entries).
# Single asyncio event-loop required; do not access from multiple loops.
_analytics_semaphores = _BoundedSemaphoreMap(MAX_CONCURRENT_ANALYTICS_PER_USER)

# Global semaphore for photo embedding (CPU/GPU-bound CLIP inference)
_photo_embed_semaphore = asyncio.Semaphore(MAX_CONCURRENT_PHOTO_EMBED)


def _full_only_processing_disabled() -> bool:
    return os.getenv("SKADIA_DEPLOYMENT_PROFILE", "full").strip() == "core"


async def _storage_read_ready_for_full_processing() -> tuple[bool, dict]:
    storage_readiness = await get_storage_readiness_async()
    return storage_readiness_ready(storage_readiness), storage_readiness


async def _stop_if_full_processing_unavailable(msg, label: str) -> bool:
    if _full_only_processing_disabled():
        logger.info("%s: skipped because SKADIA_DEPLOYMENT_PROFILE=core", label)
        await msg.ack()
        return True
    ready, storage_readiness = await _storage_read_ready_for_full_processing()
    if not ready:
        logger.warning("%s: storage_read not ready: %s", label, storage_readiness)
        await msg.nak(delay=FULL_ONLY_STORAGE_NAK_DELAY_SECONDS)
        return True
    return False

# Metrics counters
_metrics = {
    "active_tasks": 0,
    "processed_total": 0,
    "errors_total": 0,
}


def get_metrics() -> dict:
    return dict(_metrics)


def _valid_uuid(value: object) -> str | None:
    try:
        parsed = uuid.UUID(str(value))
    except (TypeError, ValueError, AttributeError):
        return None
    return str(parsed)


def _looks_like_video(path: str, content_type: str = "") -> bool:
    ext = os.path.splitext((path or "").lower())[1]
    video_mimetypes = {
        "video/mp4",
        "video/quicktime",
        "video/x-msvideo",
        "video/x-matroska",
        "video/webm",
    }
    return (
        ext in {".mp4", ".mov", ".avi", ".mkv", ".webm", ".m4v"}
        or (content_type or "").lower() in video_mimetypes
    )


async def _load_document_file_metadata(pool, file_id: str, user_id: str, doc_id: str) -> dict | None:
    """Load current user_files storage metadata for a document-processing event."""
    if not file_id or not user_id:
        return None
    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            SELECT uf.storage_backend, uf.storage_path, uf.file_type, uf.content_type, uf.size_bytes
            FROM user_files uf
            JOIN gear_documents gd
              ON gd.file_id = uf.id
             AND gd.user_id = uf.user_id
             AND gd.id = $3
            WHERE uf.id = $1 AND uf.user_id = $2 AND uf.status = 'active'
            """,
            file_id,
            user_id,
            doc_id,
        )
    if not row:
        return None
    return dict(row)


async def _load_photo_file_metadata(pool, file_id: str, user_id: str, photo_id: str) -> dict | None:
    """Load current user_files storage metadata for a photo embedding event."""
    if not file_id or not user_id or not photo_id:
        return None
    async with pool.acquire() as conn:
        row = await conn.fetchrow(
            """
            SELECT uf.storage_backend, uf.storage_path, uf.file_type, uf.content_type, uf.size_bytes
            FROM user_files uf
            JOIN activity_photos ap
              ON ap.file_id = uf.id
             AND ap.user_id = uf.user_id
             AND ap.id = $3
            WHERE uf.id = $1 AND uf.user_id = $2 AND uf.status = 'active'
            """,
            file_id,
            user_id,
            photo_id,
        )
    if not row:
        return None
    return dict(row)


async def _update_document_processing_status(
    pool,
    status: str,
    doc_id: str,
    user_id: str,
    file_id: str | None = None,
) -> None:
    async with pool.acquire() as conn:
        if file_id:
            await conn.execute(
                """
                UPDATE gear_documents
                SET processing_status = $1
                WHERE id = $2 AND user_id = $3 AND file_id = $4
                """,
                status,
                doc_id,
                user_id,
                file_id,
            )
        else:
            await conn.execute(
                """
                UPDATE gear_documents
                SET processing_status = $1
                WHERE id = $2 AND user_id = $3
                """,
                status,
                doc_id,
                user_id,
                )


async def _complete_document_import_log(
    pool,
    doc_id: str,
    user_id: str,
    file_id: str,
    success: bool,
    markdown_status: str = "",
) -> None:
    if not doc_id or not user_id:
        return
    if markdown_status not in ("completed", "failed", "skipped"):
        markdown_status = ""
    status = "completed" if success else "error"
    step_status = "completed" if success else "failed"
    error_msg = None if success else "document processing failed"
    async with pool.acquire() as conn:
        await conn.execute(
            """
            UPDATE import_log AS l
            SET status = $4,
                error_msg = $5,
                duration_ms = GREATEST(0, (EXTRACT(EPOCH FROM (NOW() - l.created_at)) * 1000)::int),
                metadata = COALESCE(l.metadata, '{}'::jsonb) || jsonb_build_object(
                  'document_processed', jsonb_build_object('success', $3::boolean, 'file_id', NULLIF($2, ''), 'markdown_status', NULLIF($8, '')),
                  'steps', COALESCE((
                    SELECT jsonb_agg(
                      CASE
                        WHEN step->>'key' = 'markdown' AND $8 <> '' THEN jsonb_set(step, '{status}', to_jsonb($8::text), true)
                        WHEN step->>'status' = 'pending' THEN jsonb_set(step, '{status}', to_jsonb($6::text), true)
                        ELSE step
                      END ORDER BY ordinality
                    )
                    FROM jsonb_array_elements(COALESCE(l.metadata->'steps', '[]'::jsonb)) WITH ORDINALITY AS s(step, ordinality)
                  ), '[]'::jsonb)
                )
            WHERE l.user_id = $7
              AND l.import_type = 'document'
              AND l.status IN ('pending', 'processing')
              AND l.metadata->>'target_id' = $1
              AND ($2 = '' OR l.metadata->>'file_id' = $2)
            """,
            doc_id,
            file_id or "",
            success,
            status,
            error_msg,
            step_status,
            user_id,
            markdown_status,
        )


async def _run_with_in_progress_heartbeat(msg, awaitable, timeout_seconds: float):
    heartbeat = getattr(msg, "in_progress", None)
    if heartbeat is None:
        return await asyncio.wait_for(awaitable, timeout=timeout_seconds)

    done = asyncio.Event()

    async def _heartbeat_loop() -> None:
        while not done.is_set():
            try:
                await asyncio.wait_for(done.wait(), timeout=DOCUMENT_IN_PROGRESS_INTERVAL_SECONDS)
            except asyncio.TimeoutError:
                try:
                    await heartbeat()
                except Exception as e:
                    logger.warning("document in-progress heartbeat failed: %s", e)

    task = asyncio.create_task(_heartbeat_loop())
    try:
        return await asyncio.wait_for(awaitable, timeout=timeout_seconds)
    finally:
        done.set()
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task


@contextlib.contextmanager
def _track_active():
    """Increment active_tasks on entry and decrement on exit (always)."""
    _metrics["active_tasks"] += 1
    try:
        yield
    finally:
        _metrics["active_tasks"] -= 1


def _should_give_up(msg) -> bool:
    """Return True on the JetStream delivery that reaches the configured max."""
    num_delivered = getattr(getattr(msg, "metadata", None), "num_delivered", None)
    retry_count = num_delivered if num_delivered is not None else (MAX_RETRIES + 1)
    return retry_count >= MAX_RETRIES


async def _decode_or_ack(msg, label: str) -> "dict | None":
    """Decode JSON from a NATS message. On failure, ack and return None."""
    try:
        return json.loads(msg.data.decode())
    except (json.JSONDecodeError, UnicodeDecodeError):
        logger.error("Invalid %s payload: %s", label, msg.data)
        await msg.ack()
        return None


# Mapping of NATS subjects to enrichment functions.
# Lift detection was removed in Phase 7 — moved to Go pipeline (RunStreamEnrichment).
HIGH_PRIORITY_STEPS: list = []

NORMAL_PRIORITY_STEPS = []

LOW_PRIORITY_STEPS = []


async def _process_message(msg, steps: list, nc: nats.NATS) -> None:
    """Process a single NATS message with the given enrichment steps."""
    data = await _decode_or_ack(msg, "message")
    if data is None:
        return

    activity_id = data.get("activity_id", "")
    user_id = data.get("user_id", "")
    if not activity_id or not user_id:
        logger.error("Missing activity_id or user_id in message")
        await msg.ack()
        return

    try:
        uuid.UUID(str(activity_id))
        uuid.UUID(str(user_id))
    except (ValueError, AttributeError):
        logger.warning(
            "Invalid UUID in NATS message: activity_id=%s user_id=%s",
            activity_id, user_id,
        )
        await msg.ack()
        return

    # enabled_modules: empty list (or missing) means all modules run.
    # Non-empty list means only the listed modules run (backwards-compatible).
    enabled_modules: list[str] = data.get("enabled_modules") or []

    sem = _user_semaphores[user_id]
    async with sem:
        with _track_active():
            try:
                pool = await get_pool()
                for step_name, step_fn in steps:
                    if enabled_modules and step_name not in enabled_modules:
                        logger.info(
                            "Skipping module %s for activity %s: not in enabled_modules %s",
                            step_name,
                            activity_id,
                            enabled_modules,
                        )
                        continue
                    try:
                        await publish_progress(
                            nc, activity_id, step_name, "running", user_id=user_id
                        )
                        await step_fn(pool, activity_id, user_id)
                        await publish_progress(
                            nc, activity_id, step_name, "done", user_id=user_id
                        )
                    except Exception as e:
                        logger.error(
                            "Enrichment step %s failed for activity %s: %s",
                            step_name,
                            activity_id,
                            e,
                        )
                        await publish_progress(
                            nc, activity_id, step_name, "error", str(e), user_id=user_id
                        )
                        # Continue with remaining steps even if one fails

                await msg.ack()
                _metrics["processed_total"] += 1
            except Exception as e:
                logger.error(
                    "Fatal error processing activity %s: %s", activity_id, e
                )
                _metrics["errors_total"] += 1

                if _should_give_up(msg):
                    logger.error(
                        "Max retries (%d) reached for activity %s, giving up",
                        MAX_RETRIES,
                        activity_id,
                    )
                    await msg.ack()
                    # Log to error table
                    try:
                        pool = await get_pool()
                        async with pool.acquire() as conn:
                            await conn.execute(
                                """
                                INSERT INTO enrichment_errors
                                    (activity_id, module, error_message)
                                VALUES ($1, 'stage2', $2)
                                ON CONFLICT DO NOTHING
                                """,
                                activity_id,
                                str(e),
                            )
                    except Exception:
                        pass
                else:
                    await msg.nak()


async def _process_document_message(msg, nc: nats.NATS) -> None:
    """Process a document-uploaded NATS message."""
    data = await _decode_or_ack(msg, "document message")
    if data is None:
        return

    doc_id = data.get("doc_id", "")
    user_id = data.get("user_id", "")
    file_id = data.get("file_id", "")

    valid_doc_id = _valid_uuid(doc_id)
    valid_user_id = _valid_uuid(user_id)
    valid_file_id = _valid_uuid(file_id)
    if not valid_doc_id or not valid_user_id:
        logger.error("Malformed document message IDs: doc_id=%s user_id=%s", doc_id, user_id)
        await msg.ack()
        return
    if not valid_file_id:
        logger.warning(
            "Ignoring legacy or malformed document message without valid file_id: doc_id=%s user_id=%s",
            valid_doc_id,
            valid_user_id,
        )
        try:
            pool = await get_pool()
            await _update_document_processing_status(pool, "failed", valid_doc_id, valid_user_id)
            await _complete_document_import_log(pool, valid_doc_id, valid_user_id, "", False)
        except Exception as e:
            logger.warning("failed to mark legacy document message as failed for %s: %s", valid_doc_id, e)
        await msg.ack()
        return

    if await _stop_if_full_processing_unavailable(msg, "document processing"):
        return

    doc_id = valid_doc_id
    user_id = valid_user_id
    file_id = valid_file_id

    sem = _user_semaphores[user_id]
    async with sem:
        with _track_active():
            markdown_status = ""
            try:
                pool = await get_pool()
                storage_metadata = await _load_document_file_metadata(pool, file_id, user_id, doc_id)
                if storage_metadata is None:
                    logger.error("document message missing active user_files metadata for %s", doc_id)
                    await _update_document_processing_status(pool, "failed", doc_id, user_id, file_id)
                    await _complete_document_import_log(pool, doc_id, user_id, file_id, False)
                    await msg.ack()
                    return
                markdown_status = "skipped"
                extracted_text = data.get("extracted_text", "") or ""
                existing_markdown = ""
                try:
                    pool = await get_pool()
                    async with pool.acquire() as conn:
                        doc_row = await conn.fetchrow(
                            "SELECT extracted_text, extracted_markdown FROM gear_documents WHERE id = $1 AND user_id = $2 AND file_id = $3",
                            doc_id,
                            user_id,
                            file_id,
                        )
                    if doc_row:
                        doc_row = dict(doc_row)
                        existing_markdown = doc_row.get("extracted_markdown") or ""
                        if not extracted_text:
                            extracted_text = doc_row.get("extracted_text") or ""
                except Exception as e:
                    logger.warning("failed to load extracted document content from DB for %s: %s", doc_id, e)

                if existing_markdown:
                    markdown = existing_markdown
                    markdown_status = "completed"
                    logger.info("reusing extracted_markdown for document %s", doc_id)
                else:
                    markdown = await _run_with_in_progress_heartbeat(
                        msg,
                        convert_document_to_markdown(
                            extracted_text=extracted_text,
                            storage_backend=storage_metadata["storage_backend"],
                            storage_path=storage_metadata["storage_path"],
                            file_type=storage_metadata["file_type"],
                            content_type=storage_metadata["content_type"],
                            size_bytes=storage_metadata.get("size_bytes"),
                        ),
                        DOCUMENT_MARKDOWN_TIMEOUT_SECONDS,
                    )
                if markdown and not existing_markdown:
                    async with pool.acquire() as conn:
                        await conn.execute(
                            "UPDATE gear_documents SET extracted_markdown = $1 WHERE id = $2 AND user_id = $3 AND file_id = $4",
                            markdown,
                            doc_id,
                            user_id,
                            file_id,
                        )
                    logger.info("stored extracted_markdown for document %s", doc_id)
                    markdown_status = "completed"
                elif not markdown:
                    markdown_status = "failed"

                extraction_text = markdown or extracted_text
                success = await _run_with_in_progress_heartbeat(
                    msg,
                    process_document(
                        pool=pool,
                        doc_id=doc_id,
                        gear_id=data.get("gear_id"),
                        component_id=data.get("component_id"),
                        user_id=user_id,
                        is_text_pdf=bool(markdown) or data.get("is_text_pdf", False),
                        extracted_text=extraction_text,
                        file_id=file_id,
                        storage_backend=storage_metadata["storage_backend"],
                        storage_path=storage_metadata["storage_path"],
                        file_type=storage_metadata["file_type"],
                        content_type=storage_metadata["content_type"],
                        size_bytes=storage_metadata.get("size_bytes"),
                    ),
                    DOCUMENT_PROCESS_TIMEOUT_SECONDS,
                )
                logger.info("document processed: %s (success=%s)", doc_id, success)

                await _update_document_processing_status(
                    pool,
                    "processed" if success else "failed",
                    doc_id,
                    user_id,
                    file_id,
                )

                await _complete_document_import_log(pool, doc_id, user_id, file_id, success, markdown_status)

                try:
                    await nc.publish(
                        "skadia.document.processed",
                        json.dumps({
                            "doc_id": doc_id,
                            "user_id": user_id,
                            "gear_id": data.get("gear_id"),
                            "file_id": file_id,
                            "success": success,
                            "markdown_status": markdown_status,
                        }).encode(),
                    )
                except Exception as e:
                    logger.warning("failed to publish processed event for document %s: %s", doc_id, e)

                await msg.ack()
                _metrics["processed_total"] += 1
            except Exception as e:
                logger.error("document processing failed for %s: %s", doc_id, e)
                _metrics["errors_total"] += 1

                give_up = _should_give_up(msg)
                if not give_up:
                    if isinstance(e, RetryableDocumentError):
                        logger.info("document processing will be retried for %s", doc_id)
                    await msg.nak(delay=30)
                    return

                try:
                    pool = await get_pool()
                    await _update_document_processing_status(pool, "failed", doc_id, user_id, file_id)
                    await _complete_document_import_log(pool, doc_id, user_id, file_id, False, markdown_status)
                except Exception as status_err:
                    logger.warning("failed to mark document %s as failed: %s", doc_id, status_err)

                logger.error(
                    "Max retries (%d) reached for document %s, giving up",
                    MAX_RETRIES,
                    doc_id,
                )
                await msg.ack()


class _JetStreamAnalyticsPublisher:
    """Thin wrapper around the JetStream context that exposes the
    `publish_analytics_invalidate` contract expected by self-writing
    analytics modules (e.g. readiness pref-race re-publish).
    """

    __slots__ = ("_js",)

    def __init__(self, js):
        self._js = js

    async def publish_analytics_invalidate(self, user_id: str) -> None:
        await self._js.publish(
            "skadia.analytics.invalidate",
            json.dumps({"user_id": user_id}).encode(),
        )


async def _process_analytics_message(msg, nc: nats.NATS, publisher=None) -> None:
    """Verarbeitet ein skadia.analytics.invalidate Event.

    Ruft alle Analytics-Module für den Nutzer auf und speichert die
    Ergebnisse in analytics_cache. Fehler einzelner Module stoppen
    nicht die anderen (graceful degradation).

    `publisher` is injected for self-writing modules (readiness uses it for
    pref-race re-publish). When omitted, a default JetStream publisher is
    derived from `nc` — kept overridable so tests can substitute a fake.
    """
    data = await _decode_or_ack(msg, "analytics")
    if data is None:
        return

    user_id = data.get("user_id", "")
    if not user_id:
        logger.error("analytics: missing user_id in message")
        await msg.ack()
        return

    if publisher is None:
        publisher = _JetStreamAnalyticsPublisher(nc.jetstream())

    # Per-user Semaphore: max 1 gleichzeitiger Analytics-Job pro Nutzer
    sem = _analytics_semaphores[user_id]
    async with sem:
        with _track_active():
            try:
                pool = await get_pool()

                # Acquire a single connection for all analytics modules
                # to avoid exhausting the pool (previously up to 9 connections)
                async with pool.acquire() as conn:
                    # Each tuple: (cache_key, module_fn, self_writes)
                    # self_writes=True means the module wrote analytics_cache
                    # itself (in its own transaction); caller MUST NOT
                    # _upsert_analytics_cache afterwards.
                    modules = [
                        ("clusters",  cluster_activities,       False),
                        ("readiness", handle_readiness_message, True),
                    ]

                    # Flag set by any self-writing module that requests a NAK
                    # (e.g. readiness already_running — another worker holds the
                    # lock; redelivery after 2 s lets the lock-holder finish).
                    nak_delay: int | None = None

                    for cache_key, module_fn, self_writes in modules:
                        try:
                            if self_writes:
                                mod_result = await module_fn(conn, user_id, publisher)
                                # already_running: another compute is in flight.
                                # NAK so this message is redelivered once the
                                # lock-holder has committed. 2 s is intentional:
                                # readiness compute is typically <1 s; 2 s gives
                                # enough headroom without overloading the queue.
                                if (
                                    isinstance(mod_result, dict)
                                    and mod_result.get("refusal_reason") == "already_running"
                                ):
                                    nak_delay = 2
                            else:
                                result = await module_fn(conn, user_id)
                                await _upsert_analytics_cache(conn, user_id, cache_key, result)
                            logger.debug(
                                "analytics: computed %s for user %s", cache_key, user_id
                            )
                        except Exception as e:
                            # Fehler dieses Moduls loggen, andere weiterlaufen lassen
                            logger.error(
                                "analytics: module %r failed for user %s: %s",
                                cache_key,
                                user_id,
                                e,
                            )

                if nak_delay is not None:
                    logger.debug(
                        "analytics: readiness already_running for user %s — NAK delay=%ds",
                        user_id,
                        nak_delay,
                    )
                    await msg.nak(delay=nak_delay)
                    return

                await msg.ack()
                _metrics["processed_total"] += 1
                logger.info("analytics: recomputed all modules for user %s", user_id)
            except Exception as e:
                logger.error("analytics: fatal error for user %s: %s", user_id, e)
                _metrics["errors_total"] += 1

                if _should_give_up(msg):
                    logger.error(
                        "analytics: max retries (%d) reached for user %s, giving up",
                        MAX_RETRIES,
                        user_id,
                    )
                    await msg.ack()
                else:
                    await msg.nak(delay=60)


async def _upsert_analytics_cache(conn, user_id: str, cache_key: str, result) -> None:
    """Schreibt ein Analytics-Ergebnis in die analytics_cache Tabelle (UPSERT)."""
    await conn.execute(
        """
        INSERT INTO analytics_cache (user_id, cache_key, computed_at, data)
        VALUES ($1, $2, NOW(), $3::jsonb)
        ON CONFLICT (user_id, cache_key) DO UPDATE
        SET data = EXCLUDED.data, computed_at = EXCLUDED.computed_at
        """,
        user_id,
        cache_key,
        json.dumps(result),
    )


async def _download_from_s3(bucket: str, key: str) -> bytes:
    """Download an object from S3-compatible storage using aioboto3."""
    session = aioboto3.Session()
    async with session.client("s3", **s3_client_kwargs()) as s3:
        response = await s3.get_object(Bucket=bucket, Key=key)
        return await response["Body"].read()


def _s3_bucket_for_file_type(file_type: str) -> str:
    """Return the S3 bucket used by ShadowFS for a file type."""
    default_bucket = os.getenv("S3_BUCKET", "skadia-originals")
    if file_type in {"activity", "health"}:
        return os.getenv("S3_BUCKET_ACTIVITY") or default_bucket
    if file_type in {"media", "document", "thumb", "preview"}:
        return os.getenv("S3_BUCKET_MEDIA") or default_bucket
    return default_bucket


async def _process_photo_embed(msg, pool, nc) -> None:
    """Process a photo embedding request from skadia.photo.embed."""
    data = await _decode_or_ack(msg, "photo_embed")
    if data is None:
        return

    valid_photo_id = _valid_uuid(data.get("photo_id", ""))
    valid_user_id = _valid_uuid(data.get("user_id", ""))
    valid_file_id = _valid_uuid(data.get("file_id", ""))
    if not valid_photo_id or not valid_user_id or not valid_file_id:
        logger.warning("photo_embed: ignoring malformed or legacy message")
        await msg.ack()
        return
    photo_id = valid_photo_id
    user_id = valid_user_id
    file_id = valid_file_id

    if await _stop_if_full_processing_unavailable(msg, "photo_embed"):
        return

    # Respect per-user setting — skip if user has disabled photo CLIP embedding
    if pool:
        async with pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT enable_photo_clip_embedding FROM user_enrichment_settings WHERE user_id = $1::uuid",
                user_id,
            )
        if row is not None and not row["enable_photo_clip_embedding"]:
            logger.debug("photo_embed: skipped for photo %s (user disabled)", photo_id)
            await msg.ack()
            return

    storage_metadata = await _load_photo_file_metadata(pool, file_id, user_id, photo_id)
    if storage_metadata is None:
        logger.warning("photo_embed: missing active user_files metadata for photo %s", photo_id)
        await msg.ack()
        return

    content_type = storage_metadata.get("content_type") or data.get("content_type", "")
    storage_path = storage_metadata["storage_path"]
    is_video_file = _looks_like_video(storage_path, content_type)
    max_bytes = MAX_PHOTO_EMBED_VIDEO_BYTES if is_video_file else MAX_PHOTO_EMBED_IMAGE_BYTES

    async with _photo_embed_semaphore:
        with _track_active():
            try:
                from models.clip import embed_image, embed_video_bytes, is_video  # lazy imports

                image_bytes = await download_user_file(
                    storage_metadata["storage_backend"],
                    storage_path,
                    storage_metadata["file_type"],
                    size_bytes=storage_metadata.get("size_bytes"),
                    max_bytes=max_bytes,
                    label="photo",
                )
                if not image_bytes:
                    logger.warning("photo_embed: storage bytes unavailable for photo %s", photo_id)
                    await msg.ack()
                    return

                if is_video_file or is_video(storage_path, content_type):
                    num_frames = max(1, min(10, int(data.get("video_clip_frames", 4) or 4)))
                    logger.info("photo_embed: video detected for photo %s, extracting %d frame(s)", photo_id, num_frames)
                    embedding = await embed_video_bytes(image_bytes, num_frames=num_frames)
                else:
                    embedding = await embed_image(image_bytes)

                # Store as pgvector literal string
                vec_str = "[" + ",".join(f"{v:.6f}" for v in embedding) + "]"
                async with pool.acquire() as conn:
                    result = await conn.execute(
                        "UPDATE activity_photos SET clip_embedding = $1::vector WHERE id = $2::uuid AND user_id = $3::uuid AND file_id = $4::uuid",
                        vec_str,
                        photo_id,
                        user_id,
                        file_id,
                    )
                if result != "UPDATE 1":
                    logger.warning("photo_embed: stale photo/file binding for photo %s file %s", photo_id, file_id)
                    await msg.ack()
                    return
                logger.info("photo_embed: embedding saved for photo %s", photo_id)
                await msg.ack()
                _metrics["processed_total"] += 1
            except Exception as e:
                logger.error(
                    "photo_embed: failed for photo %s: %s", data.get("photo_id", "?"), e
                )
                _metrics["errors_total"] += 1

                if _should_give_up(msg):
                    logger.error(
                        "photo_embed: max retries (%d) reached for photo %s, giving up — %s",
                        MAX_RETRIES,
                        photo_id,
                        e,
                    )
                    # Permanently mark as failed so catch-up doesn't re-queue this photo.
                    # Uses enrichment_errors (activity_id scope) to avoid a new migration.
                    try:
                        async with pool.acquire() as conn:
                            row = await conn.fetchrow(
                                "SELECT activity_id FROM activity_photos WHERE id = $1::uuid",
                                photo_id,
                            )
                            if row and row["activity_id"]:
                                existing = await conn.fetchval(
                                    "SELECT COUNT(*) FROM enrichment_errors "
                                    "WHERE activity_id = $1 AND module = 'clip_embedding'",
                                    row["activity_id"],
                                )
                                if not existing:
                                    await conn.execute(
                                        "INSERT INTO enrichment_errors "
                                        "(activity_id, module, error_message) "
                                        "VALUES ($1, 'clip_embedding', $2)",
                                        row["activity_id"],
                                        f"Photo {photo_id}: {e}",
                                    )
                    except Exception as db_err:
                        logger.warning("photo_embed: failed to write enrichment_error: %s", db_err)
                    await msg.ack()
                else:
                    await msg.nak()


async def _catchup_missing_embeddings(pool, js) -> None:
    """Every 60 s: publish skadia.photo.embed for photos without a CLIP embedding.

    Also manages a background_jobs entry per user so the UI ActiveJobsIndicator
    shows progress. Uses INSERT … ON CONFLICT to upsert the job row.
    """
    while True:
        try:
            if _full_only_processing_disabled():
                logger.debug("photo_embed catch-up: skipped because SKADIA_DEPLOYMENT_PROFILE=core")
                await asyncio.sleep(60)
                continue
            ready, storage_readiness = await _storage_read_ready_for_full_processing()
            if not ready:
                logger.warning("photo_embed catch-up: storage_read not ready: %s", storage_readiness)
                await asyncio.sleep(60)
                continue

            async with pool.acquire() as conn:
                # --- per-user stats (total + missing) ---
                stats = await conn.fetch(
                    "SELECT ap.user_id::text AS user_id, "
                    "COUNT(*) AS total, "
                    "COUNT(*) FILTER ("
                    "  WHERE ap.clip_embedding IS NULL "
                    "  AND NOT EXISTS ("
                    "    SELECT 1 FROM enrichment_errors ee "
                    "    WHERE ee.activity_id = ap.activity_id AND ee.module = 'clip_embedding'"
                    "  )"
                    ") AS missing "
                    "FROM activity_photos ap "
                    "JOIN user_enrichment_settings ues ON ues.user_id = ap.user_id "
                    "WHERE ues.enable_photo_clip_embedding = true "
                    "GROUP BY ap.user_id"
                )

                users_with_missing = {
                    s["user_id"]: {"total": s["total"], "missing": s["missing"]}
                    for s in stats
                    if s["missing"] > 0
                }
                all_user_ids = {s["user_id"] for s in stats}

                # --- upsert/update background_jobs per user with pending work ---
                for user_id, counts in users_with_missing.items():
                    done = counts["total"] - counts["missing"]
                    total = counts["total"]
                    # Update existing running job OR insert a new one
                    updated = await conn.execute(
                        "UPDATE background_jobs "
                        "SET progress_current = $2, progress_total = $3 "
                        "WHERE user_id = $1 AND job_type = 'photo_clip_embed' "
                        "AND status = 'running'",
                        user_id, done, total,
                    )
                    if updated == "UPDATE 0":
                        await conn.execute(
                            "INSERT INTO background_jobs "
                            "(user_id, job_type, status, progress_current, progress_total) "
                            "VALUES ($1, 'photo_clip_embed', 'running', $2, $3)",
                            user_id, done, total,
                        )

                # --- complete jobs for users whose queue is now empty ---
                await conn.execute(
                    "UPDATE background_jobs "
                    "SET status = 'completed', finished_at = NOW() "
                    "WHERE job_type = 'photo_clip_embed' AND status = 'running' "
                    "AND user_id NOT IN (SELECT DISTINCT user_id::text FROM activity_photos "
                    "WHERE clip_embedding IS NULL)"
                )

                # --- publish next batch (LIMIT 50) ---
                # Exclude permanently failed photos (enrichment_errors module='clip_embedding').
                # All other formats (image/*, video/*, HEIC, JXL) are now supported.
                rows = await conn.fetch(
                    "SELECT ap.id, ap.user_id, ap.file_id, ap.content_type, "
                    "ues.video_clip_frames "
                    "FROM activity_photos ap "
                    "JOIN user_files uf ON uf.id = ap.file_id AND uf.user_id = ap.user_id "
                    "JOIN user_enrichment_settings ues ON ues.user_id = ap.user_id "
                    "WHERE ap.clip_embedding IS NULL "
                    "AND uf.status = 'active' "
                    "AND ues.enable_photo_clip_embedding = true "
                    "AND NOT EXISTS ("
                    "  SELECT 1 FROM enrichment_errors ee "
                    "  WHERE ee.activity_id = ap.activity_id "
                    "  AND ee.module = 'clip_embedding'"
                    ") "
                    "LIMIT 50"
                )

            if rows:
                for row in rows:
                    payload = {
                        "photo_id": str(row["id"]),
                        "user_id": str(row["user_id"]),
                        "file_id": str(row["file_id"]),
                        "content_type": row.get("content_type") or "",
                        "video_clip_frames": row.get("video_clip_frames") or 4,
                    }
                    await js.publish(
                        "skadia.photo.embed",
                        json.dumps(payload).encode(),
                    )
                logger.debug(
                    "photo_embed catch-up: queued %d photo embeddings", len(rows)
                )
        except Exception as e:
            logger.warning("photo_embed catch-up scan failed: %s", e)
        await asyncio.sleep(60)


async def _ensure_stream(js, name: str, subjects: list[str]) -> None:
    """Create a JetStream stream if it doesn't exist."""
    try:
        await js.find_stream_name_by_subject(subjects[0])
    except nats.js.errors.NotFoundError:
        await js.add_stream(name=name, subjects=subjects, retention="workqueue")


async def _pull_loop(sub, handler, name: str) -> None:
    """Generic pull-consumer loop: fetch one message at a time, dispatch to handler.

    Pull consumers don't bind exclusively to a subscription, so they survive
    service restarts without "consumer is already bound" errors.

    On cancellation (shutdown), the done_callback on each handler task ensures
    the global semaphore is released even if the task is cancelled mid-flight.
    """
    logger.info("Pull consumer %s started", name)
    try:
        while True:
            try:
                msgs = await sub.fetch(batch=1, timeout=5)
                for msg in msgs:
                    # Guard: if cancelled between acquire and create_task,
                    # the finally below won't help — so wrap tightly.
                    try:
                        await _global_semaphore.acquire()
                        task = asyncio.create_task(handler(msg))
                        task.add_done_callback(lambda _: _global_semaphore.release())
                    except asyncio.CancelledError:
                        _global_semaphore.release()
                        raise
            except nats.errors.TimeoutError:
                pass  # No messages available — poll again
            except asyncio.CancelledError:
                raise
            except Exception as e:
                logger.error("Pull consumer %s fetch error: %s", name, e)
                await asyncio.sleep(1)
    except asyncio.CancelledError:
        logger.info("Pull consumer %s shutting down", name)


async def start_consumers(nc: nats.NATS) -> list[asyncio.Task]:
    """Start JetStream pull consumers for all queues.

    Returns a list of asyncio.Tasks (one per consumer loop) that can be
    cancelled on shutdown. Uses pull-based consumers instead of push-based
    to avoid "consumer is already bound" errors on restart.
    """
    js = nc.jetstream()
    tasks: list[asyncio.Task] = []

    # --- Stage 2 enrichment (3 priority queues) ---
    await _ensure_stream(js, "stage2", ["skadia.stage2.>"])

    for subject, durable, steps, name in [
        ("skadia.stage2.high", "ml-stage2-high", HIGH_PRIORITY_STEPS, "stage2-high"),
        ("skadia.stage2.normal", "ml-stage2-normal", NORMAL_PRIORITY_STEPS, "stage2-normal"),
        ("skadia.stage2.low", "ml-stage2-low", LOW_PRIORITY_STEPS, "stage2-low"),
    ]:
        if not steps:
            continue
        sub = await js.pull_subscribe(
            subject,
            durable=durable,
            config=ConsumerConfig(
                deliver_policy=DeliverPolicy.ALL,
                ack_policy=AckPolicy.EXPLICIT,
                max_deliver=MAX_RETRIES,
            ),
        )
        handler = lambda msg, s=steps: _process_message(msg, s, nc)
        tasks.append(asyncio.create_task(_pull_loop(sub, handler, name)))

    # --- Document processing ---
    await _ensure_stream(js, "document", ["skadia.document.>"])

    sub_doc = await js.pull_subscribe(
        "skadia.document.uploaded",
        durable="ml-document-processor",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES,
            ack_wait=DOCUMENT_ACK_WAIT_SECONDS,
        ),
    )
    tasks.append(asyncio.create_task(
        _pull_loop(sub_doc, lambda msg: _process_document_message(msg, nc), "document")
    ))

    # --- Analytics invalidation ---
    await _ensure_stream(js, "analytics", ["skadia.analytics.>"])

    sub_analytics = await js.pull_subscribe(
        "skadia.analytics.invalidate",
        durable="ml-analytics-invalidate",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES,
            ack_wait=300,
        ),
    )
    tasks.append(asyncio.create_task(
        _pull_loop(sub_analytics, lambda msg: _process_analytics_message(msg, nc), "analytics")
    ))

    # --- Photo CLIP embedding ---
    # Uses the existing SKADIA stream (skadia.> wildcard already covers skadia.photo.embed).
    await _ensure_stream(js, "SKADIA", ["skadia.>"])

    pool = await get_pool()

    # --- Startup cleanup: mark orphaned running jobs as failed ---
    # Any background_job left in status='running' at startup was interrupted
    # by a previous container crash/restart and will never complete on its own.
    try:
        async with pool.acquire() as conn:
            n = await conn.execute(
                "UPDATE background_jobs "
                "SET status = 'failed', finished_at = NOW() "
                "WHERE status = 'running' AND job_type = 'photo_clip_embed'",
            )
        logger.info("Startup cleanup: marked orphaned photo_clip_embed jobs as failed (%s)", n)
    except Exception as e:
        logger.warning("Startup cleanup: failed to mark orphaned jobs: %s", e)

    sub_photo = await js.pull_subscribe(
        "skadia.photo.embed",
        durable="ml-photo-embed",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES,
            ack_wait=120,
        ),
    )
    tasks.append(asyncio.create_task(
        _pull_loop(
            sub_photo,
            lambda msg: _process_photo_embed(msg, pool, nc),
            "photo-embed",
        )
    ))

    # Periodic catch-up: re-queue photos whose embedding is still missing
    tasks.append(asyncio.create_task(_catchup_missing_embeddings(pool, js)))

    logger.info("All NATS pull consumers started (%d tasks)", len(tasks))
    return tasks
