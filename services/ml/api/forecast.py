"""Forecast endpoints using TSFM models (TSPulse, TTM, Chronos-2).

Supports forecasting and anomaly detection on time-series metrics
like CTL, ATL, HRV, resting HR from pmc_daily and health tables.
"""
import asyncio
import logging
from datetime import date, timedelta
from typing import Optional
from uuid import UUID

import numpy as np
import torch
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)

router = APIRouter(prefix="/ml/forecast", tags=["forecast"])

# Metrics and their DB sources
METRIC_SOURCES = {
    "ctl": ("pmc_daily", "ctl", "date"),
    "atl": ("pmc_daily", "atl", "date"),
    "tsb": ("pmc_daily", "tsb", "date"),
    "tss": ("pmc_daily", "tss", "date"),
    "hrv_rmssd": ("health_snapshots", "rmssd_ms", "measured_at_utc"),
    "resting_hr": ("pmc_daily", "resting_hr", "date"),
}

MIN_POINTS_TTM = 512
MIN_POINTS_CHRONOS = 60
MIN_POINTS_TSPULSE = 512


class ForecastRequest(BaseModel):
    user_id: UUID
    metric: str = "ctl"
    horizon_days: int = Field(default=14, ge=1, le=96)
    model: str = "chronos"  # ttm, chronos


class AnomalyRequest(BaseModel):
    user_id: UUID
    metric: str = "ctl"
    lookback_days: int = Field(default=730, ge=90)


class ForecastResponse(BaseModel):
    metric: str
    model: str
    history_days: int
    forecast: list[dict]
    trend: Optional[str] = None


class AnomalyResponse(BaseModel):
    metric: str
    model: str
    anomalies: list[dict]
    total_points: int


async def _fetch_metric(user_id: UUID, metric: str, limit_days: int) -> tuple[list[str], np.ndarray]:
    """Fetch time-series data for a metric from the database."""
    from main import app
    pool = app.state.db_pool

    source = METRIC_SOURCES.get(metric)
    if not source:
        raise HTTPException(status_code=400, detail=f"Unknown metric: {metric}. Available: {list(METRIC_SOURCES.keys())}")

    table, column, date_col = source

    if table == "health_snapshots":
        query = f"""
            SELECT {date_col}::date::text as d, AVG({column}) as v
            FROM {table}
            WHERE user_id = $1 AND {date_col} >= CURRENT_DATE - $2::int
            GROUP BY {date_col}::date
            ORDER BY d
        """
    else:
        query = f"""
            SELECT {date_col}::text as d, {column} as v
            FROM {table}
            WHERE user_id = $1 AND {date_col} >= CURRENT_DATE - $2::int
              AND {column} IS NOT NULL
            ORDER BY {date_col}
        """

    rows = await pool.fetch(query, user_id, limit_days)
    if not rows:
        return [], np.array([])

    dates = [r["d"] for r in rows]
    values = np.array([float(r["v"]) for r in rows], dtype=np.float32)
    return dates, values


@router.post("/predict", response_model=ForecastResponse)
async def predict(req: ForecastRequest):
    """Run TSFM forecast on a time-series metric."""
    if req.model not in ("ttm", "chronos"):
        raise HTTPException(status_code=400, detail="Model must be 'ttm' or 'chronos'")

    min_points = MIN_POINTS_TTM if req.model == "ttm" else MIN_POINTS_CHRONOS
    lookback = max(min_points * 2, 1460)  # 4 years for good context

    dates, values = await _fetch_metric(req.user_id, req.metric, lookback)

    if len(values) < min_points:
        raise HTTPException(
            status_code=422,
            detail=f"Insufficient data: {len(values)} points, need {min_points}+ for {req.model}"
        )

    if req.model == "chronos":
        forecast_values = await _predict_chronos(values, req.horizon_days)
    else:
        forecast_values = await _predict_ttm(values, req.horizon_days)

    # Build forecast dates
    last_date = date.fromisoformat(dates[-1])
    forecast_list = []
    for i, val in enumerate(forecast_values):
        d = last_date + timedelta(days=i + 1)
        forecast_list.append({"date": d.isoformat(), "value": round(float(val), 2)})

    # Determine trend
    if len(forecast_values) >= 2:
        diff = forecast_values[-1] - forecast_values[0]
        if diff > values.std() * 0.5:
            trend = "steigend"
        elif diff < -values.std() * 0.5:
            trend = "fallend"
        else:
            trend = "stabil"
    else:
        trend = None

    return ForecastResponse(
        metric=req.metric,
        model=req.model,
        history_days=len(values),
        forecast=forecast_list,
        trend=trend,
    )


