"""SQL Agent — generates and executes read-only SQL queries.

Uses skadia_readonly PostgreSQL role with explicit table whitelist + RLS.
"""
import logging
import re
from uuid import UUID

import sqlglot
import sqlglot.expressions as exp

logger = logging.getLogger(__name__)

# Blocklist patterns (case-insensitive)
_BLOCKLIST = re.compile(
    r"\b(pg_catalog|pg_stat|pg_read_file|lo_import|information_schema|"
    r"COPY|DO\s+\$|pg_sleep|set\s+role|set\s+session)\b",
    re.IGNORECASE,
)

TABLE_WHITELIST = {
    "activities", "activity_streams", "activity_laps", "activity_splits",
    "activity_geo_tags", "activity_stops", "activity_rr_intervals", "activity_photos",
    "health_snapshots", "health_sleep", "health_weight_entries",
    "gear", "gear_components", "segments", "segment_efforts",
    "pmc_daily", "user_daily_features", "tags", "activity_tags",
    "osm_pois", "osm_ways", "osm_pistes", "osm_glaciers", "osm_wetlands", "osm_routes",
    # NEU hinzugefügt:
    "osm_lifts", "osm_waterways", "osm_waters", "osm_forests", "osm_boundaries",
    "osm_import_regions",
}

SCHEMA_CONTEXT = """Available tables (PostgreSQL, READ-ONLY):

activities: id UUID, user_id UUID, title TEXT, sport_type TEXT, sub_type TEXT, started_at TIMESTAMPTZ, elapsed_time_s INT, moving_time_s INT, distance_m FLOAT8, elevation_gain_m FLOAT8, avg_speed_ms FLOAT8, max_speed_ms FLOAT8, avg_heart_rate FLOAT8, max_heart_rate INT, avg_power_w FLOAT8, max_power_w INT, avg_cadence FLOAT8, avg_temperature_c FLOAT8, tss FLOAT8, trimp FLOAT8, is_indoor BOOL, is_commute BOOL, start_city TEXT, weather_temp_c FLOAT8, weather_condition TEXT, gear_id UUID
activity_streams: activity_id UUID, time TIMESTAMPTZ, lat FLOAT8, lng FLOAT8, altitude_m FLOAT8, speed_ms FLOAT8, distance_m FLOAT8, heart_rate SMALLINT, power_w SMALLINT, cadence SMALLINT, temperature_c SMALLINT
activity_laps: activity_id UUID, message_index INT, start_time TIMESTAMPTZ, total_elapsed_time_s FLOAT8, total_distance_m FLOAT8, avg_speed_ms FLOAT8, avg_heart_rate INT, max_heart_rate INT, avg_cadence INT, max_cadence INT, avg_power_w INT, max_power_w INT, total_ascent_m FLOAT8, total_descent_m FLOAT8, total_calories INT
health_snapshots: user_id UUID, measured_at_utc TIMESTAMPTZ, rmssd_ms FLOAT8, sdrr_ms FLOAT8, time_of_day TEXT, source_type TEXT, extra_data JSONB
health_sleep: user_id UUID, calendar_date DATE, sleep_start_utc TIMESTAMPTZ, sleep_end_utc TIMESTAMPTZ, deep_seconds INT, light_seconds INT, rem_seconds INT, awake_seconds INT, score_overall INT, score_quality INT
health_weight_entries: user_id UUID, measured_at_utc TIMESTAMPTZ, weight_g INT, bmi FLOAT8, body_fat_pct FLOAT8
segments: id UUID, created_by UUID, name TEXT, sport_group TEXT, distance_m FLOAT8, elevation_gain_m FLOAT8, effort_count INT
segment_efforts: segment_id UUID, activity_id UUID, user_id UUID, started_at TIMESTAMPTZ, elapsed_time_s INT, sport_type TEXT, avg_power_w FLOAT8, avg_heart_rate FLOAT8, avg_speed_ms FLOAT8, is_personal_best BOOL, is_reverse BOOL
gear: id UUID, user_id UUID, name TEXT, type TEXT, brand TEXT, model TEXT, total_distance_m FLOAT8, status TEXT
pmc_daily: user_id UUID, date DATE, tss FLOAT8, ctl FLOAT8, atl FLOAT8, tsb FLOAT8
user_daily_features: user_id UUID, date DATE, activity_id UUID, feature_key TEXT, value_num FLOAT8, value_json JSONB
tags: id UUID, user_id UUID, name TEXT, color TEXT
activity_tags: activity_id UUID, tag_id UUID
activity_geo_tags: activity_id UUID, category TEXT, name TEXT, osm_id BIGINT, ele FLOAT8, lat FLOAT8, lng FLOAT8, dist_m FLOAT8
activity_stops: activity_id UUID, started_at TIMESTAMPTZ, ended_at TIMESTAMPTZ, duration_s INT, location GEOMETRY(Point,4326), name TEXT, poi_type TEXT, poi_name TEXT, poi_osm_id BIGINT
activity_photos: id UUID, user_id UUID, activity_id UUID, taken_at TIMESTAMPTZ, lat FLOAT8, lng FLOAT8, is_video BOOL, duration_s FLOAT8
osm_pois: id BIGINT, osm_id BIGINT, name TEXT, category TEXT, type TEXT, subtype TEXT, ele REAL, geom GEOMETRY(Geometry,4326) — Points (nodes), Polygons (closed ways), MultiPolygons (relations); categories: summit, pass, settlement, gastronomy, accommodation, shelter, water_supply, transport, parking, viewpoint, shop, barrier
osm_ways: osm_id BIGINT, name TEXT, highway TEXT, surface TEXT, geom GEOMETRY(LineString,4326) — all routable ways (roads, paths, trails)
osm_routes: id BIGINT, osm_id BIGINT, name TEXT, ref TEXT, route TEXT, network TEXT, colour TEXT, distance REAL, geom GEOMETRY(MultiLineString,4326) — named route relations (hiking/bicycle/mtb/foot/ski/running/ferry/via_ferrata)

osm_lifts: id BIGINT, osm_id BIGINT, name TEXT, type TEXT, geom GEOMETRY(LineString,4326)
osm_waterways: id BIGINT, osm_id BIGINT, name TEXT, waterway TEXT, geom GEOMETRY(LineString,4326)
osm_waters: id BIGINT, osm_id BIGINT, name TEXT, natural TEXT, geom GEOMETRY(Polygon,4326)
osm_forests: id BIGINT, osm_id BIGINT, name TEXT, leaf_type TEXT, geom GEOMETRY(Polygon,4326)
osm_boundaries: id BIGINT, osm_id BIGINT, name TEXT, admin_level INT, boundary_type TEXT, protect_class TEXT, geom GEOMETRY(Polygon,4326)
osm_import_regions: region_id TEXT (PK), label TEXT, continent TEXT, imported_at TIMESTAMPTZ, pbf_size_bytes BIGINT, categories TEXT[], status TEXT

IMPORTANT: Do NOT add user_id filters — Row-Level Security (RLS) handles this automatically. Do NOT use $1/$2 parameter placeholders — write values directly into the query.
"""


