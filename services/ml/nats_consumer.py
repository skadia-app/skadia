import asyncio
import json
import logging
import os
import uuid
from collections import defaultdict

import aioboto3
import nats
from nats.js.api import ConsumerConfig, DeliverPolicy, AckPolicy

from db import get_pool
from progress import publish_progress
from enrichment import (
    enrich_peaks,
    enrich_settlements,
    enrich_commute,
    enrich_borders,
    enrich_water,
    enrich_forests,
    enrich_surfaces,
    enrich_stops,
    enrich_ascents,
    compute_route_embedding,
    enrich_lifts,
    enrich_intervals,
)
from enrichment.document_vlm import convert_to_markdown, process_document, upload_to_s3
from analytics import (
    compute_readiness,
    detect_anomalies,
    cluster_activities,
    compute_projection,
    compute_correlations,
)

logger = logging.getLogger(__name__)

MAX_RETRIES = 3
MAX_CONCURRENT_PER_USER = 2
MAX_CONCURRENT_ANALYTICS_PER_USER = 1
MAX_GLOBAL_CONCURRENT = 5
MAX_CONCURRENT_PHOTO_EMBED = 2

# Global semaphore to prevent resource exhaustion under burst load
_global_semaphore = asyncio.Semaphore(MAX_GLOBAL_CONCURRENT)

# Per-user semaphores for rate limiting
_user_semaphores: dict[str, asyncio.Semaphore] = defaultdict(
    lambda: asyncio.Semaphore(MAX_CONCURRENT_PER_USER)
)

# Per-user analytics semaphores (max 1 concurrent analytics job per user)
_analytics_semaphores: dict[str, asyncio.Semaphore] = defaultdict(
    lambda: asyncio.Semaphore(MAX_CONCURRENT_ANALYTICS_PER_USER)
)

# Global semaphore for photo embedding (CPU/GPU-bound CLIP inference)
_photo_embed_semaphore = asyncio.Semaphore(MAX_CONCURRENT_PHOTO_EMBED)

# Metrics counters
_metrics = {
    "active_tasks": 0,
    "processed_total": 0,
    "errors_total": 0,
}


def get_metrics() -> dict:
    return dict(_metrics)


# Mapping of NATS subjects to enrichment functions
#
# NOTE: The following 5 modules have been migrated to Go SQL (geo_enrich.go)
# and are executed synchronously in Stage 1 before this NATS event is emitted:
#   peaks, settlements, forests, water, borders
#
# stage1.go always excludes these names from EnabledModules, so the
# enabled_modules filter in _process_message() will skip them even if the
# Python functions remain listed here.  They are kept for reference only.
HIGH_PRIORITY_STEPS = [
    ("peaks", enrich_peaks),          # Migrated to Go (geo_enrich.go) — skipped via EnabledModules
    ("lifts", enrich_lifts),
    ("settlements", enrich_settlements),  # Migrated to Go (geo_enrich.go) — skipped via EnabledModules
    ("commute", enrich_commute),
    ("borders", enrich_borders),      # Migrated to Go (geo_enrich.go) — skipped via EnabledModules
]

NORMAL_PRIORITY_STEPS = [
    ("water", enrich_water),          # Migrated to Go (geo_enrich.go) — skipped via EnabledModules
    ("forests", enrich_forests),      # Migrated to Go (geo_enrich.go) — skipped via EnabledModules
    ("surfaces", enrich_surfaces),
    ("stops", enrich_stops),
    ("intervals", enrich_intervals),
]

LOW_PRIORITY_STEPS = [
    ("ascents", enrich_ascents),
    ("embeddings", compute_route_embedding),
]


async def _process_message(msg, steps: list, nc: nats.NATS) -> None:
    """Process a single NATS message with the given enrichment steps."""
    try:
        data = json.loads(msg.data.decode())
    except (json.JSONDecodeError, UnicodeDecodeError):
        logger.error("Invalid message payload: %s", msg.data)
        await msg.ack()
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
        _metrics["active_tasks"] += 1
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

            # Check retry count via metadata (metadata attribute may be None or absent)
            num_delivered = getattr(getattr(msg, "metadata", None), "num_delivered", None)
            retry_count = num_delivered if num_delivered is not None else (MAX_RETRIES + 1)
            if retry_count >= MAX_RETRIES:
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
        finally:
            _metrics["active_tasks"] -= 1


