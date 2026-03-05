import asyncpg
import json
import logging

logger = logging.getLogger(__name__)


async def enrich_borders(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Detect border crossings along the activity track.

    Uses ST_Crosses on osm_boundaries (admin_level 2 = country, 4 = state).
    Writes results to activity_geo_tags with category='border'.

    Returns the number of border crossings found.
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
                SELECT track_full AS geom
                FROM activity_tracks WHERE activity_id = $1
            )
            SELECT b.name, b.admin_level, b.country_code,
                   ST_Intersection(route.geom, ST_Boundary(b.geom)) AS crossing_geom
            FROM osm_boundaries b, route
            WHERE ST_Crosses(route.geom, ST_Boundary(b.geom))
              AND b.admin_level IN (2, 4)
            ORDER BY b.admin_level
            """,
            activity_id,
        )

        if not rows:
            return 0

        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'border'",
            activity_id,
        )

        for row in rows:
            name = row["name"] or "Grenze"
            meta = {}
            if row["admin_level"] is not None:
                meta["admin_level"] = row["admin_level"]
            if row["country_code"]:
                meta["country_code"] = row["country_code"]

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, map_type, map_section, meta)
                VALUES ($1, 'border', $2, 'section', $3, $4)
                """,
                activity_id,
                name,
                row["crossing_geom"],
                json.dumps(meta),
            )

        logger.info(
            "Enriched %d border crossings for activity %s",
            len(rows),
            activity_id,
        )
        return len(rows)
