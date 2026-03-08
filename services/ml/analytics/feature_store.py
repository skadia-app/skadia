"""Feature engineering using TSFEL.

Extracts ~65 statistical/spectral/fractal features from activity_streams.
Writes to user_daily_features with feature_key prefix 'tsfel.'.
"""
import logging
from datetime import date
from uuid import UUID
import json

import numpy as np
import tsfel

logger = logging.getLogger(__name__)

# TSFEL config: statistical + temporal features (no spectral for irregular data)
TSFEL_CONFIG = tsfel.get_features_by_domain("statistical")


async def compute_activity_features(
    pool,
    user_id: UUID,
    activity_id: UUID,
    activity_date: date,
) -> dict:
    """Extract TSFEL features from activity streams."""
    rows = await pool.fetch(
        """
        SELECT heart_rate, speed_ms, cadence, power_w, altitude_m
        FROM activity_streams
        WHERE activity_id = $1
        ORDER BY time
        """,
        activity_id,
    )

    if not rows or len(rows) < 50:
        return {}

    import pandas as pd

    # Build DataFrame from available columns
    data = {}
    for col in ["heart_rate", "speed_ms", "cadence", "power_w", "altitude_m"]:
        values = [r[col] for r in rows if r[col] is not None]
        if len(values) > len(rows) * 0.3:  # Need at least 30% coverage
            arr = np.array([r[col] if r[col] is not None else np.nan for r in rows], dtype=np.float64)
            # Forward fill NaN
            mask = np.isnan(arr)
            if mask.any():
                idx = np.where(~mask, np.arange(len(arr)), 0)
                np.maximum.accumulate(idx, out=idx)
                arr = arr[idx]
            data[col] = arr

    if not data:
        return {}

    df = pd.DataFrame(data)

    try:
        features = tsfel.time_series_features_extractor(
            TSFEL_CONFIG, df, fs=1, verbose=0
        )
        feature_dict = features.iloc[0].to_dict()
    except Exception as e:
        logger.warning("TSFEL failed for activity %s: %s", activity_id, e)
        return {}

    # Store features
    clean_features = {}
    async with pool.acquire() as conn:
        for key, value in feature_dict.items():
            if isinstance(value, (int, float)) and np.isfinite(value):
                clean_key = key.replace(" ", "_").lower()
                clean_features[clean_key] = round(float(value), 6)
                await conn.execute(
                    """
                    INSERT INTO user_daily_features (user_id, date, activity_id, feature_key, value_num, computed_at)
                    VALUES ($1, $2, $3, $4, $5, NOW())
                    ON CONFLICT (user_id, date, feature_key, activity_id)
                    DO UPDATE SET value_num = $5, computed_at = NOW()
                    """,
                    user_id, activity_date, activity_id, f"tsfel.{clean_key}", float(value),
                )

        # Also store as compact blob
        await conn.execute(
            """
            INSERT INTO user_daily_features (user_id, date, activity_id, feature_key, value_json, computed_at)
            VALUES ($1, $2, $3, 'tsfel.full_blob', $4::jsonb, NOW())
            ON CONFLICT (user_id, date, feature_key, activity_id)
            DO UPDATE SET value_json = $4::jsonb, computed_at = NOW()
            """,
            user_id, activity_date, activity_id, json.dumps(clean_features),
        )

    return clean_features