async def _process_document_message(msg, nc: nats.NATS) -> None:
    """Process a document-uploaded NATS message."""
    try:
        data = json.loads(msg.data.decode())
    except (json.JSONDecodeError, UnicodeDecodeError):
        logger.error("Invalid document message payload: %s", msg.data)
        await msg.ack()
        return

    doc_id = data.get("doc_id", "")
    user_id = data.get("user_id", "")
    if not doc_id or not user_id:
        logger.error("Missing doc_id or user_id in document message")
        await msg.ack()
        return

    sem = _user_semaphores[user_id]
    async with sem:
        _metrics["active_tasks"] += 1
        try:
            pool = await get_pool()
            success = await process_document(
                pool=pool,
                doc_id=doc_id,
                gear_id=data.get("gear_id"),
                component_id=data.get("component_id"),
                user_id=user_id,
                is_text_pdf=data.get("is_text_pdf", False),
                extracted_text=data.get("extracted_text"),
                s3_bucket=data.get("s3_bucket", ""),
                s3_key=data.get("s3_key", ""),
            )
            await msg.ack()
            _metrics["processed_total"] += 1
            logger.info("document processed: %s (success=%s)", doc_id, success)

            # PDF→Markdown Konvertierung (best-effort)
            if success:
                extracted_text = data.get("extracted_text", "") or ""
                if not extracted_text:
                    try:
                        pool = await get_pool()
                        async with pool.acquire() as conn:
                            extracted_text = (
                                await conn.fetchval(
                                    "SELECT extracted_text FROM gear_documents WHERE id = $1",
                                    doc_id,
                                )
                                or ""
                            )
                    except Exception as e:
                        logger.warning("failed to load extracted_text from DB for %s: %s", doc_id, e)

                if extracted_text:
                    try:
                        markdown = await convert_to_markdown(extracted_text)
                        if markdown:
                            pool = await get_pool()
                            async with pool.acquire() as conn:
                                await conn.execute(
                                    "UPDATE gear_documents SET extracted_markdown = $1 WHERE id = $2",
                                    markdown,
                                    doc_id,
                                )
                            logger.info("stored extracted_markdown for document %s", doc_id)

                            s3_key = data.get("s3_key", "")
                            s3_bucket = data.get("s3_bucket", "skadia-media")
                            if s3_key:
                                md_key = s3_key.rsplit(".", 1)[0] + ".md"
                                try:
                                    await upload_to_s3(
                                        s3_bucket,
                                        md_key,
                                        markdown.encode("utf-8"),
                                        "text/markdown",
                                    )
                                    logger.info("uploaded markdown sidecar: %s", md_key)
                                except Exception as e:
                                    logger.warning("failed to upload markdown sidecar %s: %s", md_key, e)
                    except Exception as e:
                        logger.warning("markdown conversion failed for document %s: %s", doc_id, e)

            # Publish document.processed event
            try:
                await nc.publish(
                    "skadia.document.processed",
                    json.dumps({
                        "doc_id": doc_id,
                        "user_id": user_id,
                        "gear_id": data.get("gear_id"),
                        "success": success,
                    }).encode(),
                )
            except Exception as e:
                logger.warning("failed to publish document.processed: %s", e)
        except Exception as e:
            logger.error("document processing failed for %s: %s", doc_id, e)
            _metrics["errors_total"] += 1

            num_delivered = getattr(getattr(msg, "metadata", None), "num_delivered", None)
            retry_count = num_delivered if num_delivered is not None else (MAX_RETRIES + 1)
            if retry_count >= MAX_RETRIES:
                logger.error(
                    "Max retries (%d) reached for document %s, giving up",
                    MAX_RETRIES,
                    doc_id,
                )
                await msg.ack()
            else:
                await msg.nak(delay=30)
        finally:
            _metrics["active_tasks"] -= 1


async def _process_analytics_message(msg, nc: nats.NATS) -> None:
    """Verarbeitet ein skadia.analytics.invalidate Event.

    Ruft alle Analytics-Module für den Nutzer auf und speichert die
    Ergebnisse in analytics_cache. Fehler einzelner Module stoppen
    nicht die anderen (graceful degradation).
    """
    try:
        data = json.loads(msg.data.decode())
    except (json.JSONDecodeError, UnicodeDecodeError):
        logger.error("analytics: invalid message payload: %s", msg.data)
        await msg.ack()
        return

    user_id = data.get("user_id", "")
    if not user_id:
        logger.error("analytics: missing user_id in message")
        await msg.ack()
        return

    # Per-user Semaphore: max 1 gleichzeitiger Analytics-Job pro Nutzer
    sem = _analytics_semaphores[user_id]
    async with sem:
        _metrics["active_tasks"] += 1
        try:
            pool = await get_pool()

            # Acquire a single connection for all analytics modules
            # to avoid exhausting the pool (previously up to 9 connections)
            async with pool.acquire() as conn:
                # Definiere alle zu berechnenden Module: (cache_key, factory)
                modules = [
                    ("readiness", compute_readiness),
                    ("anomalies", detect_anomalies),
                    ("clusters", cluster_activities),
                    ("projection", compute_projection),
                    ("correlations", compute_correlations),
                ]

                for cache_key, module_fn in modules:
                    try:
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

            await msg.ack()
            _metrics["processed_total"] += 1
            logger.info("analytics: recomputed all modules for user %s", user_id)
        except Exception as e:
            logger.error("analytics: fatal error for user %s: %s", user_id, e)
            _metrics["errors_total"] += 1

            num_delivered = getattr(getattr(msg, "metadata", None), "num_delivered", None)
            retry_count = num_delivered if num_delivered is not None else (MAX_RETRIES + 1)
            if retry_count >= MAX_RETRIES:
                logger.error(
                    "analytics: max retries (%d) reached for user %s, giving up",
                    MAX_RETRIES,
                    user_id,
                )
                await msg.ack()
            else:
                await msg.nak(delay=60)
        finally:
            _metrics["active_tasks"] -= 1


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
    """Download an object from Garage S3 using aioboto3."""
    session = aioboto3.Session()
    async with session.client(
        "s3",
        endpoint_url=os.getenv("GARAGE_ENDPOINT", "http://garage:3900"),
        aws_access_key_id=os.getenv("GARAGE_ACCESS_KEY"),
        aws_secret_access_key=os.getenv("GARAGE_SECRET_KEY"),
    ) as s3:
        response = await s3.get_object(Bucket=bucket, Key=key)
        return await response["Body"].read()


