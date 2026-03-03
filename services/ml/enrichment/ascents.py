import asyncpg
import logging
import numpy as np
from scipy.signal import savgol_filter

logger = logging.getLogger(__name__)

# Default thresholds (can be overridden by user_enrichment_settings)
MIN_GRADIENT_PCT = 3.0
MIN_ELEVATION_M = 20.0
MIN_LENGTH_M = 100.0
SMOOTHING_WINDOW = 51


async def enrich_ascents(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Detect significant ascents using Savitzky-Golay filter on the elevation profile.

    Smooths the altitude data, identifies ascending segments above configurable
    thresholds, and writes them to activity_climbs.

    Returns the number of ascents found.
    """
    async with pool.acquire() as conn:
        # Load user-specific thresholds if available
        settings = await conn.fetchrow(
            """
            SELECT climb_min_gradient_pct, climb_min_elevation_m, climb_min_length_m
            FROM user_enrichment_settings
            WHERE user_id = $1
            """,
            user_id,
        )
        min_gradient = settings["climb_min_gradient_pct"] if settings else MIN_GRADIENT_PCT
        min_elevation = settings["climb_min_elevation_m"] if settings else MIN_ELEVATION_M
        min_length = settings["climb_min_length_m"] if settings else MIN_LENGTH_M

        # Fetch elevation and distance streams
        stream_rows = await conn.fetch(
            """
            SELECT altitude_m, distance_m
            FROM activity_streams
            WHERE activity_id = $1
              AND altitude_m IS NOT NULL
              AND distance_m IS NOT NULL
            ORDER BY time
            """,
            activity_id,
        )

        if len(stream_rows) < SMOOTHING_WINDOW:
            return 0

        altitudes = np.array([r["altitude_m"] for r in stream_rows], dtype=float)
        distances = np.array([r["distance_m"] for r in stream_rows], dtype=float)

        # Apply Savitzky-Golay filter to smooth altitude
        window = min(SMOOTHING_WINDOW, len(altitudes))
        if window % 2 == 0:
            window -= 1
        if window < 5:
            return 0

        smoothed = savgol_filter(altitudes, window_length=window, polyorder=3)

        # Find ascending segments
        ascents = []
        in_ascent = False
        start_idx = 0

        for i in range(1, len(smoothed)):
            climbing = smoothed[i] > smoothed[i - 1]
            if climbing and not in_ascent:
                start_idx = i - 1
                in_ascent = True
            elif not climbing and in_ascent:
                _maybe_add_ascent(
                    ascents, smoothed, distances, start_idx, i,
                    min_gradient, min_elevation, min_length,
                )
                in_ascent = False

        # Handle ascent at end
        if in_ascent:
            _maybe_add_ascent(
                ascents, smoothed, distances, start_idx, len(smoothed) - 1,
                min_gradient, min_elevation, min_length,
            )

        if not ascents:
            return 0

        # Delete existing climbs for re-enrichment
        await conn.execute(
            "DELETE FROM activity_climbs WHERE activity_id = $1",
            activity_id,
        )

        for idx, ascent in enumerate(ascents):
            await conn.execute(
                """
                INSERT INTO activity_climbs
                    (activity_id, climb_number, start_distance_m, end_distance_m,
                     length_m, elevation_gain_m, avg_gradient_pct, max_gradient_pct)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
                """,
                activity_id,
                idx + 1,
                ascent["start_distance_m"],
                ascent["end_distance_m"],
                ascent["length_m"],
                ascent["elevation_gain_m"],
                ascent["avg_gradient_pct"],
                ascent["max_gradient_pct"],
            )

        logger.info(
            "Enriched %d ascents for activity %s", len(ascents), activity_id
        )
        return len(ascents)


def _maybe_add_ascent(
    ascents: list,
    smoothed: np.ndarray,
    distances: np.ndarray,
    start_idx: int,
    end_idx: int,
    min_gradient: float,
    min_elevation: float,
    min_length: float,
) -> None:
    """Add an ascent to the list if it meets the threshold criteria."""
    elevation_gain = float(smoothed[end_idx] - smoothed[start_idx])
    length_m = float(distances[end_idx] - distances[start_idx])

    if elevation_gain < min_elevation or length_m < min_length:
        return

    avg_gradient = (elevation_gain / length_m * 100) if length_m > 0 else 0
    if avg_gradient < min_gradient:
        return

    # Calculate max gradient over 100m segments
    max_gradient = 0.0
    for i in range(start_idx, end_idx):
        seg_dist = distances[i + 1] - distances[i]
        if seg_dist > 0:
            seg_grad = (smoothed[i + 1] - smoothed[i]) / seg_dist * 100
            max_gradient = max(max_gradient, seg_grad)

    ascents.append(
        {
            "start_distance_m": float(distances[start_idx]),
            "end_distance_m": float(distances[end_idx]),
            "length_m": length_m,
            "elevation_gain_m": elevation_gain,
            "avg_gradient_pct": round(avg_gradient, 1),
            "max_gradient_pct": round(max_gradient, 1),
        }
    )
