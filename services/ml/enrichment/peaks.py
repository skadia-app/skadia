import asyncpg
import logging

logger = logging.getLogger(__name__)

# Default search radius in meters around the track
DEFAULT_RADIUS_M = 100


async def enrich_peaks(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Find peaks/passes within radius of the activity track via ST_DWithin.

    Uses the osm_peaks table (populated by osm2pgsql Flex) and writes results
    to activity_geo_tags with category 'summit' or 'pass'.

    Returns the number of peaks found.
    """
    async with pool.acquire() as conn:
        track = await conn.fetchval(
            "SELECT track_full FROM activity_tracks WHERE activity_id = $1",
            activity_id,
        )
        if track is None:
            logger.debug("No track found for activity %s", activity_id)
            return 0

        rows = await conn.fetch(
            """
            WITH route AS (
                SELECT track_full AS geom
                FROM activity_tracks WHERE activity_id = $1
            )
            SELECT p.osm_id, p.name, p.ele, p.type,
                   ROUND(ST_Distance(route.geom::geography, p.geom::geography))::int AS dist_m,
                   ST_Y(p.geom) AS lat, ST_X(p.geom) AS lon
            FROM osm_peaks p, route
            WHERE ST_DWithin(route.geom::geography, p.geom::geography, $2)
              AND p.name IS NOT NULL
            ORDER BY dist_m
            """,
            activity_id,
            DEFAULT_RADIUS_M,
        )

        if not rows:
            return 0

        # Delete existing geo tags for this activity+category to allow re-enrichment
        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('summit', 'pass')",
            activity_id,
        )

        for row in rows:
            # Map osm_peaks.type to geo_tag category
            category = "pass" if row["type"] in ("pass", "saddle") else "summit"
            meta = {}
            if row["dist_m"] is not None:
                meta["dist_m"] = row["dist_m"]

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_point_lat, map_point_lon, ele_m, dist_m, meta)
                VALUES ($1, $2, $3, $4, 'point', $5, $6, $7, $8, $9)
                """,
                activity_id,
                category,
                row["name"],
                row["osm_id"],
                row["lat"],
                row["lon"],
                row["ele"],
                row["dist_m"],
                meta,
            )

        logger.info(
            "Enriched %d peaks for activity %s", len(rows), activity_id
        )
        return len(rows)
