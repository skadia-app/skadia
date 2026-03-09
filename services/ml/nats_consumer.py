import asyncio
import json
import logging
import os
from collections import defaultdict

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

# Metrics counters
_metrics = {
    "active_tasks": 0,
    "processed_total": 0,
    "errors_total": 0,
}


def get_metrics() -> dict:
    return dict(_metrics)


# Mapping of NATS subjects to enrichment functions
HIGH_PRIORITY_STEPS = [
    ("peaks", enrich_peaks),
    ("lifts", enrich_lifts),
    ("settlements", enrich_settlements),
    ("commute", enrich_commute),
    ("borders", enrich_borders),
]

NORMAL_PRIORITY_STEPS = [
    ("water", enrich_water),
    ("forests", enrich_forests),
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

            # Check retry count via metadata
            retry_count = msg.metadata.num_delivered if hasattr(msg, "metadata") else 0
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

            retry_count = msg.metadata.num_delivered if hasattr(msg, "metadata") else 0
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

            retry_count = msg.metadata.num_delivered if hasattr(msg, "metadata") else 0
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

    logger.info("All NATS pull consumers started (%d tasks)", len(tasks))
    return tasks
