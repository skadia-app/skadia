"""TSFM model loader with lazy initialization.

Models are downloaded on first use from HuggingFace.
Cached in /data/models/ volume.
"""
import asyncio
import logging
import os

logger = logging.getLogger(__name__)

MODEL_CACHE_DIR = os.getenv("HF_HOME", "/data/models/huggingface")

_tspulse = None
_tspulse_lock = asyncio.Lock()

_ttm = None
_ttm_lock = asyncio.Lock()

_chronos = None
_chronos_lock = asyncio.Lock()


async def get_tspulse():
    """Load TSPulse R1 anomaly detection model (~4MB)."""
    global _tspulse
    async with _tspulse_lock:
        if _tspulse is None:
            try:
                from tsfm_public.models.tspulse import TSPulseForReconstruction
                _tspulse = TSPulseForReconstruction.from_pretrained(
                    "ibm-granite/granite-timeseries-tspulse-r1",
                    revision="main",
                    cache_dir=MODEL_CACHE_DIR,
                )
                _tspulse.eval()
                logger.info("TSPulse R1 model loaded")
            except Exception as e:
                logger.error("Failed to load TSPulse: %s", e)
                return None
        return _tspulse


async def get_ttm():
    """Load TTM R2 forecasting model (~3MB)."""
    global _ttm
    async with _ttm_lock:
        if _ttm is None:
            try:
                from tsfm_public.models.tinytimemixer import TinyTimeMixerForPrediction
                _ttm = TinyTimeMixerForPrediction.from_pretrained(
                    "ibm-granite/granite-timeseries-ttm-r2",
                    prediction_filter_length=96,
                    cache_dir=MODEL_CACHE_DIR,
                )
                _ttm.eval()
                logger.info("TTM R2 model loaded")
            except Exception as e:
                logger.error("Failed to load TTM: %s", e)
                return None
        return _ttm


async def get_chronos():
    """Load Chronos-2 forecasting model (~480MB)."""
    global _chronos
    async with _chronos_lock:
        if _chronos is None:
            try:
                from chronos import ChronosPipeline
                _chronos = ChronosPipeline.from_pretrained(
                    "amazon/chronos-t5-small",
                    cache_dir=MODEL_CACHE_DIR,
                    device_map="cpu",
                )
                logger.info("Chronos-2 model loaded")
            except Exception as e:
                logger.error("Failed to load Chronos-2: %s", e)
                return None
        return _chronos


def available_models() -> dict:
    """Return status of available TSFM models (no eager loading)."""
    return {
        "tspulse": {"loaded": _tspulse is not None, "size_mb": 4, "purpose": "anomaly"},
        "ttm": {"loaded": _ttm is not None, "size_mb": 3, "purpose": "forecast"},
        "chronos": {"loaded": _chronos is not None, "size_mb": 480, "purpose": "forecast"},
    }
