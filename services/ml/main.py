import logging
import os
from contextlib import asynccontextmanager

import nats
from fastapi import FastAPI
from fastapi.responses import JSONResponse
from pythonjsonlogger.json import JsonFormatter

from db import get_pool, close_pool
from nats_consumer import start_consumers, get_metrics

log_level = os.getenv("LOG_LEVEL", "INFO").upper()

_log_handler = logging.StreamHandler()
_log_handler.setFormatter(JsonFormatter(
    fmt="%(asctime)s %(levelname)s %(name)s %(message)s",
    rename_fields={"asctime": "timestamp", "levelname": "level", "name": "logger"},
))
logging.basicConfig(level=getattr(logging, log_level, logging.INFO), handlers=[_log_handler])
logger = logging.getLogger(__name__)

_nc: nats.NATS | None = None
_consumer_tasks: list = []


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _nc, _consumer_tasks

    # Startup
    logger.info("ML service starting up")

    # Initialize database pool
    await get_pool()
    logger.info("Database pool initialized")

    # Connect to NATS with resilience callbacks
    nats_url = os.environ.get("NATS_URL", "nats://nats:4222")

    async def _on_disconnect():
        logger.warning("NATS disconnected")

    async def _on_reconnect():
        logger.info("NATS reconnected")

    async def _on_error(e):
        logger.error("NATS error: %s", e)

    _nc = await nats.connect(
        nats_url,
        error_cb=_on_error,
        disconnected_cb=_on_disconnect,
        reconnected_cb=_on_reconnect,
        max_reconnect_attempts=-1,
        reconnect_time_wait=2,
    )
    logger.info("Connected to NATS at %s", nats_url)

    # Start JetStream pull consumers (returns asyncio.Task list)
    _consumer_tasks = await start_consumers(_nc)

    yield

    # Shutdown — cancel pull consumer tasks (no unsubscribe needed for pull)
    logger.info("ML service shutting down")

    for task in _consumer_tasks:
        task.cancel()

    if _nc and not _nc.is_closed:
        await _nc.close()

    await close_pool()
    logger.info("ML service shut down complete")


app = FastAPI(title="Skadia ML Service", lifespan=lifespan)


# Probe endpoints are registered FIRST — before any include_router() call below.
# FastAPI resolves routes in registration order; keeping probes first ensures no
# sub-router can ever shadow /livez, /readyz or /health even if someone later
# drops the prefix from api/health.py (which currently uses prefix="/ml").

@app.get("/livez")
async def livez():
    """Liveness probe: process is alive. Does NOT check dependencies."""
    return {"status": "ok"}


@app.get("/health")
async def health():
    """Alias for /livez — backwards compat for ml_probe.go capabilities check."""
    return {"status": "ok"}


@app.get("/readyz")
async def readyz():
    """Readiness probe: returns 200 if DB and NATS (if configured) are connected, 503 otherwise."""
    issues = {}

    # Check DB
    try:
        pool = await get_pool()
        if pool is not None:
            async with pool.acquire() as conn:
                await conn.execute("SELECT 1")
        else:
            issues["db"] = "not configured"
    except Exception as e:
        issues["db"] = str(e)

    # Check NATS (only if configured)
    try:
        if _nc is not None:
            if not _nc.is_connected:
                issues["nats"] = "disconnected"
        # If _nc is None, NATS isn't configured — skip check
    except Exception as e:
        issues["nats"] = str(e)

    if issues:
        return JSONResponse(status_code=503, content={"status": "not ready", **issues})
    return {"status": "ready"}


@app.get("/metrics")
async def metrics():
    return get_metrics()


from api.features import router as features_router
app.include_router(features_router)

from api.chat import router as chat_router
app.include_router(chat_router)

from api.health import router as health_router
app.include_router(health_router)

from api.forecast import router as forecast_router
app.include_router(forecast_router)

from api.embed import router as embed_router
app.include_router(embed_router)

# Defensive: verify no imported router shadowed our probe endpoints.
# Runs at import time — caught immediately if a probe path is accidentally removed
# or if a future sub-router registers a conflicting path before the probes.
_probe_paths = {"/livez", "/readyz", "/health"}
_registered_paths = {route.path for route in app.routes if hasattr(route, "path")}
for _path in _probe_paths:
    assert _path in _registered_paths, f"Probe endpoint {_path!r} missing from app routes — registration order broken"
