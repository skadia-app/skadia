"""Readiness score computation from HRV, ATL and sleep data."""
import logging
import statistics

logger = logging.getLogger(__name__)


async def compute_readiness(conn, user_id: str) -> dict:
    """Berechnet einen Readiness-Score (0-100) aus HRV, ATL und Schlaf.

    Gewichtung:
      - HRV-Trend  40 %
      - Ermüdung   30 %  (invertiertes ATL)
      - Schlaf     30 %
    """
    try:
        # 1. HRV 7-Tage-Trend
        hrv_rows = await conn.fetch(
            """
            SELECT rmssd_ms FROM health_hrv_snapshots
            WHERE user_id = $1 AND measured_at_utc > NOW() - interval '7 days'
            ORDER BY measured_at_utc
            """,
            user_id,
        )

        # 2. Letzter PMC-ATL-Wert
        atl_row = await conn.fetchrow(
            """
            SELECT atl FROM pmc_daily
            WHERE user_id = $1 ORDER BY date DESC LIMIT 1
            """,
            user_id,
        )

        # 3. Letzter Schlaf-Score
        sleep_row = await conn.fetchrow(
            """
            SELECT score_overall FROM health_sleep
            WHERE user_id = $1 ORDER BY calendar_date DESC LIMIT 1
            """,
            user_id,
        )
    except Exception as e:
        logger.error("compute_readiness: DB query failed for user %s: %s", user_id, e)
        return _empty_readiness()

    # Faktoren berechnen
    hrv_values = [r["rmssd_ms"] for r in hrv_rows if r["rmssd_ms"] is not None]
    hrv_factor = _compute_hrv_factor(hrv_values)

    atl = atl_row["atl"] if atl_row and atl_row["atl"] is not None else 50.0
    # ATL 0 → 100 (frisch), ATL 150+ → 0 (sehr müde)
    atl_factor = max(0.0, min(100.0, 100.0 - (float(atl) / 150.0) * 100.0))

    sleep_score = (
        sleep_row["score_overall"]
        if sleep_row and sleep_row["score_overall"] is not None
        else 50
    )
    sleep_factor = float(sleep_score)

    score = int(round(hrv_factor * 0.4 + atl_factor * 0.3 + sleep_factor * 0.3))

    def status(v: float) -> str:
        if v >= 70:
            return "gut"
        if v >= 40:
            return "moderat"
        return "niedrig"

    return {
        "score": score,
        "level": "hoch" if score >= 70 else "moderat" if score >= 40 else "niedrig",
        "factors": [
            {
                "name": "HRV-Trend",
                "value": round(hrv_factor),
                "status": status(hrv_factor),
            },
            {
                "name": "Ermüdung",
                "value": round(atl_factor),
                "status": status(atl_factor),
            },
            {
                "name": "Schlaf",
                "value": round(sleep_factor),
                "status": status(sleep_factor),
            },
        ],
    }


def _compute_hrv_factor(values: list) -> float:
    """Berechnet einen 0-100-Faktor aus dem 7-Tage-HRV-Trend.

    Vergleicht die letzten 3 Werte mit dem Gesamt-Mittelwert.
    ratio 1.0 → 100 (über Baseline), ratio 0.7 → 0 (deutlich darunter).
    """
    if len(values) < 3:
        return 50.0

    baseline = statistics.mean(values)
    if baseline == 0:
        return 50.0

    recent = statistics.mean(values[-3:])
    ratio = recent / baseline
    # ratio 0.7 → 0, ratio 1.3 → 100, linear clipped
    factor = (ratio - 0.7) / (1.3 - 0.7) * 100.0
    return min(100.0, max(0.0, factor))


def _empty_readiness() -> dict:
    """Gibt einen leeren Readiness-Dict zurück (Fehlerfall)."""
    return {
        "score": 50,
        "level": "moderat",
        "factors": [],
    }
