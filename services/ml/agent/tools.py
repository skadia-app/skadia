"""Tool definitions for LLM tool-use (function calling).

Contains 11 domain-specific tools with scenario-oriented descriptions,
enum parameters, and compact result formatting.
"""
import json
import logging
from datetime import date, timedelta
from uuid import UUID

logger = logging.getLogger(__name__)

# Canonical sport type enum shared across tools
_SPORT_TYPES = [
    "running", "cycling", "hiking", "walking", "swimming",
    "trail_running", "mountain_biking", "alpine_skiing",
    "cross_country_skiing", "strength_training", "open_water_swimming",
    "mountaineering", "gravel_cycling", "indoor_cycling",
    "rowing", "paddle_boarding", "kayaking", "yoga", "other",
]

ALL_TOOL_DEFINITIONS = [
    # ── 1. query_activities ────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_activities",
            "description": (
                "Fetch individual activity records (title, date, distance, pace/speed, "
                "HR, elevation, city). Use when the user asks about SPECIFIC activities "
                "(e.g. 'mein letzter Lauf', 'schnellste Radtour', 'alle Bergtouren im März'). "
                "Do NOT use for totals/counts — use aggregate_stats instead. "
                "Do NOT use for health/HRV questions — use get_health_status. "
                "sport_type must be an exact DB value from the enum."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "sport_type": {
                        "type": "string",
                        "description": "Filter by sport type (exact DB value)",
                        "enum": _SPORT_TYPES,
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30; use 365 for 'letztes Mal')",
                        "default": 30,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results to return (default 10, max 50)",
                        "default": 10,
                    },
                    "sort_by": {
                        "type": "string",
                        "enum": ["date", "distance", "duration", "hr", "elevation"],
                        "description": "Sort order (default: date)",
                        "default": "date",
                    },
                    "min_distance_km": {
                        "type": "number",
                        "description": "Minimum distance in km",
                    },
                    "title_search": {
                        "type": "string",
                        "description": "Partial match on activity title (case-insensitive)",
                    },
                },
            },
        },
    },

    # ── 2. aggregate_stats ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "aggregate_stats",
            "description": (
                "Compute aggregate statistics over a time window: total distance, "
                "total duration, activity count, average HR, total elevation, "
                "or weekly/monthly breakdowns. "
                "Use for 'Wie weit?', 'Wie oft?', 'Wie viel insgesamt?' questions. "
                "Do NOT use to list individual activities — use query_activities for that. "
                "Example: 'Wie weit bin ich diese Woche gelaufen?' → "
                "aggregate_stats(metric='total_distance', sport_type='running', days=7)"
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "metric": {
                        "type": "string",
                        "enum": [
                            "total_distance", "total_duration", "count",
                            "avg_heart_rate", "total_elevation", "avg_distance",
                        ],
                        "description": "Which aggregate metric to compute",
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to a specific sport (omit for all sports)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30)",
                        "default": 30,
                    },
                    "group_by": {
                        "type": "string",
                        "enum": ["none", "week", "month", "sport"],
                        "description": "Break down results by time period or sport (default: none)",
                        "default": "none",
                    },
                },
            },
        },
    },

    # ── 3. get_health_status ───────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_health_status",
            "description": (
                "Retrieve health and recovery data: HRV trend (RMSSD, SDRR, status), "
                "sleep metrics (duration, deep sleep, score), resting HR, readiness score, "
                "body weight, SpO2. "
                "Use for 'Wie ist mein HRV?', 'Wie habe ich geschlafen?', "
                "'Bin ich erholt?', 'Wie ist mein Ruhepuls?' questions. "
                "Do NOT use for training load/fatigue — use get_training_load. "
                "Do NOT use for activity statistics — use aggregate_stats."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "aspect": {
                        "type": "string",
                        "enum": ["hrv", "sleep", "readiness", "weight", "resting_hr", "all"],
                        "description": "Which health aspect to retrieve (default: all)",
                        "default": "all",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30)",
                        "default": 30,
                    },
                },
            },
        },
    },

    # ── 4. get_training_load ───────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_training_load",
            "description": (
                "Fetch Performance Management Chart (PMC) data: CTL (Fitness/42-day avg), "
                "ATL (Fatigue/7-day avg), TSB (Form = CTL - ATL), ACWR (ATL/CTL ratio). "
                "Use for 'Wie ist meine Form?', 'Trainiere ich zu viel?', "
                "'Wie hoch ist meine Fitness?', 'Bin ich überlastet?' questions. "
                "TSB interpretation: <-30 überlastet, -10..+5 optimal, >+15 Formverlust. "
                "ACWR: >1.5 = erhöhtes Verletzungsrisiko. "
                "Do NOT use for HRV/sleep — use get_health_status instead."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30, use 42 for full CTL view)",
                        "default": 30,
                    },
                },
            },
        },
    },

    # ── 5. compare_periods ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "compare_periods",
            "description": (
                "Compare two date ranges on activity volume metrics: count, total distance, "
                "total duration, average HR, total elevation. "
                "Use for 'Vergleich März vs. Februar', 'Dieses Jahr vs. letztes Jahr' questions. "
                "All dates in YYYY-MM-DD format. "
                "Do NOT use for health/HRV comparisons — use get_health_status with different day windows."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "period1_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period1_end": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_end": {"type": "string", "description": "YYYY-MM-DD"},
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit comparison to one sport (optional)",
                    },
                },
                "required": ["period1_start", "period1_end", "period2_start", "period2_end"],
            },
        },
    },

    # ── 6. get_gear_stats ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_gear_stats",
            "description": (
                "Retrieve gear/equipment statistics: name, type, brand, model, "
                "total distance, total activities, status (active/stored/retired). "
                "Use for 'Wie viele km haben meine Schuhe?', 'Welche Ausrüstung nutze ich?', "
                "'Muss ich bald neue Schuhe kaufen?' questions. "
                "Do NOT use for activity statistics — use aggregate_stats for that. "
                "Without gear_id returns all active gear sorted by total distance."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "gear_id": {
                        "type": "string",
                        "description": "UUID of specific gear item (optional; omit for all active gear)",
                    },
                    "gear_type": {
                        "type": "string",
                        "enum": ["shoes", "bike", "wetsuit", "helmet", "watch", "other"],
                        "description": "Filter by gear type (optional)",
                    },
                },
            },
        },
    },

    # ── 7. query_geo_features ──────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_geo_features",
            "description": (
                "Query geographic features tagged in activities: peaks (Gipfel), passes (Pässe), "
                "lakes, rivers, forests, settlements, borders, and gastronomy stops (Hütten, Restaurants). "
                "Use for 'Wie viele Gipfel?', 'Welche Pässe?', 'Wo war ich essen?', "
                "'Auf welchen Hütten?' questions. "
                "Do NOT use for route/map queries — use search_routes for that. "
                "Do NOT use for segment PRs — use query_segments."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "feature_type": {
                        "type": "string",
                        "enum": [
                            "peak", "pass", "lake", "river", "forest",
                            "settlement", "border", "gastronomy",
                        ],
                        "description": "Type of geo feature to query",
                    },
                    "min_elevation": {
                        "type": "number",
                        "description": "Minimum elevation in meters (for peaks/passes)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 365)",
                        "default": 365,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results (default 20)",
                        "default": 20,
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to activities of this sport (optional)",
                    },
                },
            },
        },
    },

    # ── 8. query_segments ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_segments",
            "description": (
                "Search segments and their effort history: personal records (PR), "
                "recent attempts, segment distance and elevation. "
                "Use for 'Bestzeit auf X', 'PR auf Hausberg', 'Wie oft habe ich Segment Y gefahren?' questions. "
                "Do NOT use for geo features like peaks — use query_geo_features. "
                "Do NOT use for activity lists — use query_activities."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {
                        "type": "string",
                        "description": "Segment name search (partial match)",
                    },
                    "include_efforts": {
                        "type": "boolean",
                        "description": "Include effort history and PR (default true)",
                        "default": True,
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window for efforts in days (default 365)",
                        "default": 365,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max segments to return (default 5)",
                        "default": 5,
                    },
                },
            },
        },
    },

    # ── 9. get_analytics ──────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "get_analytics",
            "description": (
                "Retrieve ML-computed analytics from the cache: anomaly detection results, "
                "activity clustering, route similarity groups, readiness score, "
                "HRV-performance correlations, DFA alpha1 (aerobic threshold). "
                "Use for 'Gibt es Anomalien?', 'Welche Aktivitäten sind ähnlich?', "
                "'Wie korreliert HRV mit meiner Leistung?' questions. "
                "Do NOT use for raw HRV data — use get_health_status instead. "
                "Results may be cached up to 24h old."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "analysis": {
                        "type": "string",
                        "enum": [
                            "anomaly", "clustering", "readiness", "correlations",
                            "dfa_alpha1", "route_similarity",
                        ],
                        "description": "Which ML analysis result to retrieve",
                    },
                },
                "required": ["analysis"],
            },
        },
    },

    # ── 10. search_routes ─────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "search_routes",
            "description": (
                "Search for activities by geographic area or route similarity. "
                "Use for 'Welche Aktivitäten in der Nähe von X?', "
                "'Ähnliche Routen wie heute?', 'War ich schon mal in Y?' questions. "
                "Do NOT use for segment queries — use query_segments. "
                "Do NOT use for geo feature counts (peaks/huts) — use query_geo_features."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "location_name": {
                        "type": "string",
                        "description": "City or region name to search near",
                    },
                    "activity_id": {
                        "type": "string",
                        "description": "UUID of reference activity for similarity search",
                    },
                    "sport_type": {
                        "type": "string",
                        "enum": _SPORT_TYPES,
                        "description": "Limit to sport type (optional)",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 365)",
                        "default": 365,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max results (default 10)",
                        "default": 10,
                    },
                },
            },
        },
    },

    # ── 11. query_data ────────────────────────────────────────────────────
    {
        "type": "function",
        "function": {
            "name": "query_data",
            "description": (
                "General-purpose data retrieval for questions not covered by specialized tools. "
                "Fetches from: pmc_daily (TSS/CTL/ATL), health_snapshots, health_sleep, "
                "health_weight_entries, activity_laps, activity_splits, activity_intervals. "
                "Use as fallback when no specific tool matches the user's question. "
                "Do NOT use for activities, geo features, segments, or gear — use the specialized tools. "
                "Specify the exact table and desired columns."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "table": {
                        "type": "string",
                        "enum": [
                            "pmc_daily", "health_snapshots", "health_sleep",
                            "health_weight_entries", "activity_laps",
                            "activity_splits", "activity_intervals",
                        ],
                        "description": "Table to query",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Lookback window in days (default 30)",
                        "default": 30,
                    },
                    "limit": {
                        "type": "integer",
                        "description": "Max rows to return (default 30)",
                        "default": 30,
                    },
                    "activity_id": {
                        "type": "string",
                        "description": "UUID to filter by activity (for lap/split/interval tables)",
                    },
                },
                "required": ["table"],
            },
        },
    },
]

