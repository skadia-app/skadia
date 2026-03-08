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
            SELECT rmssd_ms FROM health_snapshots
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


async def compute_readiness_v2(pool, user_id) -> dict:
    """Readiness v2: HRV-CV, ACWR, changepoint proximity penalty, confidence score.

    Stores result in analytics_cache under 'readiness_v2'.
    """
    import json
    import math

    result = {
        "score": 50,
        "level": "moderat",
        "confidence": 0.0,
        "factors": [],
        "explanation": "",
    }
    data_points = 0

    async with pool.acquire() as conn:
        # 1. HRV-CV (7-day rolling coefficient of variation from user_daily_features)
        hrv_rows = await conn.fetch(
            """
            SELECT value_num FROM user_daily_features
            WHERE user_id = $1 AND feature_key = 'hrv.HRV_RMSSD'
              AND date >= CURRENT_DATE - 7 AND value_num IS NOT NULL
            ORDER BY date
            """,
            user_id,
        )
        hrv_cv_factor = 50.0
        hrv_cv_label = "keine Daten"
        if len(hrv_rows) >= 3:
            hrv_vals = [float(r["value_num"]) for r in hrv_rows]
            mean_hrv = sum(hrv_vals) / len(hrv_vals)
            if mean_hrv > 0:
                std_hrv = (sum((v - mean_hrv) ** 2 for v in hrv_vals) / len(hrv_vals)) ** 0.5
                cv = std_hrv / mean_hrv
                # Low CV = stable HRV = good; CV > 0.3 = unstable = bad
                hrv_cv_factor = max(0.0, min(100.0, 100.0 - cv * 250.0))
                hrv_cv_label = "stabil" if cv < 0.1 else "variabel" if cv < 0.2 else "instabil"
            data_points += 1

        # 2. ACWR from pmc_daily (atl/ctl ratio)
        pmc_row = await conn.fetchrow(
            """
            SELECT atl, ctl FROM pmc_daily
            WHERE user_id = $1 ORDER BY date DESC LIMIT 1
            """,
            user_id,
        )
        acwr_factor = 50.0
        acwr_label = "keine Daten"
        if pmc_row and pmc_row["atl"] is not None and pmc_row["ctl"] is not None:
            ctl = float(pmc_row["ctl"])
            atl = float(pmc_row["atl"])
            if ctl > 0:
                acwr = atl / ctl
                # ACWR 0.8-1.3 optimal; >1.5 overreach risk; <0.5 detraining
                if 0.8 <= acwr <= 1.3:
                    acwr_factor = 85.0
                elif acwr > 1.5:
                    acwr_factor = max(0.0, 85.0 - (acwr - 1.3) * 100.0)
                else:
                    acwr_factor = max(0.0, acwr / 0.8 * 70.0)
                acwr_label = f"ACWR {acwr:.2f}"
            data_points += 1

        # 3. Recent changepoint proximity penalty
        cp_penalty = 0.0
        cp_label = ""
        try:
            cp_row = await conn.fetchrow(
                """
                SELECT data FROM analytics_cache
                WHERE user_id = $1 AND cache_key = 'changepoints.hrv_rmssd'
                """,
                user_id,
            )
            if cp_row and cp_row["data"]:
                changepoints = json.loads(cp_row["data"])
                from datetime import date as date_cls, timedelta
                cutoff = date_cls.today() - timedelta(days=14)
                recent_cps = [
                    cp for cp in changepoints
                    if cp.get("date") and cp["date"] >= cutoff.isoformat()
                ]
                if recent_cps:
                    # Penalty based on how many and how large the changes are
                    cp_penalty = min(20.0, len(recent_cps) * 7.0)
                    cp_label = f"{len(recent_cps)} Changepoint(s) in letzten 14 Tagen"
        except Exception:
            pass

        # 4. Sleep score
        sleep_row = await conn.fetchrow(
            """
            SELECT score_overall FROM health_sleep
            WHERE user_id = $1 ORDER BY calendar_date DESC LIMIT 1
            """,
            user_id,
        )
        sleep_factor = 50.0
        if sleep_row and sleep_row["score_overall"] is not None:
            sleep_factor = float(sleep_row["score_overall"])
            data_points += 1

    # Confidence: 0-1 based on available data sources (max 3)
    confidence = round(min(1.0, data_points / 3.0), 2)

    # Weighted score
    raw_score = (hrv_cv_factor * 0.35 + acwr_factor * 0.35 + sleep_factor * 0.30) - cp_penalty
    score = int(round(max(0, min(100, raw_score))))

    level = "hoch" if score >= 70 else "moderat" if score >= 40 else "niedrig"

    factors = [
        {"name": "HRV-Stabilität", "value": round(hrv_cv_factor), "label": hrv_cv_label},
        {"name": "ACWR", "value": round(acwr_factor), "label": acwr_label},
        {"name": "Schlaf", "value": round(sleep_factor), "label": ""},
    ]
    if cp_penalty > 0:
        factors.append({"name": "Changepoint-Malus", "value": -round(cp_penalty), "label": cp_label})

    explanation_parts = [f"HRV-Stabilität: {round(hrv_cv_factor)}% ({hrv_cv_label})"]
    explanation_parts.append(f"ACWR: {round(acwr_factor)}% ({acwr_label})")
    explanation_parts.append(f"Schlaf: {round(sleep_factor)}%")
    if cp_label:
        explanation_parts.append(f"Malus: {cp_label}")

    result = {
        "score": score,
        "level": level,
        "confidence": confidence,
        "factors": factors,
        "explanation": " | ".join(explanation_parts),
    }

    # Store in analytics_cache
    async with pool.acquire() as conn:
        import json as _json
        await conn.execute(
            """
            INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
            VALUES ($1, 'readiness_v2', $2::jsonb, NOW())
            ON CONFLICT (user_id, cache_key)
            DO UPDATE SET data = $2::jsonb, computed_at = NOW()
            """,
            user_id, _json.dumps(result),
        )

    return result
