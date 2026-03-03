import asyncpg
import logging
from datetime import timedelta

logger = logging.getLogger(__name__)

# Speed threshold in m/s below which a point is considered stationary
SPEED_THRESHOLD = 0.5
# Minimum stop duration in seconds
MIN_DURATION_S = 60
# Radius in meters for POI matching around a stop point
POI_RADIUS_M = 50


async def enrich_stops(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Detect stops in the activity and match them to nearby POIs.

    A stop is defined as consecutive points where speed < 0.5 m/s for > 60 seconds.
    Each detected stop is matched against osm_amenities within 50m.
    Writes results to activity_stops table.

    Returns the number of stops found.
    """
    async with pool.acquire() as conn:
        # Fetch stream points with speed, time, and location
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
                        # Use the median point as the stop location
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

        # Delete existing stops for re-enrichment
        await conn.execute(
            "DELETE FROM activity_stops WHERE activity_id = $1",
            activity_id,
        )

        for stop in stops:
            # Try to match a POI
            poi = await conn.fetchrow(
                """
                WITH stop_pt AS (
                    SELECT ST_SetSRID(ST_MakePoint($2, $1), 4326) AS geom
                )
                SELECT a.osm_id, a.name, a.amenity, a.tourism,
                       ROUND(ST_Distance(stop_pt.geom::geography, a.geom::geography))::int AS dist_m
                FROM osm_amenities a, stop_pt
                WHERE ST_DWithin(stop_pt.geom::geography, a.geom::geography, $3)
                ORDER BY dist_m LIMIT 1
                """,
                stop["lat"],
                stop["lng"],
                POI_RADIUS_M,
            )

            poi_type = poi["amenity"] or poi["tourism"] if poi else None
            poi_name = poi["name"] if poi else None
            poi_osm_id = poi["osm_id"] if poi else None

            await conn.execute(
                """
                INSERT INTO activity_stops
                    (activity_id, started_at, ended_at, duration_s,
                     location, poi_type, poi_name, poi_osm_id)
                VALUES ($1, $2, $3, $4,
                        ST_SetSRID(ST_MakePoint($6, $5), 4326),
                        $7, $8, $9)
                """,
                activity_id,
                stop["started_at"],
                stop["ended_at"],
                stop["duration_s"],
                stop["lat"],
                stop["lng"],
                poi_type,
                poi_name,
                poi_osm_id,
            )

        # Also write significant stops (>15 min) as geo tags
        for stop in stops:
            if stop["duration_s"] >= 900 and poi_name:
                await conn.execute(
                    """
                    INSERT INTO activity_geo_tags
                        (activity_id, category, name, map_type,
                         map_point_lat, map_point_lon, duration_min, meta)
                    VALUES ($1, 'stop', $2, 'point', $3, $4, $5, $6)
                    """,
                    activity_id,
                    poi_name or f"Stopp ({stop['duration_s'] // 60} min)",
                    stop["lat"],
                    stop["lng"],
                    stop["duration_s"] // 60,
                    {"poi_type": poi_type} if poi_type else {},
                )

        logger.info(
            "Enriched %d stops for activity %s", len(stops), activity_id
        )
        return len(stops)