# Backward-compatible alias used by existing call sites
TOOL_DEFINITIONS = ALL_TOOL_DEFINITIONS


def get_tool_definitions(tool_names: list[str] | None = None) -> list[dict]:
    """Return tool definitions, optionally filtered by name.

    Args:
        tool_names: List of tool names to include. Pass None to get all tools.

    Returns:
        List of tool definition dicts ready for LLM function-calling API.
    """
    if tool_names is None:
        return ALL_TOOL_DEFINITIONS
    name_set = set(tool_names)
    return [t for t in ALL_TOOL_DEFINITIONS if t["function"]["name"] in name_set]


# ── Helper formatters ──────────────────────────────────────────────────────────

def _format_duration(seconds: int | float) -> str:
    """Format seconds as human-readable duration string."""
    if not seconds:
        return "0min"
    mins = int(seconds) // 60
    if mins >= 60:
        return f"{mins // 60}h{mins % 60:02d}min"
    return f"{mins}min"


def _format_activity_compact(row: dict) -> dict:
    """Format activity record for LLM consumption (short keys, pre-computed values)."""
    distance_km = round(row.get("distance_m", 0) / 1000, 1)
    duration_s = row.get("elapsed_time_s") or row.get("duration_s", 0)
    speed = row.get("avg_speed", 0)

    result = {
        "date": row["started_at"].strftime("%d.%m.%Y") if row.get("started_at") else "",
        "title": row.get("title", ""),
        "sport": row.get("sport_type", ""),
        "km": distance_km,
        "duration": _format_duration(duration_s),
        "hr": row.get("avg_heart_rate"),
        "elevation_m": round(row.get("elevation_gain_m", 0) or 0),
        "city": row.get("start_city", ""),
    }

    # Add pace or speed depending on sport
    if row.get("sport_type") in ("running", "trail_running", "hiking", "walking"):
        if speed and speed > 0:
            pace_min_km = (1000.0 / speed) / 60.0
            result["pace"] = f"{int(pace_min_km)}:{int((pace_min_km % 1) * 60):02d} min/km"
    elif speed and speed > 0:
        result["speed"] = f"{round(speed * 3.6, 1)} km/h"

    return result


