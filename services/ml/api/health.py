"""Health and model status endpoints."""
from fastapi import APIRouter
import platform
import os

router = APIRouter(prefix="/ml", tags=["health"])


@router.get("/models")
async def list_models():
    """Aggregate model status: LLM availability + TSFM models."""
    try:
        from config import get_llm_role_statuses
        llm_roles = await get_llm_role_statuses()
    except Exception:
        llm_roles = {}

    tsfm_status = {}
    try:
        from models.tsfm import available_models
        tsfm_status = available_models()
    except Exception:
        pass

    return {
        "llm": {"roles": llm_roles},
        "tsfm": tsfm_status,
        "python": platform.python_version(),
        "mode": os.getenv("ML_MODE", "local"),
    }


def _detect_gpu() -> dict:
    try:
        import torch
        if torch.cuda.is_available():
            backend = "rocm" if getattr(torch.version, "hip", None) else "cuda"
            return {"type": backend, "name": torch.cuda.get_device_name(0)}
    except ImportError:
        pass
    if os.path.exists("/dev/kfd"):
        return {"type": "cpu", "name": "GPU device present, but no active ML backend detected"}
    return {"type": "cpu", "name": "No GPU detected"}
