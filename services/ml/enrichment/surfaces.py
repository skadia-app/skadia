import asyncpg
import logging

logger = logging.getLogger(__name__)

# Buffer in meters for surface matching
BUFFER_M = 10


async def enrich_surfaces(
    pool: asyncpg.Pool, activity_id: str, user_id: str
) -> int:
    """Detect surface types along the activity track.

    Matches the track against osm_ways with a surface tag using ST_DWithin (10m).
    Calculates the percentage of each surface type along the route.
    Writes results to activity_surfaces table.

    Returns the number of surface types found.
    """
    async with pool.acquire() as conn:
        track = await conn.fetchval(
            "SELECT track_full FROM activity_tracks WHERE activity_id = $1",
            activity_id,
        )
        if track is None:
            return 0

        rows = await conn.fetch(
            """
            WITH route AS (
                SELECT track_full AS geom,
                       ST_Length(track_full::geography) AS total_length_m
                FROM activity_tracks WHERE activity_id = $1
            ),
            matched AS (
                SELECT w.surface, w.highway,
                       ST_Length(ST_Intersection(
                           ST_Buffer(route.geom::geography, $2)::geometry, w.geom
                       )::geography) AS match_length_m,
                       route.total_length_m
                FROM osm_ways w, route
                WHERE ST_DWithin(route.geom::geography, w.geom::geography, $2)
                  AND w.surface IS NOT NULL
            )
            SELECT surface, highway,
                   ROUND(SUM(match_length_m))::int AS total_m,
                   ROUND(SUM(match_length_m) / MAX(total_length_m) * 100, 1) AS pct
            FROM matched
            GROUP BY surface, highway
            HAVING SUM(match_length_m) > 10
            ORDER BY total_m DESC
            """,
            activity_id,
            BUFFER_M,
        )

        if not rows:
            return 0

        # Delete existing surface data for re-enrichment
        await conn.execute(
            "DELETE FROM activity_surfaces WHERE activity_id = $1",
            activity_id,
        )

        for row in rows:
            await conn.execute(
                """
                INSERT INTO activity_surfaces
                    (activity_id, surface_type, path_type, length_m, percentage)
                VALUES ($1, $2, $3, $4, $5)
                """,
                activity_id,
                row["surface"],
                row["highway"] or "unknown",
                float(row["total_m"]),
                float(row["pct"]),
            )

        logger.info(
            "Enriched %d surface types for activity %s",
            len(rows),
            activity_id,
        )
        return len(rows)