# ── Tool execution ─────────────────────────────────────────────────────────────

async def execute_tool(pool, user_id: UUID, tool_name: str, args: dict) -> dict:
    """Execute a tool call and return results."""
    dispatch = {
        "query_activities": _query_activities,
        "aggregate_stats": _aggregate_stats,
        "get_health_status": _get_health_status,
        "get_training_load": _get_training_load,
        "compare_periods": _compare_periods,
        "get_gear_stats": _get_gear_stats,
        "query_geo_features": _query_geo_features,
        "query_segments": _query_segments,
        "get_analytics": _get_analytics,
        "search_routes": _search_routes,
        "query_data": _query_data,
        # Legacy aliases kept for backward compatibility
        "get_hrv_trend": _get_hrv_trend_legacy,
        "get_readiness": _get_readiness_legacy,
    }
    fn = dispatch.get(tool_name)
    if fn is None:
        return {"error": f"Unknown tool: {tool_name}"}
    return await fn(pool, user_id, **args)


# ── Implementations ────────────────────────────────────────────────────────────

_VALID_SORTS = {
    "date": "started_at",
    "distance": "distance_m",
    "duration": "elapsed_time_s",
    "hr": "avg_heart_rate",
    "elevation": "elevation_gain_m",
}


async def _query_activities(
    pool, user_id,
    sport_type=None, days=30, limit=10, sort_by="date",
    min_distance_km=None, title_search=None,
):
    since = date.today() - timedelta(days=days)
    sort_col = _VALID_SORTS.get(sort_by, "started_at")

    conditions = ["a.user_id = $1", "a.started_at >= $2"]
    params: list = [user_id, since]

    if sport_type:
        params.append(sport_type)
        conditions.append(f"a.sport_type = ${len(params)}")
    if min_distance_km is not None:
        params.append(min_distance_km * 1000)
        conditions.append(f"a.distance_m >= ${len(params)}")
    if title_search:
        params.append(f"%{title_search}%")
        conditions.append(f"a.title ILIKE ${len(params)}")

    where = " AND ".join(conditions)
    query = (
        "SELECT a.id, a.title, a.sport_type, a.started_at, "
        "a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, "
        "a.avg_speed, a.start_city "
        f"FROM activities a WHERE {where} "
        f"ORDER BY a.{sort_col} DESC LIMIT {min(int(limit), 50)}"
    )
    rows = await pool.fetch(query, *params)
    return {"activities": [_format_activity_compact(dict(r)) for r in rows]}