def validate_sql(sql: str) -> tuple[bool, str]:
    """Validate SQL query for safety. Returns (is_valid, error_message).

    Uses sqlglot AST parsing to catch DML/DDL/system-table access at the
    syntax-tree level, which is more robust than regex-only approaches.
    """
    # Quick blocklist check before parsing
    if _BLOCKLIST.search(sql):
        return False, "Zugriff auf Systemtabellen/-funktionen nicht erlaubt (nur SELECT erlaubt)"

    # Reject parameter placeholders ($1, $2, ...) — RLS handles user_id
    if re.search(r"\$\d+", sql):
        return False, (
            "Parameter-Platzhalter ($1, $2, ...) nicht erlaubt. "
            "Schreibe Werte direkt in die Abfrage. "
            "user_id-Filter ist NICHT nötig (wird automatisch per RLS angewandt)."
        )

    # Reject user_id filters — RLS handles this automatically
    if re.search(r"\buser_id\s*=", sql, re.IGNORECASE):
        return False, (
            "user_id-Filter NICHT verwenden! Row-Level Security (RLS) filtert "
            "automatisch nach dem aktuellen Benutzer. Entferne die WHERE user_id=... Klausel."
        )

    # Reject current_user/session_user which cause type mismatches with UUID columns
    if re.search(r"\b(current_user|session_user|current_role)\b", sql, re.IGNORECASE):
        return False, (
            "current_user/session_user nicht verwenden — RLS filtert automatisch. "
            "Entferne den user_id-Filter komplett."
        )

    # Multi-statement check — strip trailing semicolons then look for interior ones
    stripped = sql.strip().rstrip(";")
    if ";" in stripped:
        return False, "Nur einzelne Abfragen erlaubt (kein Semikolon)"

    try:
        statements = sqlglot.parse(sql, dialect="postgres")
    except sqlglot.errors.ParseError as e:
        return False, f"SQL-Syntax-Fehler: {e}"

    if len(statements) != 1:
        return False, "Nur einzelne Abfragen erlaubt"

    stmt = statements[0]
    # Allow SELECT and UNION/INTERSECT/EXCEPT (all are Query subclasses)
    if not isinstance(stmt, (exp.Select, exp.Union, exp.Intersect, exp.Except)):
        return False, "Nur SELECT-Abfragen erlaubt (inkl. UNION/INTERSECT/EXCEPT)"

    # Check for DML/DDL nodes embedded anywhere in the AST
    for node in stmt.walk():
        if isinstance(node, (exp.Insert, exp.Update, exp.Delete, exp.Drop,
                             exp.Alter, exp.Create, exp.Command)):
            return False, f"Verbotene Operation: {type(node).__name__}"

    return True, ""


