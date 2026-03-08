"""SQL Agent — generates and executes read-only SQL queries."""
import logging
import re
from uuid import UUID

import asyncpg

logger = logging.getLogger(__name__)

TABLE_WHITELIST = {
    "activities", "activity_streams", "activity_laps", "activity_splits",
    "activity_geo_tags", "activity_stops", "activity_rr_intervals", "activity_photos",
    "health_hrv_snapshots", "health_sleep", "health_weight_entries",
    "gear", "gear_components", "segments", "segment_efforts",
    "pmc_daily", "user_daily_features", "tags", "activity_tags",
}

SCHEMA_CONTEXT = """Available tables (PostgreSQL, READ-ONLY):

activities: id UUID, user_id UUID, title TEXT, sport_type TEXT, sub_type TEXT, started_at TIMESTAMPTZ, elapsed_time_s INT, moving_time_s INT, distance_m FLOAT8, elevation_gain_m FLOAT8, avg_speed_ms FLOAT8, max_speed_ms FLOAT8, avg_heart_rate FLOAT8, max_heart_rate INT, avg_power_w FLOAT8, max_power_w INT, avg_cadence FLOAT8, avg_temperature_c FLOAT8, tss FLOAT8, trimp FLOAT8, is_indoor BOOL, is_commute BOOL, start_city TEXT, weather_temp_c FLOAT8, weather_condition TEXT, gear_id UUID
activity_streams: activity_id UUID, time TIMESTAMPTZ, lat FLOAT8, lng FLOAT8, altitude_m FLOAT8, speed_ms FLOAT8, distance_m FLOAT8, heart_rate SMALLINT, power_w SMALLINT, cadence SMALLINT, temperature_c SMALLINT
activity_laps: activity_id UUID, message_index INT, start_time TIMESTAMPTZ, total_elapsed_time_s FLOAT8, total_distance_m FLOAT8, avg_speed_ms FLOAT8, avg_heart_rate INT, max_heart_rate INT, avg_cadence INT, max_cadence INT, avg_power_w INT, max_power_w INT, total_ascent_m FLOAT8, total_descent_m FLOAT8, total_calories INT
health_hrv_snapshots: user_id UUID, measured_at_utc TIMESTAMPTZ, rmssd_ms FLOAT8, sdrr_ms FLOAT8, time_of_day TEXT, source_type TEXT, extra_data JSONB
health_sleep: user_id UUID, calendar_date DATE, sleep_start_utc TIMESTAMPTZ, sleep_end_utc TIMESTAMPTZ, deep_seconds INT, light_seconds INT, rem_seconds INT, awake_seconds INT, score_overall INT, score_quality INT
health_weight_entries: user_id UUID, measured_at_utc TIMESTAMPTZ, weight_g INT, bmi FLOAT8, body_fat_pct FLOAT8
gear: id UUID, user_id UUID, name TEXT, type TEXT, brand TEXT, model TEXT, total_distance_m FLOAT8, status TEXT
pmc_daily: user_id UUID, date DATE, tss FLOAT8, ctl FLOAT8, atl FLOAT8, tsb FLOAT8
user_daily_features: user_id UUID, date DATE, activity_id UUID, feature_key TEXT, value_num FLOAT8, value_json JSONB
tags: id UUID, user_id UUID, name TEXT, color TEXT
activity_tags: activity_id UUID, tag_id UUID

IMPORTANT: Always filter by user_id. Never query across users.
"""


def validate_sql(sql: str) -> tuple[bool, str]:
    """Validate SQL query for safety."""
    sql_upper = sql.upper().strip()

    if not sql_upper.startswith("SELECT"):
        return False, "Only SELECT queries allowed"

    forbidden = ["INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE",
                 "GRANT", "REVOKE", "COPY", "EXECUTE", "CALL"]
    for word in forbidden:
        if re.search(rf'\b{word}\b', sql_upper):
            return False, f"Forbidden keyword: {word}"

    if re.search(r'\bpg_\w+', sql, re.IGNORECASE):
        return False, "System functions not allowed"

    blocked_functions = [
        r'\bcurrent_setting\b', r'\bset_config\b', r'\blo_import\b', r'\blo_export\b',
        r'\bpg_read_file\b', r'\bpg_write_file\b', r'\binformation_schema\b',
    ]
    for pattern in blocked_functions:
        if re.search(pattern, sql, re.IGNORECASE):
            return False, f"Blocked function/schema: {pattern}"

    sql_no_strings = re.sub(r"'[^']*'", "", sql)
    if sql_no_strings.count(";") > 1:
        return False, "Multiple statements not allowed"

    return True, ""


async def execute_sql(
    pool: asyncpg.Pool,
    sql: str,
    user_id: UUID,
    max_rows: int = 100,
) -> list[dict]:
    """Execute a validated SQL query."""
    valid, error = validate_sql(sql)
    if not valid:
        raise ValueError(f"SQL validation failed: {error}")

    async with pool.acquire() as conn:
        async with conn.transaction(readonly=True):
            await conn.execute("SET LOCAL app.current_user_id = $1", str(user_id))
            rows = await conn.fetch(sql, timeout=5.0)
            return [dict(r) for r in rows[:max_rows]]