async def _aggregate_stats(pool, user_id, metric="count", sport_type=None, days=30, group_by="none"):
    since = date.today() - timedelta(days=days)

    _METRIC_SQL = {
        "total_distance": "COALESCE(SUM(distance_m), 0) / 1000.0 AS value",
        "total_duration": "COALESCE(SUM(elapsed_time_s), 0) / 3600.0 AS value",
        "count": "COUNT(*) AS value",
        "avg_heart_rate": "ROUND(AVG(avg_heart_rate)::numeric, 1) AS value",
        "total_elevation": "COALESCE(SUM(elevation_gain_m), 0) AS value",
        "avg_distance": "ROUND((COALESCE(AVG(distance_m), 0) / 1000.0)::numeric, 2) AS value",
    }
    agg_expr = _METRIC_SQL.get(metric, "COUNT(*) AS value")

    conditions = ["user_id = $1", "started_at >= $2"]
    params: list = [user_id, since]

    if sport_type:
        params.append(sport_type)
        conditions.append(f"sport_type = ${len(params)}")

    where = " AND ".join(conditions)

    if group_by == "week":
        group_expr = "date_trunc('week', started_at)::date AS period"
        query = f"SELECT {group_expr}, {agg_expr} FROM activities WHERE {where} GROUP BY period ORDER BY period"
    elif group_by == "month":
        group_expr = "date_trunc('month', started_at)::date AS period"
        query = f"SELECT {group_expr}, {agg_expr} FROM activities WHERE {where} GROUP BY period ORDER BY period"
    elif group_by == "sport":
        query = f"SELECT sport_type AS period, {agg_expr} FROM activities WHERE {where} GROUP BY sport_type ORDER BY value DESC"
    else:
        query = f"SELECT {agg_expr}, COUNT(*) AS activity_count FROM activities WHERE {where}"

    rows = await pool.fetch(query, *params)
    data = [dict(r) for r in rows]

    _METRIC_UNITS = {
        "total_distance": "km",
        "total_duration": "h",
        "count": "activities",
        "avg_heart_rate": "bpm",
        "total_elevation": "m",
        "avg_distance": "km",
    }

    return {
        "metric": metric,
        "unit": _METRIC_UNITS.get(metric, ""),
        "sport_type": sport_type,
        "days": days,
        "group_by": group_by,
        "data": data,
    }


