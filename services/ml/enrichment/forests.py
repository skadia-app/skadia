import asyncpg
import json
import logging

logger = logging.getLogger(__name__)

# Minimum intersection length in meters to count a forest
MIN_LENGTH_M = 200


async def enrich_forests(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Find forests intersected by the activity track.

    Uses ST_Intersects on osm_forests and filters by minimum intersection length.
    Writes results to activity_geo_tags with category='forest'.

    Returns the number of forests found.
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
            ),
            intersected AS (
                SELECT f.osm_id, f.name,
                       ST_Intersection(route.geom, f.geom) AS isect,
                       ST_XMin(f.geom) AS bbox_west, ST_YMin(f.geom) AS bbox_south,
                       ST_XMax(f.geom) AS bbox_east, ST_YMax(f.geom) AS bbox_north
                FROM osm_forests f, route
                WHERE ST_Intersects(f.geom, route.geom)
            )
            SELECT osm_id, name,
                   ROUND(ST_Length(isect::geography))::int AS length_m,
                   bbox_west, bbox_south, bbox_east, bbox_north
            FROM intersected
            WHERE ST_Length(isect::geography) >= $2
            ORDER BY length_m DESC
            """,
            activity_id,
            MIN_LENGTH_M,
        )

        if not rows:
            return 0

        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'forest'",
            activity_id,
        )

        for row in rows:
            meta = {}

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north,
                     length_m, meta)
                VALUES ($1, 'forest', $2, $3, 'bbox', $4, $5, $6, $7, $8, $9)
                """,
                activity_id,
                row["name"] or "Wald",
                row["osm_id"],
                row["bbox_west"],
                row["bbox_south"],
                row["bbox_east"],
                row["bbox_north"],
                row["length_m"],
                json.dumps(meta),
            )

        logger.info(
            "Enriched %d forests for activity %s", len(rows), activity_id
        )
        return len(rows)
