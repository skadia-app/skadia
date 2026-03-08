"""Extended HRV analysis using NeuroKit2.

Computes ~15 curated HRV metrics from RR intervals, plus full 124-metric JSONB blob.
Writes results to user_daily_features table.
"""
import logging
from datetime import date
from uuid import UUID

import neurokit2 as nk
import numpy as np

logger = logging.getLogger(__name__)

# Curated metrics to store as individual features (for fast queries)
CURATED_METRICS = [
    "HRV_RMSSD", "HRV_SDNN", "HRV_pNN50", "HRV_MeanNN",
    "HRV_LF", "HRV_HF", "HRV_LFHF",
    "HRV_SD1", "HRV_SD2", "HRV_SD1SD2",
    "HRV_SampEn", "HRV_ApEn",
    "HRV_DFA_alpha1", "HRV_DFA_alpha2",
    "HRV_CSI",  # Cardiac Stress Index
]


async def compute_hrv_extended(
    pool,
    user_id: UUID,
    activity_id: UUID,
    activity_date: date,
) -> dict:
    """Compute extended HRV metrics from RR intervals."""
    rr_rows = await pool.fetch(
        """
        SELECT rr_ms FROM activity_rr_intervals
        WHERE activity_id = $1
        ORDER BY seq
        """,
        activity_id,
    )

    if not rr_rows or len(rr_rows) < 30:
        logger.info("Skipping HRV: too few RR intervals (%d) for activity %s",
                     len(rr_rows) if rr_rows else 0, activity_id)
        return {}

    rr_ms = np.array([r["rr_ms"] for r in rr_rows], dtype=np.float64)

    # Filter artifacts (RR < 300ms or > 2000ms)
    mask = (rr_ms >= 300) & (rr_ms <= 2000)
    rr_clean = rr_ms[mask]

    if len(rr_clean) < 30:
        logger.info("Skipping HRV: too few clean RR intervals (%d)", len(rr_clean))
        return {}

    try:
        peaks = np.cumsum(rr_clean).astype(int)  # simulated peak positions in ms
        sampling_rate = 1000  # ms-basierte Peaks
        hrv_time = nk.hrv_time(peaks, sampling_rate=sampling_rate, show=False)
        hrv_freq = nk.hrv_frequency(peaks, sampling_rate=sampling_rate, show=False)
        hrv_nonlinear = nk.hrv_nonlinear(peaks, sampling_rate=sampling_rate, show=False)
        import pandas as pd
        hrv_all = pd.concat([hrv_time, hrv_freq, hrv_nonlinear], axis=1)
        hrv_dict = hrv_all.iloc[0].to_dict()
    except Exception as e:
        logger.warning("NeuroKit2 HRV failed for activity %s: %s", activity_id, e)
        return {}

    # Clean NaN/Inf values
    full_blob = {}
    for k, v in hrv_dict.items():
        if isinstance(v, (int, float)) and np.isfinite(v):
            full_blob[k] = round(float(v), 6)

    # Write curated metrics as individual features
    curated = {}
    for key in CURATED_METRICS:
        if key in full_blob:
            curated[key] = full_blob[key]

    # Store in user_daily_features
    import json
    async with pool.acquire() as conn:
        # Individual curated metrics
        for key, value in curated.items():
            await conn.execute(
                """
                INSERT INTO user_daily_features (user_id, date, activity_id, feature_key, value_num, computed_at)
                VALUES ($1, $2, $3, $4, $5, NOW())
                ON CONFLICT (user_id, date, feature_key, activity_id)
                DO UPDATE SET value_num = $5, computed_at = NOW()
                """,
                user_id, activity_date, activity_id, f"hrv.{key}", value,
            )

        # Full JSONB blob
        await conn.execute(
            """
            INSERT INTO user_daily_features (user_id, date, activity_id, feature_key, value_json, computed_at)
            VALUES ($1, $2, $3, 'hrv.full_blob', $4::jsonb, NOW())
            ON CONFLICT (user_id, date, feature_key, activity_id)
            DO UPDATE SET value_json = $4::jsonb, computed_at = NOW()
            """,
            user_id, activity_date, activity_id, json.dumps(full_blob),
        )

    return curated