async def _get_health_status(pool, user_id, aspect="all", days=30):
    since = date.today() - timedelta(days=days)
    result: dict = {}

    if aspect in ("hrv", "all"):
        rows = await pool.fetch(
            "SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms, "
            "extra_data "
            "FROM health_snapshots WHERE user_id = $1 AND measured_at_utc >= $2 "
            "ORDER BY measured_at_utc",
            user_id, since,
        )
        hrv_data = []
        for r in rows:
            entry = {"date": r["date"], "rmssd_ms": r["rmssd_ms"], "sdrr_ms": r["sdrr_ms"]}
            # HRV status fields (weekly_avg, baseline, status) live in extra_data JSONB
            extra = r["extra_data"]
            if extra:
                if isinstance(extra, str):
                    extra = json.loads(extra)
                for key in ("weekly_avg", "baseline_low", "baseline_high", "status"):
                    if key in extra:
                        entry[key] = extra[key]
            hrv_data.append(entry)
        result["hrv"] = {
            "data": hrv_data,
            "latest": hrv_data[-1] if hrv_data else None,
            "avg_rmssd": round(sum(r["rmssd_ms"] or 0 for r in hrv_data) / len(hrv_data), 1) if hrv_data else None,
        }

    if aspect in ("sleep", "all"):
        rows = await pool.fetch(
            "SELECT calendar_date AS date, "
            "(COALESCE(deep_seconds, 0) + COALESCE(light_seconds, 0) + COALESCE(rem_seconds, 0)) AS total_sleep_s, "
            "deep_seconds, light_seconds, rem_seconds, awake_seconds, score_overall AS score "
            "FROM health_sleep WHERE user_id = $1 AND calendar_date >= $2 ORDER BY calendar_date",
            user_id, since,
        )
        sleep_data = [dict(r) for r in rows]
        result["sleep"] = {
            "data": sleep_data,
            "avg_score": round(sum(r.get("score") or 0 for r in sleep_data) / len(sleep_data), 1) if sleep_data else None,
            "avg_total_h": round(sum((r.get("total_sleep_s") or 0) for r in sleep_data) / 3600 / len(sleep_data), 1) if sleep_data else None,
        }

    if aspect in ("readiness", "all"):
        row = await pool.fetchrow(
            "SELECT data FROM analytics_cache "
            "WHERE user_id = $1 AND cache_key = 'readiness' "
            "ORDER BY computed_at DESC LIMIT 1",
            user_id,
        )
        if row and row["data"]:
            result["readiness"] = json.loads(row["data"]) if isinstance(row["data"], str) else row["data"]
        else:
            result["readiness"] = None

    if aspect in ("weight", "all"):
        rows = await pool.fetch(
            "SELECT measured_at_utc::date AS date, "
            "ROUND((weight_g / 1000.0)::numeric, 1) AS weight_kg, body_fat_pct "
            "FROM health_weight_entries WHERE user_id = $1 AND measured_at_utc >= $2 "
            "ORDER BY measured_at_utc",
            user_id, since,
        )
        result["weight"] = [dict(r) for r in rows]

    if aspect in ("resting_hr", "all"):
        rows = await pool.fetch(
            "SELECT calendar_date AS date, resting_heart_rate AS resting_hr_bpm "
            "FROM health_daily_summary WHERE user_id = $1 AND calendar_date >= $2 "
            "AND resting_heart_rate IS NOT NULL ORDER BY calendar_date",
            user_id, since,
        )
        result["resting_hr"] = [dict(r) for r in rows]

    return result


