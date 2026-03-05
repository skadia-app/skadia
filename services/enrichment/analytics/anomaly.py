"""HRV anomaly detection via rolling z-score."""
import logging

import numpy as np

logger = logging.getLogger(__name__)

_MIN_DATA_POINTS = 14
_WINDOW_DAYS = 30
_ZSCORE_THRESHOLD = -2.0


async def detect_anomalies(pool, user_id: str) -> list:
    """Erkennt HRV-Anomalien über einen rollierenden 30-Tage-Baseline-Z-Score.

    Gibt eine Liste von {date, rmssd_ms, zscore, baseline_mean, baseline_std}
    für alle Tage zurück, an denen der Z-Score unter -2.0 fiel.

    Graceful degradation: liefert [] wenn weniger als 14 Datenpunkte vorhanden.
    """
    try:
        rows = await pool.fetch(
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