async def _process_photo_embed(msg, pool, nc) -> None:
    """Process a photo embedding request from skadia.photo.embed."""
    data: dict = {}
    try:
        data = json.loads(msg.data.decode())
    except (json.JSONDecodeError, UnicodeDecodeError):
        logger.error("photo_embed: invalid message payload: %s", msg.data)
        await msg.ack()
        return

    photo_id = data.get("photo_id", "")
    user_id = data.get("user_id", "")
    s3_bucket = data.get("s3_bucket", "")
    if not photo_id or not s3_bucket:
        logger.error("photo_embed: missing photo_id or s3_bucket in message")
        await msg.ack()
        return

    # Respect per-user setting — skip if user has disabled photo CLIP embedding
    if user_id and pool:
        async with pool.acquire() as conn:
            row = await conn.fetchrow(
                "SELECT enable_photo_clip_embedding FROM user_enrichment_settings WHERE user_id = $1::uuid",
                user_id,
            )
        if row is not None and not row["enable_photo_clip_embedding"]:
            logger.debug("photo_embed: skipped for photo %s (user disabled)", photo_id)
            await msg.ack()
            return

    # Prefer preview (smaller), fall back to original
    image_key = data.get("s3_key_preview") or data.get("s3_key", "")
    if not image_key:
        logger.error("photo_embed: no s3_key in message for photo %s", photo_id)
        await msg.ack()
        return

    async with _photo_embed_semaphore:
        _metrics["active_tasks"] += 1
        try:
            from models.clip import embed_image, embed_video_bytes, is_video  # lazy imports

            image_bytes = await _download_from_s3(s3_bucket, image_key)

            if is_video(image_key, data.get("content_type", "")):
                logger.info("photo_embed: video detected for photo %s, extracting frames", photo_id)
                embedding = await embed_video_bytes(image_bytes)
            else:
                embedding = await embed_image(image_bytes)

            # Store as pgvector literal string
            vec_str = "[" + ",".join(f"{v:.6f}" for v in embedding) + "]"
            async with pool.acquire() as conn:
                await conn.execute(
                    "UPDATE activity_photos SET clip_embedding = $1::vector WHERE id = $2::uuid",
                    vec_str,
                    photo_id,
                )
            logger.info("photo_embed: embedding saved for photo %s", photo_id)
            await msg.ack()
            _metrics["processed_total"] += 1
        except Exception as e:
            logger.error(
                "photo_embed: failed for photo %s: %s", data.get("photo_id", "?"), e
            )
            _metrics["errors_total"] += 1

            num_delivered = getattr(getattr(msg, "metadata", None), "num_delivered", None)
            retry_count = num_delivered if num_delivered is not None else (MAX_RETRIES + 1)
            if retry_count >= MAX_RETRIES:
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
        finally:
            _metrics["active_tasks"] -= 1


async def _catchup_missing_embeddings(pool, js) -> None:
    """Every 60 s: publish skadia.photo.embed for photos without a CLIP embedding.

    Also manages a background_jobs entry per user so the UI ActiveJobsIndicator
    shows progress. Uses INSERT … ON CONFLICT to upsert the job row.
    """
    while True:
        try:
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
                    "SELECT ap.id, ap.user_id, ap.s3_bucket, ap.s3_key, ap.s3_key_preview, "
                    "ap.content_type "
                    "FROM activity_photos ap "
                    "JOIN user_enrichment_settings ues ON ues.user_id = ap.user_id "
                    "WHERE ap.clip_embedding IS NULL "
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
                        "s3_bucket": row["s3_bucket"],
                        "s3_key": row["s3_key"],
                        "s3_key_preview": row["s3_key_preview"],
                        "content_type": row.get("content_type") or "",
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
        sub = await js.pull_subscribe(
            subject,
            durable=durable,
            config=ConsumerConfig(
                deliver_policy=DeliverPolicy.ALL,
                ack_policy=AckPolicy.EXPLICIT,
                max_deliver=MAX_RETRIES + 1,
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
            max_deliver=MAX_RETRIES + 1,
            ack_wait=120,
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
            max_deliver=MAX_RETRIES + 1,
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
            max_deliver=MAX_RETRIES + 1,
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
