"""FastAPI endpoints for feature engineering and analytics."""
from fastapi import APIRouter
from pydantic import BaseModel
from uuid import UUID
from datetime import date


router = APIRouter(prefix="/ml/features", tags=["features"])


class FeatureRequest(BaseModel):
    user_id: UUID
    activity_id: UUID
    activity_date: date


class ChangepointRequest(BaseModel):
    user_id: UUID
    metric: str = "hrv_rmssd"
    lookback_days: int = 180


@router.post("/hrv-extended")
async def hrv_extended(req: FeatureRequest):
    from analytics.hrv_extended import compute_hrv_extended
    from db import get_pool
    pool = await get_pool()
    result = await compute_hrv_extended(pool, req.user_id, req.activity_id, req.activity_date)
    return {"status": "ok", "features": result}


@router.post("/tsfel")
async def tsfel_features(req: FeatureRequest):
    from analytics.feature_store import compute_activity_features
    from db import get_pool
    pool = await get_pool()
    result = await compute_activity_features(pool, req.user_id, req.activity_id, req.activity_date)
    return {"status": "ok", "features_count": len(result)}


@router.post("/changepoints")
async def changepoints(req: ChangepointRequest):
    from analytics.changepoints import detect_changepoints
    from db import get_pool
    pool = await get_pool()
    result = await detect_changepoints(pool, req.user_id, req.metric, req.lookback_days)
    return {"status": "ok", "changepoints": result}
