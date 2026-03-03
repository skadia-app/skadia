import asyncpg
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
            SELECT s.osm_id, s.name, s.type,
                   ST_Y(ST_Centroid(s.geom)) AS center_lat,
                   ST_X(ST_Centroid(s.geom)) AS center_lon,
                   CASE WHEN GeometryType(s.geom)='POINT' THEN NULL
                        ELSE ST_XMin(s.geom) END AS bbox_west,
                   CASE WHEN GeometryType(s.geom)='POINT' THEN NULL
                        ELSE ST_YMin(s.geom) END AS bbox_south,
                   CASE WHEN GeometryType(s.geom)='POINT' THEN NULL
                        ELSE ST_XMax(s.geom) END AS bbox_east,
                   CASE WHEN GeometryType(s.geom)='POINT' THEN NULL
                        ELSE ST_YMax(s.geom) END AS bbox_north
            FROM osm_settlements s, route
            WHERE (GeometryType(s.geom)='POINT'
                      AND ST_DWithin(route.geom::geography, s.geom::geography, 200))
               OR (GeometryType(s.geom)<>'POINT'
                      AND ST_Intersects(s.geom, route.geom))
            ORDER BY s.type, s.name
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
            has_bbox = row["bbox_west"] is not None
            map_type = "bbox" if has_bbox else "point"

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_point_lat, map_point_lon,
                     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north,
                     meta)
                VALUES ($1, 'settlement', $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
                """,
                activity_id,
                row["name"],
                row["osm_id"],
                map_type,
                row["center_lat"],
                row["center_lon"],
                row["bbox_west"],
                row["bbox_south"],
                row["bbox_east"],
                row["bbox_north"],
                {"type": row["type"]} if row["type"] else {},
            )

        logger.info(
            "Enriched %d settlements for activity %s", len(rows), activity_id
        )
        return len(rows)
