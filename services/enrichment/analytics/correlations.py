"""Pearson-Korrelationsanalyse zwischen Training und Gesundheits-Metriken."""
import logging
import math

logger = logging.getLogger(__name__)

_MIN_PAIRS = 10


async def compute_correlations(conn, user_id: str) -> dict:
    """Berechnet Pearson-Korrelationen zwischen Sport- und Gesundheits-Metriken.

    Analysierte Paare:
      1. HRV (Vortag) <-> TSS (Folgetag)
      2. Schlaf-Score <-> TSS des nächsten Tages
      3. Temperatur <-> Pace (min/km)
      4. Wochentag <-> TSS

    Rückgabe:
      {correlations: [{feature_a, feature_b, r, interpretation, n}]}
    """
    correlations = []

    # --- 1. HRV (Vortag) <-> TSS (Folgetag) ---
    try:
        rows = await conn.fetch(
            """
            WITH hrv_daily AS (
                SELECT measured_at_utc::date AS date, AVG(rmssd_ms) AS rmssd
                FROM health_hrv_snapshots
                WHERE user_id = $1
                  AND measured_at_utc > NOW() - interval '180 days'
                GROUP BY 1
            ),
            tss_daily AS (
                SELECT date, tss FROM pmc_daily
                WHERE user_id = $1
                  AND date > CURRENT_DATE - 180
            )
            SELECT h.rmssd, t.tss
            FROM hrv_daily h
            JOIN tss_daily t ON t.date = h.date + 1
            WHERE h.rmssd IS NOT NULL AND t.tss IS NOT NULL
            """,
            user_id,
        )
        pairs = [(float(r["rmssd"]), float(r["tss"])) for r in rows]
        if len(pairs) >= _MIN_PAIRS:
            r, n = _pearsonr(pairs)
            if r is not None:
                correlations.append(
                    {
                        "feature_a": "hrv_rmssd_ms",
                        "feature_b": "tss_next_day",
                        "r": round(r, 3),
                        "interpretation": _interpret(r, inverse=True),
                        "n": n,
                    }
                )
    except Exception as e:
        logger.warning("correlations: HRV<->TSS query failed: %s", e)

    # --- 2. Schlaf-Score <-> TSS (Folgetag) ---
    try:
        rows = await conn.fetch(
            """
            WITH sleep_daily AS (
                SELECT calendar_date AS date, score_overall AS sleep_score
                FROM health_sleep
                WHERE user_id = $1
                  AND calendar_date > CURRENT_DATE - 180
                  AND score_overall IS NOT NULL
            ),
            tss_daily AS (
                SELECT date, tss FROM pmc_daily
                WHERE user_id = $1
                  AND date > CURRENT_DATE - 180
            )
            SELECT s.sleep_score, t.tss
            FROM sleep_daily s
            JOIN tss_daily t ON t.date = s.date + 1
            WHERE t.tss IS NOT NULL
            """,
            user_id,
        )
        pairs = [(float(r["sleep_score"]), float(r["tss"])) for r in rows]
        if len(pairs) >= _MIN_PAIRS:
            r, n = _pearsonr(pairs)
            if r is not None:
                correlations.append(
                    {
                        "feature_a": "sleep_score",
                        "feature_b": "tss_next_day",
                        "r": round(r, 3),
                        "interpretation": _interpret(r),
                        "n": n,
                    }
                )
    except Exception as e:
        logger.warning("correlations: Sleep<->TSS query failed: %s", e)

    # --- 3. Temperatur <-> Pace ---
    try:
        rows = await conn.fetch(
            """
            SELECT weather_temp_c, moving_time_s, distance_m
            FROM activities
            WHERE user_id = $1
              AND sport_type IN ('running', 'trail_running')
              AND weather_temp_c IS NOT NULL
              AND moving_time_s > 0
              AND distance_m > 0
              AND started_at > NOW() - interval '365 days'
            """,
            user_id,
        )
        pairs = []
        for r in rows:
            pace_min_km = (
                float(r["moving_time_s"]) / 60.0 / (float(r["distance_m"]) / 1000.0)
            )
            pairs.append((float(r["weather_temp_c"]), pace_min_km))
        if len(pairs) >= _MIN_PAIRS:
            r, n = _pearsonr(pairs)
            if r is not None:
                correlations.append(
                    {
                        "feature_a": "temperature_c",
                        "feature_b": "pace_min_km",
                        "r": round(r, 3),
                        "interpretation": _interpret(r),
                        "n": n,
                    }
                )
    except Exception as e:
        logger.warning("correlations: Temp<->Pace query failed: %s", e)

    # --- 4. Wochentag <-> TSS ---
    try:
        rows = await conn.fetch(
            """
            SELECT EXTRACT(DOW FROM started_at) AS weekday, training_stress_score
            FROM activities
            WHERE user_id = $1
              AND training_stress_score IS NOT NULL
              AND training_stress_score > 0
              AND started_at > NOW() - interval '365 days'
            """,
            user_id,
        )
        pairs = [
            (float(r["weekday"]), float(r["training_stress_score"])) for r in rows
        ]
        if len(pairs) >= _MIN_PAIRS:
            r, n = _pearsonr(pairs)
            if r is not None:
                correlations.append(
                    {
                        "feature_a": "weekday",
                        "feature_b": "tss",
                        "r": round(r, 3),
                        "interpretation": _interpret(r),
                        "n": n,
                    }
                )
    except Exception as e:
        logger.warning("correlations: Weekday<->TSS query failed: %s", e)

    return {"correlations": correlations}


