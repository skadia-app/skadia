"""Lift detection for activities.

Method A: OSM-based matching via PostGIS (primary).
Method B: Heuristic detection (fallback, saves to activity_lift_candidates).
"""
import asyncpg
import logging
import math
from typing import Optional

logger = logging.getLogger(__name__)

PROXIMITY_M = 100
HAUSDORFF_MAX_M = 50
MIN_COVERAGE = 0.7

HEURISTIC_MIN_SPEED_MS = 2.0
HEURISTIC_MAX_SPEED_MS = 5.0
HEURISTIC_MAX_CV = 0.05
HEURISTIC_MIN_GRADIENT = 0.20
HEURISTIC_MAX_SINUOSITY = 1.15
HEURISTIC_MIN_DURATION_S = 60
HEURISTIC_NO_SPEED_MIN_RATE = 0.6
HEURISTIC_NO_SPEED_MIN_DURATION = 90

LIFT_RELEVANT_SPORTS = {
    'alpine_skiing', 'cycling', 'mountain_biking', 'hiking', 'mountaineering',
    'cross_country_skiing', 'snowboarding',
}


async def enrich_lifts(pool: asyncpg.Pool, activity_id: str, user_id: str) -> int:
    """Detect lift rides in activity. Returns number of detected rides."""
    async with pool.acquire() as conn:
        sport_type = await conn.fetchval(
            "SELECT sport_type FROM activities WHERE id = $1", activity_id
        )
        if sport_type and sport_type not in LIFT_RELEVANT_SPORTS:
            return 0

        has_osm = await conn.fetchval(
            "SELECT EXISTS (SELECT 1 FROM osm_lifts LIMIT 1)"
        )
        if not has_osm:
            return 0

        track = await conn.fetchval(
            "SELECT track_full FROM activity_tracks WHERE activity_id = $1", activity_id
        )
        if track is None:
            return 0

        # Bestehende Lift-Daten löschen, damit ein Re-Run idempotent ist.
        await conn.execute(
            "DELETE FROM activity_lift_candidates WHERE activity_id = $1",
            activity_id,
        )

        total = 0

        # Methode A: OSM-Matching
        candidates = await conn.fetch(
            """
            SELECT
                l.way_id AS id, l.osm_id, l.name,
                ST_Length(l.geom::geography) AS lift_length_m,
                ST_HausdorffDistance(
                    ST_Intersection(
                        ST_Buffer(route.geom::geography, $2)::geometry,
                        l.geom
                    ),
                    l.geom, 0.1
                ) AS hausdorff_m,
                ST_Length(
                    ST_Intersection(
                        ST_Buffer(route.geom::geography, $2)::geometry,
                        l.geom
                    )::geography
                ) / NULLIF(ST_Length(l.geom::geography), 0) AS coverage_ratio,
                degrees(ST_Azimuth(ST_StartPoint(l.geom), ST_EndPoint(l.geom))) AS lift_bearing
            FROM osm_lifts l,
                 (SELECT track_full AS geom FROM activity_tracks WHERE activity_id = $1) route
            WHERE ST_DWithin(route.geom::geography, l.geom::geography, $3)
            ORDER BY hausdorff_m
            """,
            activity_id, 30.0, float(PROXIMITY_M),
        )

        for c in candidates:
            if c["hausdorff_m"] is None or c["hausdorff_m"] > HAUSDORFF_MAX_M:
                continue
            if c["coverage_ratio"] is None or c["coverage_ratio"] < MIN_COVERAGE:
                continue

            segment = await conn.fetchrow(
                """
                SELECT s.id, sv.id AS version_id
                FROM segments s
                JOIN segment_versions sv ON sv.segment_id = s.id
                WHERE s.osm_lift_id = $1
                ORDER BY sv.version_number DESC LIMIT 1
                """,
                c["osm_id"],
            )
            if segment is None:
                continue

            ride = await _extract_ride_timewindow(conn, activity_id, c)
            if ride is None:
                continue

            is_reverse = _check_is_reverse(ride["bearing_deg"], c["lift_bearing"])

            await conn.execute(
                """
                INSERT INTO segment_efforts
                    (activity_id, segment_id, segment_version_id,
                     started_at, elapsed_time_s, distance_m, elevation_gain_m,
                     is_reverse, sport_type)
                SELECT $1, $2, $3, $4, $5,
                       ST_Length(sv.track_geojson::geometry::geography),
                       $6, $7, a.sport_type
                FROM segment_versions sv, activities a
                WHERE sv.id = $3 AND a.id = $1
                ON CONFLICT DO NOTHING
                """,
                activity_id, segment["id"], segment["version_id"],
                ride["started_at"], ride["duration_s"], ride["ascent_m"], is_reverse,
            )
            total += 1

        # Methode B: Heuristik-Fallback
        heuristic_rides = await _detect_heuristic_rides(conn, activity_id)
        for ride in heuristic_rides:
            await conn.execute(
                """
                INSERT INTO activity_lift_candidates
                    (activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
                VALUES ($1, $2, $3, $4, $5, $6, $7, $8, false)
                ON CONFLICT (activity_id, started_at) DO NOTHING
                """,
                activity_id, ride["started_at"], ride["ended_at"],
                ride["duration_s"], ride["ascent_m"], ride["descent_m"],
                ride["distance_m"], ride["confidence"],
            )

        await _update_lift_stats(conn, activity_id)
        logger.info("Detected %d lift rides (OSM) for activity %s", total, activity_id)
        return total