async def _get_training_load(pool, user_id, days=30):
    since = date.today() - timedelta(days=days)
    rows = await pool.fetch(
        "SELECT date, tss, ctl, atl, tsb FROM pmc_daily "
        "WHERE user_id = $1 AND date >= $2 ORDER BY date",
        user_id, since,
    )
    data = [dict(r) for r in rows]
    if data:
        latest = data[-1]
        acwr = round(latest["atl"] / latest["ctl"], 2) if latest.get("ctl") and latest["ctl"] > 0 else None
        return {"pmc_data": data, "current": {**latest, "acwr": acwr}}
    return {"pmc_data": [], "current": None}


async def _compare_periods(
    pool, user_id,
    period1_start, period1_end,
    period2_start, period2_end,
    sport_type=None,
):
    async def _period_stats(start, end):
        conditions = ["user_id = $1", "started_at::date BETWEEN $2 AND $3"]
        params: list = [user_id, date.fromisoformat(start), date.fromisoformat(end)]
        if sport_type:
            params.append(sport_type)
            conditions.append(f"sport_type = ${len(params)}")
        where = " AND ".join(conditions)
        row = await pool.fetchrow(
            f"SELECT COUNT(*) AS count, "
            f"ROUND((COALESCE(SUM(distance_m), 0) / 1000.0)::numeric, 1) AS total_distance_km, "
            f"COALESCE(SUM(elapsed_time_s), 0) AS total_duration_s, "
            f"ROUND(COALESCE(AVG(avg_heart_rate), 0)::numeric, 1) AS avg_heart_rate, "
            f"ROUND(COALESCE(SUM(elevation_gain_m), 0)::numeric, 0) AS total_elevation_m "
            f"FROM activities WHERE {where}",
            *params,
        )
        return dict(row) if row else {}

    p1 = await _period_stats(period1_start, period1_end)
    p2 = await _period_stats(period2_start, period2_end)
    return {
        "period1": {"range": f"{period1_start} – {period1_end}", **p1},
        "period2": {"range": f"{period2_start} – {period2_end}", **p2},
    }


async def _get_gear_stats(pool, user_id, gear_id=None, gear_type=None):
    from uuid import UUID as UUIDType
    conditions = ["user_id = $1"]
    params: list = [user_id]

    if gear_id:
        params.append(UUIDType(gear_id))
        conditions.append(f"id = ${len(params)}")
    else:
        conditions.append("status = 'active'")

    if gear_type:
        params.append(gear_type)
        conditions.append(f"type = ${len(params)}")

    where = " AND ".join(conditions)
    rows = await pool.fetch(
        f"SELECT id, name, type, brand, model, "
        f"ROUND((total_distance_m / 1000.0)::numeric, 0) AS total_distance_km, "
        f"total_activities, status "
        f"FROM gear WHERE {where} ORDER BY total_distance_m DESC",
        *params,
    )
    return {"gear": [dict(r) for r in rows]}


