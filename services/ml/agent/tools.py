"""Tool definitions for LLM tool-use (function calling)."""
import json
import logging
from datetime import date, timedelta
from uuid import UUID

logger = logging.getLogger(__name__)


TOOL_DEFINITIONS = [
    {
        "type": "function",
        "function": {
            "name": "query_activities",
            "description": (
                "Suche Aktivitäten des Users. Gibt Titel, Sportart, Datum, Distanz (m), "
                "Dauer (s), Höhenmeter, Durchschnitts-HR und Startort zurück. "
                "sport_type DB-Werte: running, cycling, hiking, walking, swimming, "
                "alpine_skiing, cross_country_skiing, strength_training, trail_running, "
                "mountain_biking, open_water_swimming. "
                "Setze days=365 für 'letzte' Fragen wenn 30 Tage zu kurz sein könnten."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "sport_type": {
                        "type": "string",
                        "description": "DB sport_type (z.B. 'cycling', 'running')",
                    },
                    "days": {
                        "type": "integer",
                        "description": "Zeitraum in Tagen rückwärts (default 30, nutze 365 für 'letztes Mal')",
                        "default": 30,
                    },
                    "limit": {"type": "integer", "description": "Max Ergebnisse", "default": 10},
                    "sort_by": {
                        "type": "string",
                        "enum": ["date", "distance", "duration", "hr"],
                        "default": "date",
                    },
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_hrv_trend",
            "description": "HRV-Verlauf (RMSSD, SDRR) für N Tage. Für Erholungs- und Stressbewertung.",
            "parameters": {
                "type": "object",
                "properties": {
                    "days": {"type": "integer", "default": 30},
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_training_load",
            "description": (
                "Trainingsbelastung (PMC): CTL (Fitness), ATL (Ermüdung), TSB (Form), "
                "ACWR (Acute:Chronic Ratio). Für Belastungssteuerung."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "days": {"type": "integer", "default": 30},
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "compare_periods",
            "description": "Vergleiche zwei Zeiträume (Anzahl, Distanz, Dauer, HR, Höhenmeter). Datumsformat: YYYY-MM-DD.",
            "parameters": {
                "type": "object",
                "properties": {
                    "period1_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period1_end": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_start": {"type": "string", "description": "YYYY-MM-DD"},
                    "period2_end": {"type": "string", "description": "YYYY-MM-DD"},
                },
                "required": ["period1_start", "period1_end", "period2_start", "period2_end"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_readiness",
            "description": "Aktuelle Trainingsbereitschaft (Score, Faktoren wie HRV-Trend, Ermüdung, Schlaf).",
            "parameters": {"type": "object", "properties": {}},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "get_gear_stats",
            "description": "Ausrüstungs-Statistik (Name, Typ, Marke, Gesamtdistanz, Status). Ohne gear_id: alle aktiven.",
            "parameters": {
                "type": "object",
                "properties": {
                    "gear_id": {"type": "string", "description": "Optional: UUID eines bestimmten Gear"},
                },
            },
        },
    },
]


async def execute_tool(pool, user_id: UUID, tool_name: str, args: dict) -> dict:
    """Execute a tool call and return results."""
    if tool_name == "query_activities":
        return await _query_activities(pool, user_id, **args)
    elif tool_name == "get_hrv_trend":
        return await _get_hrv_trend(pool, user_id, **args)
    elif tool_name == "get_training_load":
        return await _get_training_load(pool, user_id, **args)
    elif tool_name == "get_readiness":
        return await _get_readiness(pool, user_id)
    elif tool_name == "get_gear_stats":
        return await _get_gear_stats(pool, user_id, **args)
    elif tool_name == "compare_periods":
        return await _compare_periods(pool, user_id, **args)
    else:
        return {"error": f"Unknown tool: {tool_name}"}


async def _query_activities(pool, user_id, sport_type=None, days=30, limit=10, sort_by="date"):
    since = date.today() - timedelta(days=days)
    _VALID_SORTS = {
        "date": "started_at",
        "distance": "distance_m",
        "duration": "elapsed_time_s",
        "hr": "avg_heart_rate",
    }
    if sort_by not in _VALID_SORTS:
        sort_by = "date"
    sort_col = _VALID_SORTS[sort_by]

    # Include start_location from reverse geocoding
    base = (
        "SELECT a.id, a.title, a.sport_type, a.started_at, "
        "a.distance_m, a.elapsed_time_s, a.avg_heart_rate, a.elevation_gain_m, "
        "a.start_location, a.start_city "
        "FROM activities a "
    )
    if sport_type:
        query = (
            base +
            f"WHERE a.user_id = $1 AND a.started_at >= $2 AND a.sport_type = $3 "
            f"ORDER BY a.{sort_col} DESC LIMIT {min(limit, 50)}"
        )
        rows = await pool.fetch(query, user_id, since, sport_type)
    else:
        query = (
            base +
            f"WHERE a.user_id = $1 AND a.started_at >= $2 "
            f"ORDER BY a.{sort_col} DESC LIMIT {min(limit, 50)}"
        )
        rows = await pool.fetch(query, user_id, since)

    activities = []
    for r in rows:
        d = dict(r)
        # Human-readable formatting
        if d.get("distance_m"):
            d["distance_km"] = round(d["distance_m"] / 1000, 1)
        if d.get("elapsed_time_s"):
            mins = d["elapsed_time_s"] // 60
            d["duration_str"] = f"{mins // 60}h{mins % 60:02d}min" if mins >= 60 else f"{mins}min"
        if d.get("started_at"):
            d["date"] = str(d["started_at"])[:10]
        activities.append(d)

    return {"activities": activities}


async def _get_hrv_trend(pool, user_id, days=30):
    since = date.today() - timedelta(days=days)
    rows = await pool.fetch(
        "SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms "
        "FROM health_hrv_snapshots WHERE user_id = $1 AND measured_at_utc >= $2 "
        "ORDER BY measured_at_utc",
        user_id, since,
    )
    return {"hrv_data": [dict(r) for r in rows]}


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
        acwr = round(latest["atl"] / latest["ctl"], 2) if latest["ctl"] and latest["ctl"] > 0 else None
        return {"pmc_data": data, "current": {**latest, "acwr": acwr}}
    return {"pmc_data": [], "current": None}


async def _get_readiness(pool, user_id):
    row = await pool.fetchrow(
        "SELECT data FROM analytics_cache "
        "WHERE user_id = $1 AND cache_key = 'readiness' "
        "ORDER BY computed_at DESC LIMIT 1",
        user_id,
    )
    if row:
        return json.loads(row["data"])
    return {"readiness": None}


async def _get_gear_stats(pool, user_id, gear_id=None):
    if gear_id:
        from uuid import UUID as UUIDType
        rows = await pool.fetch(
            "SELECT id, name, type, brand, model, total_distance_m, status "
            "FROM gear WHERE user_id = $1 AND id = $2",
            user_id, UUIDType(gear_id),
        )
    else:
        rows = await pool.fetch(
            "SELECT id, name, type, brand, model, total_distance_m, status "
            "FROM gear WHERE user_id = $1 AND status = 'active' "
            "ORDER BY total_distance_m DESC",
            user_id,
        )
    return {"gear": [dict(r) for r in rows]}


async def _compare_periods(pool, user_id, period1_start, period1_end, period2_start, period2_end):
    async def _period_stats(start, end):
        row = await pool.fetchrow(
            "SELECT COUNT(*) AS count, "
            "COALESCE(SUM(distance_m), 0) AS total_distance, "
            "COALESCE(SUM(elapsed_time_s), 0) AS total_duration, "
            "COALESCE(AVG(avg_heart_rate), 0) AS avg_heart_rate, "
            "COALESCE(SUM(elevation_gain_m), 0) AS total_elevation "
            "FROM activities WHERE user_id = $1 AND started_at::date BETWEEN $2 AND $3",
            user_id, date.fromisoformat(start), date.fromisoformat(end),
        )
        return dict(row) if row else {}

    p1 = await _period_stats(period1_start, period1_end)
    p2 = await _period_stats(period2_start, period2_end)
    return {"period1": p1, "period2": p2}
