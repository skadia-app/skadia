import asyncpg
import logging

logger = logging.getLogger(__name__)


async def compute_route_embedding(
    pool: asyncpg.Pool, activity_id: str, user_id: str = ""
) -> None:
    """CPU-only BGE-M3 embedding for activity route.

    Currently disabled until GPU configuration is tested.
    When enabled, this will:
    1. Extract route features (sport type, geo tags, elevation profile)
    2. Generate a 1024-dim BGE-M3 embedding
    3. Store it in activities.embedding for similarity search
    """
    # TODO: Enable after GPU configuration
    # - Load BAAI/bge-m3 model
    # - Build text representation from activity metadata + geo tags
    # - Generate 1024-dim embedding
    # - UPDATE activities SET embedding = $2 WHERE id = $1
    logger.debug(
        "Route embedding skipped for activity %s (not yet enabled)",
        activity_id,
    )
