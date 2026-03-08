"""Health and model status endpoints."""
from fastapi import APIRouter
import platform
import os

router = APIRouter(prefix="/ml", tags=["health"])


@router.get("/models")
async def list_models():
    """Aggregate model status: LLM availability + TSFM models."""
    llm_status = "unknown"
    try:
        from config import get_llm_client
        client = get_llm_client()
        llm_status = "available" if client else "unavailable"
    except Exception:
        llm_status = "unavailable"

    tsfm_status = {}
    try:
        from models.tsfm import available_models
        tsfm_status = available_models()
    except Exception:
        pass

    return {
        "llm": {"status": llm_status, "base_url": os.getenv("LLM_BASE_URL", "")},
        "tsfm": tsfm_status,
        "python": platform.python_version(),
        "mode": os.getenv("ML_MODE", "local"),
    }


def _detect_gpu() -> dict:
    try:
        import torch
        if torch.cuda.is_available():
            return {"type": "cuda", "name": torch.cuda.get_device_name(0)}
    except ImportError:
        pass
    if os.path.exists("/dev/kfd"):
        return {"type": "rocm", "name": "AMD GPU detected"}
    return {"type": "cpu", "name": "No GPU detected"}
