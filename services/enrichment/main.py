import logging
import os
from contextlib import asynccontextmanager

import nats
from fastapi import FastAPI

from db import get_pool, close_pool
from nats_consumer import start_consumers, get_metrics

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

_nc: nats.NATS | None = None
_subscriptions: list = []


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _nc, _subscriptions

    # Startup
    logger.info("ML service starting up")

    # Initialize database pool
    await get_pool()
    logger.info("Database pool initialized")

    # Connect to NATS
    nats_url = os.environ.get("NATS_URL", "nats://nats:4222")
    _nc = await nats.connect(nats_url)
    logger.info("Connected to NATS at %s", nats_url)

    # Start JetStream consumers
    _subscriptions = await start_consumers(_nc)

    yield

    # Shutdown
    logger.info("ML service shutting down")

    for sub in _subscriptions:
        await sub.unsubscribe()

    if _nc and not _nc.is_closed:
        await _nc.close()

    await close_pool()
    logger.info("ML service shut down complete")


app = FastAPI(title="Skadia ML Service", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.get("/metrics")
async def metrics():
    return get_metrics()
