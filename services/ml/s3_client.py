"""Shared S3 client configuration for ML workers."""

from __future__ import annotations

import os
from pathlib import Path


def _env_or_file(key: str, fallback_key: str | None = None, default: str = "") -> str:
    file_value = os.getenv(f"{key}_FILE", "").strip()
    if file_value:
        path = Path(file_value)
        if not path.is_absolute():
            raise ValueError(f"{key}_FILE must be an absolute path")
        value = path.read_text(encoding="utf-8").strip()
        if not value:
            raise ValueError(f"{key}_FILE points to an empty file")
        return value
    value = os.getenv(key, "").strip()
    if value:
        return value
    if fallback_key:
        return _env_or_file(fallback_key, None, default)
    return default


def _bool_env(key: str) -> bool:
    return os.getenv(key, "").strip().lower() in {"1", "true", "yes", "on"}


def s3_client_kwargs() -> dict:
    endpoint = _env_or_file("S3_ENDPOINT", "GARAGE_ENDPOINT")
    storage_mode = os.getenv("SKADIA_STORAGE_MODE", "").strip()
    provider = os.getenv("SKADIA_S3_PROVIDER", "").strip()
    local_garage = storage_mode == "s3-local-garage" or (not storage_mode and provider == "local-garage")
    if not endpoint and local_garage:
        endpoint = "http://garage:3900"
    force_path_style = _bool_env("S3_FORCE_PATH_STYLE") or local_garage
    region = os.getenv("S3_REGION", "").strip()
    if not region and local_garage:
        region = "us-east-1"
    if storage_mode == "s3-external":
        if not endpoint:
            raise ValueError("S3_ENDPOINT is required when SKADIA_STORAGE_MODE=s3-external")
        if not region:
            raise ValueError("S3_REGION is required when SKADIA_STORAGE_MODE=s3-external")

    kwargs = {
        "aws_access_key_id": _env_or_file("S3_ACCESS_KEY", "GARAGE_ACCESS_KEY"),
        "aws_secret_access_key": _env_or_file("S3_SECRET_KEY", "GARAGE_SECRET_KEY"),
    }
    if region:
        kwargs["region_name"] = region
    if endpoint:
        kwargs["endpoint_url"] = endpoint
    if force_path_style:
        from botocore.config import Config

        kwargs["config"] = Config(s3={"addressing_style": "path"})
    return kwargs
