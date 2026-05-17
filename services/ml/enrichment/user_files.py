import asyncio
import logging
import os
from pathlib import Path
from typing import Optional

import aioboto3

from s3_client import s3_client_kwargs

logger = logging.getLogger(__name__)


class RetryableStorageReadError(RuntimeError):
    """Raised for transient user-file storage reads that should be redelivered."""


def local_storage_base_path() -> str:
    return (
        os.getenv("ML_SHADOWFS_BASE_PATH")
        or os.getenv("LIBRARY_BASE_PATH")
        or os.getenv("SHADOW_FS_BASE_PATH")
        or ""
    )


def s3_bucket_for_file_type(file_type: str) -> str:
    default_bucket = os.getenv("S3_BUCKET", "skadia-originals")
    if file_type in {"activity", "health"}:
        return os.getenv("S3_BUCKET_ACTIVITY") or default_bucket
    if file_type in {"media", "document", "thumb", "preview"}:
        return os.getenv("S3_BUCKET_MEDIA") or default_bucket
    return default_bucket


def read_limited_file(path: Path, max_bytes: int) -> Optional[bytes]:
    with path.open("rb") as f:
        data = f.read(max_bytes + 1)
    if len(data) > max_bytes:
        logger.warning("local user file exceeds %d bytes: %s", max_bytes, path)
        return None
    return data


async def download_s3_user_file(s3_bucket: str, s3_key: str, *, max_bytes: int, label: str) -> Optional[bytes]:
    try:
        session = aioboto3.Session()
        async with session.client("s3", **s3_client_kwargs()) as s3:
            response = await s3.get_object(Bucket=s3_bucket, Key=s3_key)
            content_length = response.get("ContentLength")
            if content_length is not None and int(content_length) > max_bytes:
                logger.warning("S3 %s exceeds %d bytes: %s/%s", label, max_bytes, s3_bucket, s3_key)
                body = response.get("Body")
                if body and hasattr(body, "close"):
                    body.close()
                return None
            body = response["Body"]
            chunks: list[bytes] = []
            total = 0
            while True:
                chunk = await body.read(64 * 1024)
                if not chunk:
                    break
                total += len(chunk)
                if total > max_bytes:
                    logger.warning("S3 %s exceeds %d bytes: %s/%s", label, max_bytes, s3_bucket, s3_key)
                    if hasattr(body, "close"):
                        body.close()
                    return None
                chunks.append(chunk)
            if hasattr(body, "close"):
                body.close()
            return b"".join(chunks)
    except Exception as e:
        logger.warning("S3 %s download failed for %s/%s: %s", label, s3_bucket, s3_key, e)
        raise RetryableStorageReadError(f"S3 {label} download failed") from e
    return None


async def download_user_file(
    storage_backend: str,
    storage_path: str,
    file_type: str,
    *,
    size_bytes: int | None = None,
    max_bytes: int,
    label: str,
) -> Optional[bytes]:
    storage_backend = (storage_backend or "").strip()
    storage_path = (storage_path or "").strip()
    if not storage_backend or not storage_path:
        return None
    if size_bytes is not None and size_bytes > max_bytes:
        logger.warning("%s metadata exceeds %d bytes: %s", label, max_bytes, storage_path)
        return None
    if storage_backend == "s3":
        return await download_s3_user_file(
            s3_bucket_for_file_type(file_type),
            storage_path,
            max_bytes=max_bytes,
            label=label,
        )
    if storage_backend == "local":
        base = local_storage_base_path()
        if not base:
            logger.warning("local ShadowFS %s storage is not mounted in ML service", label)
            raise RetryableStorageReadError(f"local ShadowFS {label} storage is not mounted")
        base_path = Path(base).resolve()
        if not base_path.exists() or not base_path.is_dir():
            logger.warning("local ShadowFS %s base is unavailable: %s", label, base)
            raise RetryableStorageReadError(f"local ShadowFS {label} base is unavailable")
        full_path = (base_path / storage_path).resolve()
        try:
            full_path.relative_to(base_path)
        except ValueError:
            logger.warning("blocked %s storage path traversal: %s", label, storage_path)
            return None
        return await asyncio.to_thread(read_limited_file, full_path, max_bytes)
    logger.info("unsupported %s storage backend %s", label, storage_backend)
    return None
