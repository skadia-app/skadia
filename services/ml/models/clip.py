"""SigLIP2 CLIP model for semantic photo search.

Lazy-loaded to avoid startup overhead (~1.6 GB, CPU-only).
Pattern follows services/ml/models/tsfm.py.

Supported formats:
- Images: JPEG, PNG, WebP, HEIC/HEIF (via pillow-heif), JXL (via pillow-jxl-plugin)
- Videos: MP4, MOV, AVI, MKV, WebM (frames extracted every 10s via ffmpeg, mean-pooled)
"""
import asyncio
import glob
import io
import logging
import os
import subprocess
import tempfile

import numpy as np
import torch
import torch.nn.functional as F
from PIL import Image

# Register HEIC/HEIF support (pillow-heif)
try:
    import pillow_heif
    pillow_heif.register_heif_opener()
    _heic_available = True
except ImportError:
    _heic_available = False

# Register JXL support (pillow-jxl-plugin)
try:
    import pillow_jxl  # noqa: F401 — side-effect: registers JXL opener
    _jxl_available = True
except ImportError:
    _jxl_available = False

_VIDEO_EXTENSIONS = {".mp4", ".mov", ".avi", ".mkv", ".webm", ".m4v"}
_VIDEO_MIMETYPES = {"video/mp4", "video/quicktime", "video/x-msvideo", "video/x-matroska", "video/webm"}

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


def is_video(s3_key: str, content_type: str = "") -> bool:
    """Return True if the file is a video based on extension or MIME type."""
    ext = os.path.splitext(s3_key.lower())[1]
    return ext in _VIDEO_EXTENSIONS or content_type.lower() in _VIDEO_MIMETYPES


async def embed_video_bytes(video_bytes: bytes) -> list[float]:
    """Extract frames every 10s via ffmpeg, embed via SigLIP2, return mean-pooled embedding.

    Strategy:
    - Use ffmpeg subprocess to extract 1 frame per 10 seconds as JPEG
    - Embed each frame independently with SigLIP2
    - Mean-pool all frame embeddings and L2-normalize the result
    - Capped at 60 frames (= 10 minutes of video)

    Raises ValueError if no frames can be extracted (corrupt/unsupported video).
    """
    with tempfile.TemporaryDirectory() as tmpdir:
        video_path = os.path.join(tmpdir, "video.bin")
        with open(video_path, "wb") as f:
            f.write(video_bytes)

        frames_pattern = os.path.join(tmpdir, "frame_%04d.jpg")
        result = subprocess.run(
            [
                "ffmpeg", "-i", video_path,
                "-vf", "fps=1/10",   # 1 frame every 10 seconds
                "-q:v", "3",         # JPEG quality (1=best, 31=worst)
                "-frames:v", "60",   # cap at 60 frames
                frames_pattern,
                "-hide_banner", "-loglevel", "error",
            ],
            capture_output=True,
        )
        if result.returncode != 0:
            raise ValueError(f"ffmpeg extraction failed: {result.stderr.decode()[:200]}")

        frame_files = sorted(glob.glob(os.path.join(tmpdir, "frame_*.jpg")))
        if not frame_files:
            raise ValueError("ffmpeg produced no frames")

        logger.info("embed_video: %d frames extracted, embedding …", len(frame_files))

        model, processor = await get_clip()
        embeddings = []

        for fpath in frame_files:
            with Image.open(fpath) as img:
                img = img.convert("RGB")
                if max(img.size) > 4096:
                    img.thumbnail((4096, 4096), Image.LANCZOS)
                inputs = processor(images=img, return_tensors="pt")

            with torch.no_grad():
                output = model.get_image_features(**inputs)
            vec = F.normalize(output, p=2, dim=-1)
            embeddings.append(vec[0].cpu().numpy())

    # Mean-pool across all frames, then L2-normalize the result
    mean_vec = np.mean(embeddings, axis=0)
    mean_tensor = torch.from_numpy(mean_vec).unsqueeze(0).float()
    normalized = F.normalize(mean_tensor, p=2, dim=-1)
    logger.info("embed_video: embedding complete (%d frames)", len(embeddings))
    return normalized[0].tolist()


def available_models() -> dict:
    """Return load status of the CLIP model (no eager loading)."""
    return {
        "siglip2": {
            "loaded": _model is not None,
            "size_mb": 1600,
            "purpose": "photo_search",
            "dim": CLIP_EMBEDDING_DIM,
            "formats": {
                "heic": _heic_available,
                "jxl": _jxl_available,
                "video": True,  # via ffmpeg subprocess
            },
        }
    }
