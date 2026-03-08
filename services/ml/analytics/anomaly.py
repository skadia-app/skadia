"""HRV anomaly detection via rolling z-score."""
import logging

import numpy as np

logger = logging.getLogger(__name__)

_MIN_DATA_POINTS = 14
_WINDOW_DAYS = 30
_ZSCORE_THRESHOLD = -2.0


async def detect_anomalies(conn, user_id: str) -> list:
    """Erkennt HRV-Anomalien über einen rollierenden 30-Tage-Baseline-Z-Score.

    Gibt eine Liste von {date, rmssd_ms, zscore, baseline_mean, baseline_std}
    für alle Tage zurück, an denen der Z-Score unter -2.0 fiel.

    Graceful degradation: liefert [] wenn weniger als 14 Datenpunkte vorhanden.
    """
    try:
        rows = await conn.fetch(
            """
            SELECT measured_at_utc::date AS date, rmssd_ms
            FROM health_hrv_snapshots
            WHERE user_id = $1
              AND measured_at_utc > NOW() - interval '90 days'
              AND rmssd_ms IS NOT NULL
            ORDER BY measured_at_utc
            """,
            user_id,
        )
    except Exception as e:
        logger.error("detect_anomalies: DB query failed for user %s: %s", user_id, e)
        return []

    if len(rows) < _MIN_DATA_POINTS:
        logger.debug(
            "detect_anomalies: insufficient data for user %s (%d points, need %d)",
            user_id,
            len(rows),
            _MIN_DATA_POINTS,
        )
        return []

    # Dedupliziere auf Tagesbasis (letzter Messwert pro Tag)
    daily: dict = {}
    for row in rows:
        daily[str(row["date"])] = float(row["rmssd_ms"])

    dates = sorted(daily.keys())
    values = [daily[d] for d in dates]
    arr = np.array(values, dtype=np.float64)

    anomalies = []
    for i in range(len(arr)):
        # Nehme die letzten _WINDOW_DAYS Datenpunkte vor Position i (exkl.)
        window_start = max(0, i - _WINDOW_DAYS)
        window = arr[window_start:i]
        if len(window) < 7:
            # Zu wenig Baseline-Daten — überspringen
            continue

        baseline_mean = float(np.mean(window))
        baseline_std = float(np.std(window, ddof=1))
        if baseline_std == 0:
            continue

        zscore = (arr[i] - baseline_mean) / baseline_std
        if zscore < _ZSCORE_THRESHOLD:
            anomalies.append(
                {
                    "date": dates[i],
                    "rmssd_ms": arr[i],
                    "zscore": round(float(zscore), 3),
                    "baseline_mean": round(baseline_mean, 1),
                    "baseline_std": round(baseline_std, 1),
                }
            )

    return anomalies


async def detect_anomalies_v2(pool, user_id) -> list:
    """Multi-metric rolling Z-score anomaly detection with severity levels.

    Metrics: HR (resting), HRV RMSSD, Sleep score, SpO2.
    Severity: low (|z|>=2), medium (|z|>=2.5), high (|z|>=3).
    Stores result in analytics_cache under 'anomalies_v2'.
    """
    import json
    from datetime import timedelta

    METRIC_QUERIES = {
        "resting_hr": """
            SELECT measured_at_utc::date AS date,
                   (extra_data->>'resting_hr')::float AS value
            FROM health_hrv_snapshots
            WHERE user_id = $1
              AND measured_at_utc > NOW() - interval '90 days'
              AND extra_data->>'resting_hr' IS NOT NULL
            ORDER BY measured_at_utc::date
        """,
        "hrv_rmssd": """
            SELECT measured_at_utc::date AS date, rmssd_ms AS value
            FROM health_hrv_snapshots
            WHERE user_id = $1
              AND measured_at_utc > NOW() - interval '90 days'
              AND rmssd_ms IS NOT NULL
            ORDER BY measured_at_utc::date
        """,
        "sleep_score": """
            SELECT calendar_date AS date, score_overall::float AS value
            FROM health_sleep
            WHERE user_id = $1
              AND calendar_date > CURRENT_DATE - 90
              AND score_overall IS NOT NULL
            ORDER BY calendar_date
        """,
        "spo2": """
            SELECT measured_at_utc::date AS date, AVG(spo2_pct) AS value
            FROM health_spo2
            WHERE user_id = $1
              AND measured_at_utc > NOW() - interval '90 days'
            GROUP BY 1
            ORDER BY 1
        """,
    }

    all_anomalies: dict[str, dict] = {}  # date -> anomaly entry

    async with pool.acquire() as conn:
        for metric, query in METRIC_QUERIES.items():
            try:
                rows = await conn.fetch(query, user_id)
            except Exception as e:
                logger.warning("anomalies_v2: query failed for metric %s: %s", metric, e)
                continue

            if len(rows) < _MIN_DATA_POINTS:
                continue

            daily: dict = {}
            for row in rows:
                daily[str(row["date"])] = float(row["value"])

            dates = sorted(daily.keys())
            values = np.array([daily[d] for d in dates], dtype=np.float64)

            for i in range(len(values)):
                window_start = max(0, i - _WINDOW_DAYS)
                window = values[window_start:i]
                if len(window) < 7:
                    continue

                baseline_mean = float(np.mean(window))
                baseline_std = float(np.std(window, ddof=1))
                if baseline_std == 0:
                    continue

                zscore = (values[i] - baseline_mean) / baseline_std
                abs_z = abs(zscore)

                # Low: |z| >= 2, Medium: |z| >= 2.5, High: |z| >= 3
                if abs_z < 2.0:
                    continue

                severity = "high" if abs_z >= 3.0 else "medium" if abs_z >= 2.5 else "low"
                dt = dates[i]

                if dt not in all_anomalies:
                    all_anomalies[dt] = {
                        "date": dt,
                        "severity": severity,
                        "affected_metrics": [],
                        "details": [],
                    }
                else:
                    # Escalate severity if worse
                    sev_rank = {"low": 0, "medium": 1, "high": 2}
                    if sev_rank[severity] > sev_rank[all_anomalies[dt]["severity"]]:
                        all_anomalies[dt]["severity"] = severity

                all_anomalies[dt]["affected_metrics"].append(metric)
                all_anomalies[dt]["details"].append({
                    "metric": metric,
                    "value": round(values[i], 2),
                    "zscore": round(float(zscore), 3),
                    "baseline_mean": round(baseline_mean, 2),
                    "baseline_std": round(baseline_std, 2),
                })

    result = sorted(all_anomalies.values(), key=lambda x: x["date"])

    # Store in analytics_cache
    async with pool.acquire() as conn:
        await conn.execute(
            """
            INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
            VALUES ($1, 'anomalies_v2', $2::jsonb, NOW())
            ON CONFLICT (user_id, cache_key)
            DO UPDATE SET data = $2::jsonb, computed_at = NOW()
            """,
            user_id, json.dumps(result),
        )

    return result
