"""Fallback template queries — works without LLM."""
import re
from typing import Optional


TEMPLATES = {
    "last_activities": {
        "patterns": [r"letzte\s+(\d+)?\s*aktivität", r"recent\s+(\d+)?\s*activit"],
        "sql": "SELECT id, title, sport_type, started_at, distance_m, duration_s FROM activities WHERE user_id = $1 ORDER BY started_at DESC LIMIT $2",
        "params": lambda m: [int(m.group(1) or 5)],
        "format": "Hier sind deine letzten {n} Aktivitäten:",
    },
    "weekly_summary": {
        "patterns": [r"wochenzu", r"diese\s*woche", r"weekly\s*summary"],
        "sql": """
            SELECT sport_type, COUNT(*) AS count,
                   ROUND(SUM(distance_m)/1000.0, 1) AS km,
                   ROUND(SUM(duration_s)/3600.0, 1) AS hours
            FROM activities
            WHERE user_id = $1 AND started_at >= NOW() - INTERVAL '7 days'
            GROUP BY sport_type ORDER BY count DESC
        """,
        "params": lambda m: [],
        "format": "Wochenzusammenfassung:",
    },
    "hrv_trend": {
        "patterns": [r"hrv.*(trend|verlauf)", r"herzratenvariab"],
        "sql": """
            SELECT measured_at_utc::date AS date, rmssd_ms, sdrr_ms
            FROM health_hrv_snapshots
            WHERE user_id = $1
            ORDER BY measured_at_utc DESC LIMIT 14
        """,
        "params": lambda m: [],
        "format": "HRV-Trend der letzten 14 Tage:",
    },
    "readiness": {
        "patterns": [r"readiness|erholung|wie\s*(geht|fit|erholt)"],
        "sql": """
            SELECT data FROM analytics_cache
            WHERE user_id = $1 AND cache_key = 'readiness'
            ORDER BY computed_at DESC LIMIT 1
        """,
        "params": lambda m: [],
        "format": "Aktuelle Readiness:",
    },
}


def match_template(message: str) -> Optional[dict]:
    """Match user message to a template. Returns template dict or None."""
    msg_lower = message.lower().strip()
    for name, tmpl in TEMPLATES.items():
        for pattern in tmpl["patterns"]:
            m = re.search(pattern, msg_lower)
            if m:
                return {"name": name, "template": tmpl, "match": m}
    return None