def _autofix_sql(sql: str) -> str:
    """Auto-fix common LLM SQL mistakes.

    PostgreSQL's ROUND() only accepts numeric, not float8.
    LLMs consistently generate ROUND(float_expr, n) which fails.
    Fix: wrap the first argument in ::numeric cast.
    """
    # Fix ROUND(expr, n) → ROUND((expr)::numeric, n)
    # Match ROUND( ... , digit) patterns — but only when first arg is not already ::numeric
    import re

    def _fix_round(m):
        inner = m.group(1).strip()
        decimals = m.group(2).strip()
        if "::numeric" in inner:
            return m.group(0)  # already fixed
        return f"ROUND(({inner})::numeric, {decimals})"

    # Iteratively fix nested ROUND calls (inner first)
    prev = None
    fixed = sql
    while fixed != prev:
        prev = fixed
        fixed = re.sub(
            r"ROUND\(([^()]+(?:\([^()]*\))?[^()]*),\s*(\d+)\)",
            _fix_round,
            fixed,
            flags=re.IGNORECASE,
        )

    # Fix common column name hallucinations
    fixed = re.sub(r"\bduration_s\b", "elapsed_time_s", fixed)
    # Only fix standalone "elevation" (not "elevation_gain_m" etc.) in geo_tags context
    fixed = re.sub(r"(?:agt?|gt)\.\belevation\b", lambda m: m.group(0).replace("elevation", "ele"), fixed)

    return fixed


async def execute_sql(
    sql: str,
    user_id: UUID,
    max_rows: int = 100,
) -> list[dict]:
    """Execute a validated SQL query using the readonly database pool.

    Uses ``get_readonly_pool()`` (skadia_readonly role) so that even if the
    LLM generates a mutating query that slips past ``validate_sql``, the DB
    role will reject it.  The ``SET LOCAL app.current_user_id`` call enables
    row-level security policies on the readonly connection.
    """
    # Auto-fix common LLM SQL mistakes before validation
    sql = _autofix_sql(sql)

    valid, error = validate_sql(sql)
    if not valid:
        raise ValueError(f"SQL validation failed: {error}")

    from db import get_readonly_pool
    pool = await get_readonly_pool()
    async with pool.acquire() as conn:
        async with conn.transaction():
            await conn.execute("SELECT set_config('app.current_user_id', $1, true)", str(user_id))
            rows = await conn.fetch(sql, timeout=5.0)
            return [dict(r) for r in rows[:max_rows]]
