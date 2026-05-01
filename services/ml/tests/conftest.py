"""Shared fixtures for ml service tests.

Most tests use the `conn` fixture: it opens an asyncpg connection, starts a
transaction, yields the connection, and rolls back at the end — every test
sees pristine DB state. Schema is the live dev Postgres (no migrations
inside tests).

Concurrency tests need two real connections that both see committed rows;
they use `concurrent_pool` and clean up explicitly.
"""
from __future__ import annotations

import os
import uuid
import pytest
import pytest_asyncio
import asyncpg


def _dsn() -> str | None:
    return os.environ.get("TEST_DATABASE_URL")


@pytest_asyncio.fixture
async def conn():
    dsn = _dsn()
    if not dsn:
        pytest.skip("TEST_DATABASE_URL not set")
    c = await asyncpg.connect(dsn)
    tx = c.transaction()
    await tx.start()
    try:
        yield c
    finally:
        try:
            await tx.rollback()
        finally:
            await c.close()


@pytest_asyncio.fixture
async def concurrent_pool():
    """Real pool that COMMITS rows. Tests using this must clean up themselves."""
    dsn = _dsn()
    if not dsn:
        pytest.skip("TEST_DATABASE_URL not set")
    pool = await asyncpg.create_pool(dsn, min_size=2, max_size=4)
    try:
        yield pool
    finally:
        await pool.close()


async def seed_user(conn) -> str:
    """Insert a fresh users row, return UUID."""
    uid = str(uuid.uuid4())
    await conn.execute(
        """
        INSERT INTO users (id, username, units, locale)
        VALUES ($1::uuid, $2, 'metric', 'de')
        """,
        uid, f"test_{uid[:8]}",
    )
    return uid