async def _extract_ride_timewindow(conn, activity_id: str, candidate: dict) -> Optional[dict]:
    rows = await conn.fetch(
        """
        SELECT time, altitude_m, speed_ms, lat, lng
        FROM activity_streams
        WHERE activity_id = $1
          AND lat IS NOT NULL
          AND ST_DWithin(
            ST_SetSRID(ST_MakePoint(lng, lat), 4326)::geography,
            (SELECT geom::geography FROM osm_lifts WHERE osm_id = $2),
            75
          )
        ORDER BY time
        """,
        activity_id, candidate["osm_id"],
    )
    if not rows or len(rows) < 2:
        return None

    started_at = rows[0]["time"]
    ended_at = rows[-1]["time"]
    import datetime
    duration_s = int((ended_at - started_at).total_seconds())
    if duration_s < 30:
        return None

    ascent_m = max(0.0, (rows[-1]["altitude_m"] or 0) - (rows[0]["altitude_m"] or 0))
    bearing = 0.0
    if rows[0]["lat"] and rows[-1]["lat"]:
        bearing = _bearing(rows[0]["lat"], rows[0]["lng"], rows[-1]["lat"], rows[-1]["lng"])

    return {"started_at": started_at, "duration_s": duration_s, "ascent_m": ascent_m, "bearing_deg": bearing}