async def _query_geo_features(
    pool, user_id,
    feature_type=None, min_elevation=None, days=365, limit=20, sport_type=None,
):
    since = date.today() - timedelta(days=days)

    # Map feature_type to DB category
    if feature_type == "gastronomy":
        table_join = (
            "JOIN activities a ON a.id = s.activity_id "
            "WHERE a.user_id = $1 AND a.started_at >= $2 AND s.category = 'gastronomy'"
        )
        params: list = [user_id, since]
        select = "s.poi_name AS name, s.poi_type AS subtype, s.lat, s.lng, s.duration_s"
        from_clause = "activity_stops s"
    else:
        conditions = ["a.user_id = $1", "a.started_at >= $2"]
        params = [user_id, since]

        if feature_type:
            params.append(feature_type)
            conditions.append(f"gt.category = ${len(params)}")
        if min_elevation is not None:
            params.append(min_elevation)
            conditions.append(f"gt.elevation >= ${len(params)}")
        if sport_type:
            params.append(sport_type)
            conditions.append(f"a.sport_type = ${len(params)}")

        where = " AND ".join(conditions)
        table_join = f"JOIN activities a ON a.id = gt.activity_id WHERE {where}"
        select = "gt.name, gt.category, gt.elevation, gt.lat, gt.lng, a.started_at::date AS activity_date, a.title AS activity_title"
        from_clause = "activity_geo_tags gt"

    query = (
        f"SELECT {select} FROM {from_clause} {table_join} "
        f"ORDER BY {'gt.elevation DESC NULLS LAST' if feature_type != 'gastronomy' else 's.duration_s DESC'} "
        f"LIMIT {min(int(limit), 100)}"
    )
    rows = await pool.fetch(query, *params)
    data = [dict(r) for r in rows]
    return {"feature_type": feature_type, "count": len(data), "features": data}


async def _query_segments(pool, user_id, query=None, include_efforts=True, days=365, limit=5):
    since = date.today() - timedelta(days=days)

    conditions = ["se.user_id = $1"]
    params: list = [user_id]

    if query:
        params.append(f"%{query}%")
        conditions.append(f"s.name ILIKE ${len(params)}")

    where = " AND ".join(conditions)
    sql = (
        "SELECT s.id, s.name, s.sport_group, "
        "ROUND((s.distance_m / 1000.0)::numeric, 2) AS distance_km, "
        "s.elevation_gain_m, "
        "COUNT(se.id) AS effort_count, "
        "MIN(se.elapsed_time_s) AS pr_seconds, "
        "MAX(se.started_at)::date AS last_attempt "
        "FROM segments s "
        "JOIN segment_efforts se ON se.segment_id = s.id "
        f"WHERE {where} "
        "GROUP BY s.id, s.name, s.sport_group, s.distance_m, s.elevation_gain_m "
        f"ORDER BY last_attempt DESC LIMIT {min(int(limit), 20)}"
    )
    rows = await pool.fetch(sql, *params)

    segments = []
    for r in rows:
        seg = dict(r)
        if seg.get("pr_seconds"):
            seg["pr_formatted"] = _format_duration(seg["pr_seconds"])
        segments.append(seg)

    if include_efforts and segments:
        # Fetch last 3 efforts for each segment
        seg_ids = [r["id"] for r in rows]
        effort_rows = await pool.fetch(
            "SELECT segment_id, started_at::date AS date, elapsed_time_s, avg_hr "
            "FROM segment_efforts "
            "WHERE user_id = $1 AND segment_id = ANY($2) AND started_at >= $3 "
            "ORDER BY started_at DESC",
            user_id, seg_ids, since,
        )
        efforts_by_seg: dict = {}
        for e in effort_rows:
            sid = str(e["segment_id"])
            if sid not in efforts_by_seg:
                efforts_by_seg[sid] = []
            if len(efforts_by_seg[sid]) < 3:
                efforts_by_seg[sid].append({
                    "date": str(e["date"]),
                    "time": _format_duration(e["elapsed_time_s"]),
                    "hr": e["avg_hr"],
                })
        for seg in segments:
            seg["recent_efforts"] = efforts_by_seg.get(str(seg["id"]), [])

    return {"segments": segments}


async def _get_analytics(pool, user_id, analysis):
    row = await pool.fetchrow(
        "SELECT data, computed_at FROM analytics_cache "
        "WHERE user_id = $1 AND cache_key = $2 "
        "ORDER BY computed_at DESC LIMIT 1",
        user_id, analysis,
    )
    if row:
        data = json.loads(row["data"]) if isinstance(row["data"], str) else row["data"]
        return {"analysis": analysis, "computed_at": str(row["computed_at"])[:19], "data": data}
    return {"analysis": analysis, "data": None, "note": "No cached result available"}


