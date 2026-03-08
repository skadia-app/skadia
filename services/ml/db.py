import asyncpg
import os

_pool: asyncpg.Pool | None = None
_readonly_pool: asyncpg.Pool | None = None


async def get_pool() -> asyncpg.Pool:
    global _pool
    if _pool is None:
        _pool = await asyncpg.create_pool(
            os.environ["DATABASE_URL"], min_size=2, max_size=10
        )
    return _pool


async def get_readonly_pool() -> asyncpg.Pool:
    """Return a connection pool using the readonly database URL.

    Falls back to DATABASE_URL if READONLY_DATABASE_URL is not set
    (warning is emitted by config.py at import time).
    """
    global _readonly_pool
    if _readonly_pool is None:
        from config import READONLY_DATABASE_URL
        _readonly_pool = await asyncpg.create_pool(
            READONLY_DATABASE_URL, min_size=1, max_size=5
        )
    return _readonly_pool


async def close_pool() -> None:
    global _pool, _readonly_pool
    if _pool:
        await _pool.close()
        _pool = None
    if _readonly_pool:
        await _readonly_pool.close()
        _readonly_pool = None
