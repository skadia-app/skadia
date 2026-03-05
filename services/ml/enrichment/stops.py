import asyncpg
import logging

logger = logging.getLogger(__name__)

# Speed threshold in m/s below which a point is considered stationary
SPEED_THRESHOLD = 0.5
# Minimum stop duration in seconds
MIN_DURATION_S = 60
# Radius in meters for POI matching around a stop point
POI_RADIUS_M = 50
# Minimum stop duration in seconds to write a gastronomy geo tag
GASTRONOMY_MIN_DURATION_S = 600

# POI subtypes that qualify as gastronomic stops
GASTRONOMY_TYPES = {
    'restaurant', 'cafe', 'biergarten', 'fast_food', 'ice_cream', 'bar',
    'pub', 'food_court',                        # amenity
    'alpine_hut', 'guest_house', 'hostel', 'hotel',  # tourism
    'kiosk', 'bakery', 'butcher',               # shop
}


async def enrich_stops(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Detect stops in the activity and match them to nearby POIs.

    A stop is defined as consecutive points where speed < 0.5 m/s for > 60 seconds.
    All stop points are matched against osm_amenities in a single batch query via LATERAL.
    Writes results to activity_stops table.

    Returns the number of stops found.
    """
    async with pool.acquire() as conn:
        stream_rows = await conn.fetch(
            """
            SELECT time, lat, lng, speed_ms
            FROM activity_streams
            WHERE activity_id = $1
              AND lat IS NOT NULL AND lng IS NOT NULL
            ORDER BY time
            """,
            activity_id,
        )

        if not stream_rows:
            return 0

        # Detect stops: consecutive low-speed points
        stops = []
        stop_start = None
        stop_points = []

        for row in stream_rows:
            speed = row["speed_ms"] or 0
            if speed < SPEED_THRESHOLD:
                if stop_start is None:
                    stop_start = row
                stop_points.append(row)
            else:
                if stop_start is not None and stop_points:
                    duration = (
                        stop_points[-1]["time"] - stop_start["time"]
                    ).total_seconds()
                    if duration >= MIN_DURATION_S:
                        mid = len(stop_points) // 2
                        stops.append(
                            {
                                "started_at": stop_start["time"],
                                "ended_at": stop_points[-1]["time"],
                                "duration_s": int(duration),
                                "lat": stop_points[mid]["lat"],
                                "lng": stop_points[mid]["lng"],
                            }
                        )
                stop_start = None
                stop_points = []

        # Handle stop at end of activity
        if stop_start is not None and stop_points:
            duration = (
                stop_points[-1]["time"] - stop_start["time"]
            ).total_seconds()
            if duration >= MIN_DURATION_S:
                mid = len(stop_points) // 2
                stops.append(
                    {
                        "started_at": stop_start["time"],
                        "ended_at": stop_points[-1]["time"],
                        "duration_s": int(duration),
                        "lat": stop_points[mid]["lat"],
                        "lng": stop_points[mid]["lng"],
                    }
                )

        if not stops:
            return 0

        # Delete existing stops and gastronomy geo tags for re-enrichment
        await conn.execute(
            "DELETE FROM activity_stops WHERE activity_id = $1",
            activity_id,
        )
        await conn.execute(
            "DELETE FROM activity_geo_tags WHERE activity_id = $1 AND category = 'gastronomy'",
            activity_id,
        )

        # Batch POI lookup: one query for all stop points via LATERAL
        lats = [s["lat"] for s in stops]
        lngs = [s["lng"] for s in stops]

        poi_rows = await conn.fetch(
            """
            WITH stops AS (
                SELECT ordinality AS idx,
                       ST_SetSRID(ST_MakePoint(lng, lat), 4326) AS geom
                FROM unnest($1::float8[], $2::float8[])
                WITH ORDINALITY AS t(lat, lng)
            )
            SELECT s.idx, a.osm_id, a.name, a.tag_key, a.tag_value
            FROM stops s
            LEFT JOIN LATERAL (
                SELECT a.osm_id, a.name, a.type AS tag_key, a.subtype AS tag_value
                FROM osm_amenities a
                WHERE ST_DWithin(s.geom::geography, a.geom::geography, $3)
                ORDER BY ST_Distance(s.geom::geography, a.geom::geography)
                LIMIT 1
            ) a ON true
            ORDER BY s.idx
            """,
            lats,
            lngs,
            float(POI_RADIUS_M),
        )

        # Map results back to stops by index (ordinality is 1-based)
        poi_by_idx = {row["idx"]: row for row in poi_rows}

        # Insert stops and gastronomy geo tags
        stop_records = []
        geo_tag_records = []

        for i, stop in enumerate(stops):
            poi = poi_by_idx.get(i + 1)
            poi_type = poi["tag_value"] if poi and poi["tag_value"] else None
            poi_name = poi["name"] if poi and poi["name"] else None
            poi_osm_id = poi["osm_id"] if poi and poi["osm_id"] else None

            stop_records.append((
                activity_id,
                stop["started_at"],
                stop["ended_at"],
                stop["duration_s"],
                stop["lat"],
                stop["lng"],
                poi_type,
                poi_name,
                poi_osm_id,
            ))

            if (
                stop["duration_s"] >= GASTRONOMY_MIN_DURATION_S
                and poi_type in GASTRONOMY_TYPES
                and poi_name
            ):
                geo_tag_records.append((
                    activity_id,
                    poi_name,
                    poi_osm_id,
                    stop["lat"],
                    stop["lng"],
                    stop["duration_s"] // 60,
                    {"poi_type": poi_type},
                ))

        await conn.executemany(
            """
            INSERT INTO activity_stops
                (activity_id, started_at, ended_at, duration_s,
                 location, poi_type, poi_name, poi_osm_id)
            VALUES ($1, $2, $3, $4,
                    ST_SetSRID(ST_MakePoint($6, $5), 4326),
                    $7, $8, $9)
            """,
            stop_records,
        )

        if geo_tag_records:
            await conn.executemany(
                """
                INSERT INTO activity_geo_tags
                    (activity_id, category, name, osm_id, map_type,
                     map_point_lat, map_point_lon, duration_min, meta)
                VALUES ($1, 'gastronomy', $2, $3, 'point', $4, $5, $6, $7)
                ON CONFLICT (activity_id, osm_id) DO NOTHING
                """,
                geo_tag_records,
            )

        logger.info(
            "Enriched %d stops (%d gastronomy) for activity %s",
            len(stops), len(geo_tag_records), activity_id,
        )
        return len(stops)
