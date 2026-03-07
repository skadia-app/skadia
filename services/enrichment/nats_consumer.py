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
)
from enrichment.document_vlm import process_document
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

    sem = _user_semaphores[user_id]
    async with sem:
        _metrics["active_tasks"] += 1
        try:
            pool = await get_pool()
            for step_name, step_fn in steps:
                try:
                    await publish_progress(
                        nc, activity_id, step_name, "running"
                    )
                    await step_fn(pool, activity_id, user_id)
                    await publish_progress(
                        nc, activity_id, step_name, "done"
                    )
                except Exception as e:
                    logger.error(
                        "Enrichment step %s failed for activity %s: %s",
                        step_name,
                        activity_id,
                        e,
                    )
                    await publish_progress(
                        nc, activity_id, step_name, "error", str(e)
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

            # Definiere alle zu berechnenden Module: (cache_key, coroutine)
            modules = [
                ("readiness", compute_readiness(pool, user_id)),
                ("anomalies", detect_anomalies(pool, user_id)),
                ("clusters", cluster_activities(pool, user_id)),
                ("projection", compute_projection(pool, user_id)),
                ("correlations", compute_correlations(pool, user_id)),
            ]

            for cache_key, coro in modules:
                try:
                    result = await coro
                    await _upsert_analytics_cache(pool, user_id, cache_key, result)
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


async def _upsert_analytics_cache(pool, user_id: str, cache_key: str, result) -> None:
    """Schreibt ein Analytics-Ergebnis in die analytics_cache Tabelle (UPSERT)."""
    await pool.execute(
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


async def start_consumers(nc: nats.NATS) -> list:
    """Start JetStream consumers for all three priority queues."""
    js = nc.jetstream()

    # Ensure the stream exists
    try:
        await js.find_stream_name_by_subject("skadia.stage2.>")
    except nats.js.errors.NotFoundError:
        await js.add_stream(
            name="stage2",
            subjects=["skadia.stage2.>"],
            retention="workqueue",
        )

    subscriptions = []

    # High priority consumer
    sub_high = await js.subscribe(
        "skadia.stage2.high",
        durable="ml-stage2-high",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES + 1,
        ),
    )
    subscriptions.append(sub_high)

    # Normal priority consumer
    sub_normal = await js.subscribe(
        "skadia.stage2.normal",
        durable="ml-stage2-normal",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES + 1,
        ),
    )
    subscriptions.append(sub_normal)

    # Low priority consumer
    sub_low = await js.subscribe(
        "skadia.stage2.low",
        durable="ml-stage2-low",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES + 1,
        ),
    )
    subscriptions.append(sub_low)

    # Start message processing loops
    async def _consume(sub, steps, name):
        logger.info("Consumer %s started", name)
        async for msg in sub.messages:
            asyncio.create_task(_process_message(msg, steps, nc))

    asyncio.create_task(_consume(sub_high, HIGH_PRIORITY_STEPS, "high"))
    asyncio.create_task(_consume(sub_normal, NORMAL_PRIORITY_STEPS, "normal"))
    asyncio.create_task(_consume(sub_low, LOW_PRIORITY_STEPS, "low"))

    # Document processing stream + consumer
    try:
        await js.find_stream_name_by_subject("skadia.document.>")
    except nats.js.errors.NotFoundError:
        await js.add_stream(
            name="document",
            subjects=["skadia.document.>"],
            retention="workqueue",
        )

    sub_doc = await js.subscribe(
        "skadia.document.uploaded",
        durable="ml-document-processor",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES + 1,
            ack_wait=120,
        ),
    )
    subscriptions.append(sub_doc)

    async def _consume_documents(sub):
        logger.info("Consumer document-processor started")
        async for msg in sub.messages:
            asyncio.create_task(_process_document_message(msg, nc))

    asyncio.create_task(_consume_documents(sub_doc))

    # Analytics invalidation stream + consumer
    try:
        await js.find_stream_name_by_subject("skadia.analytics.>")
    except nats.js.errors.NotFoundError:
        await js.add_stream(
            name="analytics",
            subjects=["skadia.analytics.>"],
            retention="workqueue",
        )

    sub_analytics = await js.subscribe(
        "skadia.analytics.invalidate",
        durable="ml-analytics-invalidate",
        config=ConsumerConfig(
            deliver_policy=DeliverPolicy.ALL,
            ack_policy=AckPolicy.EXPLICIT,
            max_deliver=MAX_RETRIES + 1,
            ack_wait=300,  # Analytics kann länger dauern (5 min)
        ),
    )
    subscriptions.append(sub_analytics)

    async def _consume_analytics(sub):
        logger.info("Consumer analytics-invalidate started")
        async for msg in sub.messages:
            asyncio.create_task(_process_analytics_message(msg, nc))

    asyncio.create_task(_consume_analytics(sub_analytics))

    logger.info("All NATS consumers started")
    return subscriptions