async def _predict_chronos(values: np.ndarray, horizon: int) -> np.ndarray:
    """Forecast using Chronos-2."""
    from models.tsfm import get_chronos
    pipeline = await get_chronos()
    if pipeline is None:
        raise HTTPException(status_code=501, detail="Chronos-2 model not available")

    def _run():
        context = torch.tensor(values, dtype=torch.float32)
        forecast = pipeline.predict(context, prediction_length=horizon)
        # forecast shape: (1, num_samples, horizon) — take median
        return np.median(forecast[0].numpy(), axis=0)

    return await asyncio.to_thread(_run)


async def _predict_ttm(values: np.ndarray, horizon: int) -> np.ndarray:
    """Forecast using TTM R2."""
    from models.tsfm import get_ttm
    model = await get_ttm()
    if model is None:
        raise HTTPException(status_code=501, detail="TTM model not available")

    # TTM needs exactly 512 input points, 1 channel
    if len(values) > 512:
        values = values[-512:]

    # Defensive: pad with last value if somehow < 512 after MIN_POINTS check
    if len(values) < 512:
        pad_count = 512 - len(values)
        values = np.concatenate([np.full(pad_count, values[-1], dtype=values.dtype), values])

    def _run():
        tensor = torch.tensor(values, dtype=torch.float32).reshape(1, 512, 1)
        with torch.no_grad():
            out = model(tensor)
        # TTM outputs 96 steps; truncate to requested horizon
        prediction = out.prediction_outputs.squeeze().numpy()
        return prediction[:min(horizon, len(prediction))]

    return await asyncio.to_thread(_run)


@router.post("/anomalies", response_model=AnomalyResponse)
async def detect_anomalies(req: AnomalyRequest):
    """Detect anomalies in a time-series metric using TSPulse R1."""
    from models.tsfm import get_tspulse
    model = await get_tspulse()
    if model is None:
        raise HTTPException(status_code=501, detail="TSPulse model not available")

    dates, values = await _fetch_metric(req.user_id, req.metric, req.lookback_days)

    if len(values) < MIN_POINTS_TSPULSE:
        raise HTTPException(
            status_code=422,
            detail=f"Insufficient data: {len(values)} points, need {MIN_POINTS_TSPULSE}+ for TSPulse"
        )

    # Process in sliding windows of 512
    window_size = 512
    anomaly_scores = np.zeros(len(values))
    window_count = np.zeros(len(values))

    for start in range(0, len(values) - window_size + 1, window_size // 2):
        end = start + window_size
        window = torch.tensor(
            values[start:end], dtype=torch.float32
        ).unsqueeze(0).unsqueeze(-1)  # (1, 512, 1)

        with torch.no_grad():
            out = model(window)

        # Reconstruction error as anomaly score
        reconstructed = out.reconstruction_outputs.squeeze().numpy()
        error = np.abs(values[start:end] - reconstructed)
        anomaly_scores[start:end] += error
        window_count[start:end] += 1

    # Average overlapping windows
    mask = window_count > 0
    anomaly_scores[mask] /= window_count[mask]

    # Flag anomalies: points with score > mean + 2*std
    threshold = anomaly_scores[mask].mean() + 2 * anomaly_scores[mask].std()
    anomalies = []
    for i in range(len(values)):
        if anomaly_scores[i] > threshold:
            anomalies.append({
                "date": dates[i],
                "value": round(float(values[i]), 2),
                "score": round(float(anomaly_scores[i]), 4),
                "threshold": round(float(threshold), 4),
            })

    return AnomalyResponse(
        metric=req.metric,
        model="tspulse-r1",
        anomalies=anomalies,
        total_points=len(values),
    )


@router.get("/models")
async def list_models():
    """List available TSFM models and their status."""
    from models.tsfm import available_models
    models = available_models()
    models["tspulse"]["min_points"] = MIN_POINTS_TSPULSE
    models["ttm"]["min_points"] = MIN_POINTS_TTM
    models["chronos"]["min_points"] = MIN_POINTS_CHRONOS
    return models