async def _search_routes(pool, user_id, location_name=None, activity_id=None, sport_type=None, days=365, limit=10):
    since = date.today() - timedelta(days=days)

    conditions = ["a.user_id = $1", "a.started_at >= $2"]
    params: list = [user_id, since]

    if location_name:
        params.append(f"%{location_name}%")
        conditions.append(
            f"(a.start_city ILIKE ${len(params)} OR a.start_location ILIKE ${len(params)})"
        )
    if sport_type:
        params.append(sport_type)
        conditions.append(f"a.sport_type = ${len(params)}")

    where = " AND ".join(conditions)
    rows = await pool.fetch(
        f"SELECT a.id, a.title, a.sport_type, a.started_at::date AS date, "
        f"ROUND((a.distance_m / 1000.0)::numeric, 1) AS distance_km, "
        f"a.elevation_gain_m, a.start_city "
        f"FROM activities a WHERE {where} "
        f"ORDER BY a.started_at DESC LIMIT {min(int(limit), 50)}",
        *params,
    )
    return {"routes": [dict(r) for r in rows]}


async def _query_data(pool, user_id, table, days=30, limit=30, activity_id=None):
    since = date.today() - timedelta(days=days)

    _TABLE_CONFIG = {
        "pmc_daily": {
            "cols": "date, tss, ctl, atl, tsb",
            "date_col": "date",
            "user_col": "user_id",
        },
        "health_snapshots": {
            "cols": "measured_at_utc::date AS date, rmssd_ms, sdrr_ms, extra_data",
            "date_col": "measured_at_utc",
            "user_col": "user_id",
        },
        "health_sleep": {
            "cols": "calendar_date AS date, "
                    "(COALESCE(deep_seconds,0)+COALESCE(light_seconds,0)+COALESCE(rem_seconds,0)) AS total_sleep_s, "
                    "deep_seconds, score_overall AS score",
            "date_col": "calendar_date",
            "user_col": "user_id",
        },
        "health_weight_entries": {
            "cols": "measured_at_utc::date AS date, ROUND((weight_g / 1000.0)::numeric, 1) AS weight_kg, body_fat_pct",
            "date_col": "measured_at_utc",
            "user_col": "user_id",
        },
        "activity_laps": {
            "cols": "message_index, total_distance_m, total_elapsed_time_s, avg_heart_rate, max_heart_rate",
            "date_col": None,
            "user_col": None,
            "activity_join": True,
        },
        "activity_splits": {
            "cols": "message_index, split_type, total_distance_m, total_elapsed_time_s, avg_speed_ms",
            "date_col": None,
            "user_col": None,
            "activity_join": True,
        },
        "activity_intervals": {
            "cols": "segment_num, duration_s, avg_heart_rate, avg_power_w, avg_speed_ms",
            "date_col": None,
            "user_col": None,
            "activity_join": True,
        },
    }

    cfg = _TABLE_CONFIG.get(table)
    if not cfg:
        return {"error": f"Table '{table}' not allowed"}

    if cfg.get("activity_join"):
        if not activity_id:
            return {"error": f"activity_id required for table '{table}'"}
        from uuid import UUID as UUIDType
        rows = await pool.fetch(
            f"SELECT {cfg['cols']} FROM {table} "
            f"WHERE activity_id = $1 ORDER BY 1 LIMIT {min(int(limit), 200)}",
            UUIDType(activity_id),
        )
    else:
        rows = await pool.fetch(
            f"SELECT {cfg['cols']} FROM {table} "
            f"WHERE {cfg['user_col']} = $1 AND {cfg['date_col']} >= $2 "
            f"ORDER BY {cfg['date_col']} DESC LIMIT {min(int(limit), 200)}",
            user_id, since,
        )

    return {"table": table, "rows": [dict(r) for r in rows]}


# ── Legacy compatibility wrappers ──────────────────────────────────────────────

async def _get_hrv_trend_legacy(pool, user_id, days=30):
    """Legacy: get_hrv_trend → get_health_status(aspect='hrv')."""
    result = await _get_health_status(pool, user_id, aspect="hrv", days=days)
    return {"hrv_data": result.get("hrv", {}).get("data", [])}


async def _get_readiness_legacy(pool, user_id):
    """Legacy: get_readiness → get_health_status(aspect='readiness')."""
    result = await _get_health_status(pool, user_id, aspect="readiness")
    return {"readiness": result.get("readiness")}
