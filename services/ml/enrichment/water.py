import asyncpg
import logging

logger = logging.getLogger(__name__)

# Default buffer in meters for water proximity detection
DEFAULT_BUFFER_M = 50


async def enrich_water(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Find water bodies and waterways near the activity track.

    Still water (osm_waters): ST_DWithin with 50m buffer → category='still_water'
    Flowing water (osm_waterways): ST_DWithin with 50m buffer → category='flowing_water'

    Returns total number of water features found.
    """
    count = 0
    async with pool.acquire() as conn:
        track = await conn.fetchval(
            "SELECT track_full FROM activity_tracks WHERE activity_id = $1",
            activity_id,
        )
        if track is None:
            return 0

        # Delete existing water geo tags for re-enrichment
        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category IN ('still_water', 'flowing_water')",
            activity_id,
        )

        # Still water (lakes, reservoirs)
        still_rows = await conn.fetch(
            """
            WITH route AS (
                SELECT track_full AS geom
                FROM activity_tracks WHERE activity_id = $1
            )
            SELECT DISTINCT ON (w.osm_id) w.osm_id, w.name,
                   ROUND(ST_Distance(route.geom::geography, w.geom::geography))::int AS min_dist_m,
                   ST_XMin(w.geom) AS bbox_west, ST_YMin(w.geom) AS bbox_south,
                   ST_XMax(w.geom) AS bbox_east, ST_YMax(w.geom) AS bbox_north,
                   ROUND(ST_Area(w.geom::geography))::bigint AS area_m2
            FROM osm_waters w, route
            WHERE ST_DWithin(route.geom::geography, w.geom::geography, $2)
              AND w.name IS NOT NULL
            ORDER BY w.osm_id, min_dist_m
            """,
            activity_id,
            DEFAULT_BUFFER_M,
        )

        for row in still_rows:
            meta = {}
            if row["area_m2"]:
                meta["area_m2"] = row["area_m2"]

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_bbox_west, map_bbox_south, map_bbox_east, map_bbox_north,
                     dist_m, meta)
                VALUES ($1, 'still_water', $2, $3, 'bbox', $4, $5, $6, $7, $8, $9)
                """,
                activity_id,
                row["name"],
                row["osm_id"],
                row["bbox_west"],
                row["bbox_south"],
                row["bbox_east"],
                row["bbox_north"],
                row["min_dist_m"],
                meta,
            )
        count += len(still_rows)

        # Flowing water (rivers, streams)
        flow_rows = await conn.fetch(
            """
            WITH route AS (
                SELECT track_full AS geom
                FROM activity_tracks WHERE activity_id = $1
            )
            SELECT DISTINCT ON (ww.osm_id) ww.osm_id, ww.name, ww.waterway,
                   ROUND(ST_Distance(route.geom::geography, ww.geom::geography))::int AS min_dist_m,
                   ST_Intersection(
                       ST_Buffer(route.geom::geography, $2)::geometry, ww.geom
                   ) AS nearby_section
            FROM osm_waterways ww, route
            WHERE ST_DWithin(route.geom::geography, ww.geom::geography, $2)
              AND ww.name IS NOT NULL
            ORDER BY ww.osm_id, min_dist_m
            """,
            activity_id,
            DEFAULT_BUFFER_M,
        )

        for row in flow_rows:
            meta = {"waterway": row["waterway"]} if row["waterway"] else {}

            await conn.execute(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_section, dist_m, meta)
                VALUES ($1, 'flowing_water', $2, $3, 'section', $4, $5, $6)
                """,
                activity_id,
                row["name"],
                row["osm_id"],
                row["nearby_section"],
                row["min_dist_m"],
                meta,
            )
        count += len(flow_rows)

    logger.info("Enriched %d water features for activity %s", count, activity_id)
    return count