def _bearing(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    lat1, lon1, lat2, lon2 = map(math.radians, [lat1, lon1, lat2, lon2])
    dlon = lon2 - lon1
    x = math.sin(dlon) * math.cos(lat2)
    y = math.cos(lat1) * math.sin(lat2) - math.sin(lat1) * math.cos(lat2) * math.cos(dlon)
    return (math.degrees(math.atan2(x, y)) + 360) % 360


def _check_is_reverse(track_bearing: float, lift_bearing: float) -> bool:
    diff = abs(track_bearing - lift_bearing) % 360
    if diff > 180:
        diff = 360 - diff
    return diff > 90


async def _detect_heuristic_rides(conn, activity_id: str) -> list:
    rows = await conn.fetch(
        """
        SELECT time, lat, lng, altitude_m, speed_ms
        FROM activity_streams
        WHERE activity_id = $1 AND lat IS NOT NULL
        ORDER BY time
        """,
        activity_id,
    )
    if len(rows) < 30:
        return []

    rides = []
    i = 0
    while i < len(rows):
        window = []
        j = i
        while j < len(rows):
            dt = (rows[j]["time"] - rows[i]["time"]).total_seconds()
            window.append(rows[j])
            if dt >= 30:
                break
            j += 1

        if len(window) < 10:
            i += 1
            continue

        speeds = [r["speed_ms"] for r in window if r["speed_ms"] is not None]
        alts = [r["altitude_m"] for r in window if r["altitude_m"] is not None]

        if len(alts) < 2:
            i += 1
            continue

        dt_window = max((window[-1]["time"] - window[0]["time"]).total_seconds(), 1)
        alt_rate = (alts[-1] - alts[0]) / dt_window
        # Lifte fahren aufwärts (alt_rate > 0) oder abwärts als Talfahrt (alt_rate < 0).
        # Reine Auf-/Abfahrten beim Gehen/Fahren werden durch Sinuosität und Gradient herausgefiltert.
        if abs(alt_rate) < 0.05:
            i += 1
            continue

        cum_dist = sum(
            _haversine(window[k]["lat"], window[k]["lng"], window[k+1]["lat"], window[k+1]["lng"])
            for k in range(len(window) - 1)
        )
        straight = _haversine(window[0]["lat"], window[0]["lng"], window[-1]["lat"], window[-1]["lng"])
        sinuosity = cum_dist / max(straight, 1)

        if sinuosity > HEURISTIC_MAX_SINUOSITY:
            i += 1
            continue

        is_candidate = False
        confidence = "low"
        abs_alt_rate = abs(alt_rate)

        if speeds:
            mean_spd = sum(speeds) / len(speeds)
            std_spd = math.sqrt(sum((s - mean_spd) ** 2 for s in speeds) / len(speeds))
            cv = std_spd / max(mean_spd, 0.001)
            gradient = abs_alt_rate / max(mean_spd, 0.001)
            if HEURISTIC_MIN_SPEED_MS <= mean_spd <= HEURISTIC_MAX_SPEED_MS and cv < HEURISTIC_MAX_CV and gradient > HEURISTIC_MIN_GRADIENT:
                is_candidate = True
                confidence = "high"
        else:
            if abs_alt_rate > HEURISTIC_NO_SPEED_MIN_RATE:
                is_candidate = True
                confidence = "medium"

        if is_candidate:
            end_idx = j
            going_up = alt_rate > 0
            while end_idx + 1 < len(rows):
                nxt = rows[end_idx + 1]
                if nxt["altitude_m"] and alts:
                    alt_delta = nxt["altitude_m"] - alts[-1]
                    # Erlaubt kleine Abweichungen (±2 m) in der Hauptrichtung
                    continues_direction = alt_delta > -2 if going_up else alt_delta < 2
                    if continues_direction:
                        alts.append(nxt["altitude_m"])
                        if nxt["speed_ms"]:
                            speeds.append(nxt["speed_ms"])
                        end_idx += 1
                    else:
                        break
                else:
                    break

            # Gesamtdistanz der erweiterten Ride-Strecke berechnen
            end_time = rows[end_idx]["time"]
            ride_rows = [r for r in rows[i:end_idx+1] if r["lat"] is not None]
            total_dist = sum(
                _haversine(ride_rows[k]["lat"], ride_rows[k]["lng"],
                           ride_rows[k+1]["lat"], ride_rows[k+1]["lng"])
                for k in range(len(ride_rows) - 1)
            )

            duration = (end_time - window[0]["time"]).total_seconds()
            min_dur = HEURISTIC_MIN_DURATION_S if confidence == "high" else HEURISTIC_NO_SPEED_MIN_DURATION
            if duration >= min_dur:
                alt_change = alts[-1] - alts[0]
                rides.append({
                    "started_at": window[0]["time"],
                    "ended_at": end_time,
                    "duration_s": int(duration),
                    "ascent_m":  max(0.0,  alt_change),
                    "descent_m": max(0.0, -alt_change),
                    "distance_m": total_dist,
                    "confidence": confidence,
                })
            i = end_idx + 1
        else:
            i += 1

    return rides


def _haversine(lat1, lon1, lat2, lon2) -> float:
    R = 6371000
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    dphi = math.radians(lat2 - lat1)
    dlam = math.radians(lon2 - lon1)
    a = math.sin(dphi/2)**2 + math.cos(phi1)*math.cos(phi2)*math.sin(dlam/2)**2
    return 2 * R * math.atan2(math.sqrt(a), math.sqrt(1 - a))


async def _update_lift_stats(conn, activity_id: str) -> None:
    """Recompute lift_* columns from both OSM segment_efforts and confirmed heuristic candidates."""
    await conn.execute(
        """
        UPDATE activities SET
            lift_count = (
                SELECT COUNT(*) FROM segment_efforts se
                JOIN segments s ON s.id = se.segment_id
                WHERE se.activity_id = $1 AND s.segment_type = 'lift'
            ) + (
                SELECT COUNT(*) FROM activity_lift_candidates
                WHERE activity_id = $1 AND confirmed = true
            ),
            lift_duration_s = (
                SELECT COALESCE(SUM(se.elapsed_time_s), 0) FROM segment_efforts se
                JOIN segments s ON s.id = se.segment_id
                WHERE se.activity_id = $1 AND s.segment_type = 'lift'
            ) + (
                SELECT COALESCE(SUM(duration_s), 0) FROM activity_lift_candidates
                WHERE activity_id = $1 AND confirmed = true
            ),
            lift_ascent_m = (
                SELECT COALESCE(SUM(CASE WHEN NOT se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
                FROM segment_efforts se
                JOIN segments s ON s.id = se.segment_id
                WHERE se.activity_id = $1 AND s.segment_type = 'lift'
            ) + (
                SELECT COALESCE(SUM(ascent_m), 0) FROM activity_lift_candidates
                WHERE activity_id = $1 AND confirmed = true
            ),
            lift_descent_m = (
                SELECT COALESCE(SUM(CASE WHEN se.is_reverse THEN se.elevation_gain_m ELSE 0 END), 0)
                FROM segment_efforts se
                JOIN segments s ON s.id = se.segment_id
                WHERE se.activity_id = $1 AND s.segment_type = 'lift'
            ) + (
                SELECT COALESCE(SUM(descent_m), 0) FROM activity_lift_candidates
                WHERE activity_id = $1 AND confirmed = true
            ),
            lift_distance_m = (
                SELECT COALESCE(SUM(se.distance_m), 0) FROM segment_efforts se
                JOIN segments s ON s.id = se.segment_id
                WHERE se.activity_id = $1 AND s.segment_type = 'lift'
            ) + (
                SELECT COALESCE(SUM(distance_m), 0) FROM activity_lift_candidates
                WHERE activity_id = $1 AND confirmed = true
            )
        WHERE id = $1
        """,
        activity_id,
    )
