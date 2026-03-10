"""SigLIP2 CLIP model for semantic photo search.

Lazy-loaded to avoid startup overhead (~1.6 GB, CPU-only).
Pattern follows services/ml/models/tsfm.py.
"""
import asyncio
import io
import logging

import torch
import torch.nn.functional as F
from PIL import Image

logger = logging.getLogger(__name__)

CLIP_MODEL_NAME = "google/siglip2-so400m-patch14-384"
CLIP_EMBEDDING_DIM = 1152

_model = None
_processor = None
_clip_lock = asyncio.Lock()


async def get_clip():
    """Lazy-load SigLIP2 (~1.6 GB, CPU-only).

    Returns (model, processor) tuple. Thread-safe via asyncio.Lock.
    """
    global _model, _processor
    async with _clip_lock:
        if _model is None:
            logger.info("Loading SigLIP2 model: %s", CLIP_MODEL_NAME)
            from transformers import AutoModel, AutoProcessor
            _processor = AutoProcessor.from_pretrained(CLIP_MODEL_NAME)
            _model = AutoModel.from_pretrained(CLIP_MODEL_NAME)
            _model.eval()
            logger.info("SigLIP2 loaded (dim=%d)", CLIP_EMBEDDING_DIM)
    return _model, _processor


async def embed_image(image_bytes: bytes) -> list[float]:
    """Compute 1152-dim L2-normalized embedding for an image.

    Scales down images >4096px before processing (SigLIP2 resizes
    internally to 384px, but very large images waste RAM on load).
    """
    model, processor = await get_clip()

    image = Image.open(io.BytesIO(image_bytes)).convert("RGB")

    MAX_DIM = 4096
    if max(image.size) > MAX_DIM:
        image.thumbnail((MAX_DIM, MAX_DIM), Image.LANCZOS)

    inputs = processor(images=image, return_tensors="pt")

    with torch.no_grad():
        # Siglip2Model.get_image_features() returns projected + pooled image embedding.
        # Confirmed via transformers docs: AutoModel resolves to Siglip2Model which
        # exposes get_image_features(**pixel_values) -> FloatTensor[batch, 1152].
        outputs = model.get_image_features(**inputs)

    vec = F.normalize(outputs, p=2, dim=-1)
    return vec[0].cpu().tolist()


async def embed_text(query: str) -> list[float]:
    """Compute 1152-dim L2-normalized embedding for a text query.

    Supports German and English (SigLIP2 multilingual).
    SigLIP2 was trained with padding="max_length" (64 tokens).
    """
    model, processor = await get_clip()

    # padding="max_length" matches SigLIP2 training-time normalization.
    inputs = processor(
        text=[query],
        return_tensors="pt",
        padding="max_length",
        truncation=True,
    )

    with torch.no_grad():
        # Siglip2Model.get_text_features() returns projected + pooled text embedding.
        # Confirmed via transformers docs: returns FloatTensor[batch, 1152].
        outputs = model.get_text_features(**inputs)

    vec = F.normalize(outputs, p=2, dim=-1)
    return vec[0].cpu().tolist()


def available_models() -> dict:
    """Return load status of the CLIP model (no eager loading)."""
    return {
        "siglip2": {
            "loaded": _model is not None,
            "size_mb": 1600,
            "purpose": "photo_search",
            "dim": CLIP_EMBEDDING_DIM,
        }
    }