def _pearsonr(pairs: list) -> tuple:
    """Berechnet den Pearson-Korrelationskoeffizienten für eine Liste von (x, y)-Paaren.

    Gibt (r, n) zurück. r ist None bei Fehler oder Degeneration.
    Nutzt scipy falls verfügbar, sonst manuelle Berechnung.
    """
    n = len(pairs)
    if n < 3:
        return None, n

    try:
        from scipy.stats import pearsonr as scipy_pearsonr

        xs = [p[0] for p in pairs]
        ys = [p[1] for p in pairs]
        r, _ = scipy_pearsonr(xs, ys)
        if math.isnan(r):
            return None, n
        return float(r), n
    except ImportError:
        pass
    except Exception:
        return None, n

    # Manuelle Berechnung (Fallback ohne scipy)
    try:
        xs = [p[0] for p in pairs]
        ys = [p[1] for p in pairs]
        mean_x = sum(xs) / n
        mean_y = sum(ys) / n
        num = sum((x - mean_x) * (y - mean_y) for x, y in zip(xs, ys))
        den_x = math.sqrt(sum((x - mean_x) ** 2 for x in xs))
        den_y = math.sqrt(sum((y - mean_y) ** 2 for y in ys))
        if den_x == 0 or den_y == 0:
            return None, n
        r = num / (den_x * den_y)
        return float(r), n
    except Exception as e:
        logger.warning("correlations: manual pearsonr failed: %s", e)
        return None, n


def _interpret(r: float, inverse: bool = False) -> str:
    """Gibt eine textuelle Interpretation des Pearson-r zurück."""
    abs_r = abs(r)

    if abs_r < 0.1:
        strength = "keine"
    elif abs_r < 0.3:
        strength = "schwache"
    elif abs_r < 0.5:
        strength = "moderate"
    elif abs_r < 0.7:
        strength = "starke"
    else:
        strength = "sehr starke"

    # Vorzeichen-Logik: bei inverse=True ist ein negativer r ein erwartetes Muster
    # (z.B. hohe HRV → niedriger TSS am Folgetag wäre positiv)
    effective_r = -r if inverse else r
    direction = "positive" if effective_r >= 0 else "negative"

    return f"{strength} {direction} Korrelation"
