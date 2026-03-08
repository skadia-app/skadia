"""Forecast endpoints — Stufe 2/3 (requires sufficient data).

Returns 501 with explanation if insufficient data for TSFM.
"""
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel
from uuid import UUID

router = APIRouter(prefix="/ml/forecast", tags=["forecast"])


class ForecastRequest(BaseModel):
    user_id: UUID
    metric: str = "hrv_rmssd"
    horizon_days: int = 7
    model: str = "ttm"  # ttm, tspulse, chronos


@router.post("/predict")
async def predict(req: ForecastRequest):
    """Run TSFM forecast. Returns 501 if model or data unavailable."""
    from models.tsfm import get_ttm, get_tspulse, get_chronos

    model_fn = {"ttm": get_ttm, "tspulse": get_tspulse, "chronos": get_chronos}.get(req.model)
    if not model_fn:
        raise HTTPException(status_code=400, detail=f"Unknown model: {req.model}")

    model_instance = model_fn()
    if model_instance is None:
        raise HTTPException(
            status_code=501,
            detail=f"Model {req.model} not available — will be downloaded on first use"
        )

    # TODO: Actual prediction logic once monitoring FIT import provides sufficient data
    return {
        "status": "stub",
        "message": f"TSFM {req.model} prediction not yet implemented — awaiting sufficient data",
        "model_loaded": True,
    }


@router.get("/models")
async def list_models():
    """List available TSFM models and their status."""
    from models.tsfm import available_models
    return available_models()
