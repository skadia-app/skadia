import asyncpg
import json
import logging

logger = logging.getLogger(__name__)


async def enrich_settlements(
    pool: asyncpg.Pool, activity_id: str, user_id: str
) -> int:
    """Find settlements along the activity track.

    Node settlements (hamlets without polygon) use a 200m buffer.
    Area settlements use ST_Intersects.

    Writes results to activity_geo_tags with category='settlement'.
    Returns the number of settlements found.
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
            SELECT s.osm_id, s.name, s.subtype AS place,
                   ST_Y(ST_Centroid(s.geom)) AS center_lat,
                   ST_X(ST_Centroid(s.geom)) AS center_lon
            FROM osm_pois s, route
            WHERE ST_DWithin(route.geom::geography, s.geom::geography, 200)
              AND s.category = 'settlement'
            ORDER BY s.subtype, s.name
            """,
            activity_id,
        )

        if not rows:
            return 0

        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'settlement'",
            activity_id,
        )

        for row in rows:
            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_point_lat, map_point_lon, meta)
                VALUES ($1, 'settlement', $2, $3, 'point', $4, $5, $6)
                """,
                activity_id,
                row["name"],
                row["osm_id"],
                row["center_lat"],
                row["center_lon"],
                json.dumps({"place": row["place"]} if row["place"] else {}),
            )

        logger.info(
            "Enriched %d settlements for activity %s", len(rows), activity_id
        )
        return len(rows)
