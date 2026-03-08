"""Changepoint detection using ruptures PELT algorithm.

Detects significant changes in HR, HRV, CTL time series.
Results stored in analytics_cache as JSONB.
"""
import logging
from datetime import date, timedelta
from uuid import UUID
import json

import numpy as np
import ruptures as rpt

logger = logging.getLogger(__name__)

MIN_POINTS = 30


async def detect_changepoints(
    pool,
    user_id: UUID,
    metric: str = "hrv_rmssd",
    lookback_days: int = 180,
) -> list[dict]:
    """Detect changepoints in a user's time series."""
    since = date.today() - timedelta(days=lookback_days)

    if metric == "hrv_rmssd":
        rows = await pool.fetch(
            """
            SELECT date, value_num FROM user_daily_features
            WHERE user_id = $1 AND feature_key = 'hrv.HRV_RMSSD'
              AND date >= $2 AND value_num IS NOT NULL
            ORDER BY date
            """,
            user_id, since,
        )
    elif metric == "resting_hr":
        rows = await pool.fetch(
            """
            SELECT measured_at_utc::date AS date,
                   (extra_data->>'resting_hr')::float AS value_num
            FROM health_hrv_snapshots
            WHERE user_id = $1 AND measured_at_utc::date >= $2
              AND extra_data->>'resting_hr' IS NOT NULL
            ORDER BY measured_at_utc::date
            """,
            user_id, since,
        )
    elif metric == "ctl":
        rows = await pool.fetch(
            """
            SELECT date, ctl AS value_num FROM pmc_daily
            WHERE user_id = $1 AND date >= $2 AND ctl IS NOT NULL
            ORDER BY date
            """,
            user_id, since,
        )
    else:
        logger.warning("Unknown metric: %s", metric)
        return []

    if not rows or len(rows) < MIN_POINTS:
        return []

    dates = [r["date"] for r in rows]
    values = np.array([float(r["value_num"]) for r in rows])

    try:
        algo = rpt.Pelt(model="rbf", min_size=7).fit(values)
        penalty = 3 * np.log(len(values))
        breakpoints = algo.predict(pen=penalty)
    except Exception as e:
        logger.warning("ruptures PELT failed for %s/%s: %s", user_id, metric, e)
        return []

    changepoints = []
    for bp in breakpoints[:-1]:
        if bp >= len(dates):
            continue

        before = values[max(0, bp - 14):bp]
        after = values[bp:min(len(values), bp + 14)]

        if len(before) > 0 and len(after) > 0:
            magnitude = float(np.mean(after) - np.mean(before))
            changepoints.append({
                "date": dates[bp].isoformat(),
                "index": bp,
                "magnitude": round(magnitude, 2),
                "direction": "up" if magnitude > 0 else "down",
                "before_mean": round(float(np.mean(before)), 2),
                "after_mean": round(float(np.mean(after)), 2),
            })

    # Store in analytics_cache
    await pool.execute(
        """
        INSERT INTO analytics_cache (user_id, cache_key, data, computed_at)
        VALUES ($1, $2, $3::jsonb, NOW())
        ON CONFLICT (user_id, cache_key)
        DO UPDATE SET data = $3::jsonb, computed_at = NOW()
        """,
        user_id, f"changepoints.{metric}", json.dumps(changepoints),
    )

    return changepoints
