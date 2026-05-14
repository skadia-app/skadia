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
    kwargs = {
        "endpoint_url": _env_or_file("S3_ENDPOINT", "GARAGE_ENDPOINT", "http://garage:3900"),
        "aws_access_key_id": _env_or_file("S3_ACCESS_KEY", "GARAGE_ACCESS_KEY"),
        "aws_secret_access_key": _env_or_file("S3_SECRET_KEY", "GARAGE_SECRET_KEY"),
        "region_name": os.getenv("S3_REGION", "us-east-1"),
    }
    if _bool_env("S3_FORCE_PATH_STYLE"):
        from botocore.config import Config

        kwargs["config"] = Config(s3={"addressing_style": "path"})
    return kwargs
