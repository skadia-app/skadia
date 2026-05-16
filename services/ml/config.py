"""ML Service configuration.

Reads from environment variables.
"""
import asyncio
import logging
import os
from pathlib import Path

import httpx

from s3_client import s3_client_kwargs

_config_logger = logging.getLogger(__name__)


def _env_or_file(key: str, default: str = "") -> str:
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
    return default


ML_MODE = os.getenv("ML_MODE", "local")

DATABASE_URL = os.getenv("DATABASE_URL", "postgresql://skadia:skadia@postgres:5432/skadia")
_readonly_url_raw = os.getenv("READONLY_DATABASE_URL")
if _readonly_url_raw:
    READONLY_DATABASE_URL = _readonly_url_raw
else:
    _config_logger.warning(
        "READONLY_DATABASE_URL not set — falling back to DATABASE_URL for readonly pool. "
        "Set READONLY_DATABASE_URL to use the skadia_readonly role."
    )
    READONLY_DATABASE_URL = DATABASE_URL

ML_LLM_BROKER_URL = os.getenv(
    "ML_LLM_BROKER_URL",
    "http://api:8080/internal/ai-broker",
).rstrip("/")
_ML_CALLBACK_SECRET = _env_or_file("ML_CALLBACK_SECRET", "")
ML_LLM_BROKER_TOKEN = _env_or_file("ML_LLM_BROKER_TOKEN", _ML_CALLBACK_SECRET)
ML_DOCUMENT_ROLE = os.getenv("ML_DOCUMENT_ROLE", "document_extract")
ML_MARKDOWN_ROLE = os.getenv("ML_MARKDOWN_ROLE", "markdown_extract")
ML_VISION_ROLE = os.getenv("ML_VISION_ROLE", "vision")
ML_LLM_ROLES = (ML_DOCUMENT_ROLE, ML_MARKDOWN_ROLE, ML_VISION_ROLE)

NATS_URL = os.getenv("NATS_URL", "nats://nats:4222")
HF_HOME = os.getenv("HF_HOME", "/data/models/huggingface")

ML_CALLBACK_URL = os.getenv("ML_CALLBACK_URL", "")
ML_CALLBACK_SECRET = _ML_CALLBACK_SECRET
S3_READ_PROBE_TIMEOUT_SECONDS = float(os.getenv("ML_S3_READ_PROBE_TIMEOUT_SECONDS", "1.0"))


class LLMRoleNotConfigured(Exception):
    """Raised when the API broker reports that an LLM role is not configured."""

    def __init__(self, role: str, status: str = "not_configured"):
        super().__init__(f"LLM role {role!r} is {status}")
        self.role = role
        self.status = status


def _broker_headers() -> dict[str, str]:
    return {
        "Content-Type": "application/json",
        "X-ML-Service-Token": ML_LLM_BROKER_TOKEN,
    }


def configured_llm_roles() -> tuple[str, ...]:
    roles: list[str] = []
    for role in ML_LLM_ROLES:
        role = (role or "").strip()
        if role and role not in roles:
            roles.append(role)
    return tuple(roles)


def _role_status_fallback(status: str) -> dict[str, str]:
    return {role: status for role in configured_llm_roles()}


def _normalize_role_statuses(payload: dict) -> dict[str, str]:
    roles = payload.get("roles", {})
    statuses: dict[str, str] = {}
    for role in configured_llm_roles():
        value = roles.get(role, "not_configured")
        if isinstance(value, dict):
            value = value.get("status", "not_configured")
        if value not in {"configured", "not_configured", "unavailable"}:
            value = "unavailable"
        statuses[role] = value
    return statuses


def llm_broker_readiness_from_statuses(statuses: dict[str, str]) -> dict:
    roles = configured_llm_roles()
    if not roles:
        return {"status": "disabled", "roles": {}}
    normalized = {}
    for role in roles:
        value = statuses.get(role, "not_configured")
        if value not in {"configured", "not_configured", "unavailable"}:
            value = "unavailable"
        normalized[role] = value
    if normalized and not ML_LLM_BROKER_TOKEN:
        return {"status": "degraded", "roles": normalized, "error": "broker_token_missing"}
    status = "ready" if all(value == "configured" for value in normalized.values()) else "degraded"
    return {"status": status, "roles": normalized}


def _local_shadowfs_base_path() -> str:
    return (
        os.getenv("ML_SHADOWFS_BASE_PATH")
        or os.getenv("LIBRARY_BASE_PATH")
        or os.getenv("SHADOW_FS_BASE_PATH")
        or ""
    )


def get_local_shadowfs_readiness() -> dict:
    base = _local_shadowfs_base_path()
    if not base:
        return {"status": "disabled"}
    path = Path(base)
    if not path.exists():
        return {"status": "degraded", "base_path": base, "error": "missing"}
    if not path.is_dir():
        return {"status": "degraded", "base_path": base, "error": "not_directory"}
    if not os.access(path, os.R_OK):
        return {"status": "degraded", "base_path": base, "error": "not_readable"}
    return {"status": "ready", "base_path": str(path)}


def get_s3_storage_readiness() -> dict:
    storage_mode = os.getenv("SKADIA_STORAGE_MODE", "").strip()
    provider = os.getenv("SKADIA_S3_PROVIDER", "").strip()
    has_s3_config = any(
        os.getenv(key, "").strip()
        for key in (
            "S3_ENDPOINT",
            "GARAGE_ENDPOINT",
            "S3_ACCESS_KEY",
            "GARAGE_ACCESS_KEY",
            "S3_SECRET_KEY",
            "GARAGE_SECRET_KEY",
        )
    )
    if not storage_mode.startswith("s3") and provider != "local-garage" and not has_s3_config:
        return {"status": "disabled"}
    try:
        kwargs = s3_client_kwargs()
    except Exception as e:
        return {"status": "degraded", "error": str(e)}
    if not kwargs.get("aws_access_key_id") or not kwargs.get("aws_secret_access_key"):
        return {"status": "degraded", "error": "credentials_missing"}
    bucket = os.getenv("S3_BUCKET", "skadia-originals").strip() or "skadia-originals"
    try:
        _probe_s3_bucket_readable(kwargs, bucket)
    except Exception as e:
        return {"status": "degraded", "error": "s3_read_probe_failed", "detail": str(e)}
    return {"status": "ready"}


def _probe_s3_bucket_readable(kwargs: dict, bucket: str) -> None:
    import boto3
    from botocore.config import Config

    probe_kwargs = dict(kwargs)
    probe_config = Config(
        connect_timeout=S3_READ_PROBE_TIMEOUT_SECONDS,
        read_timeout=S3_READ_PROBE_TIMEOUT_SECONDS,
        retries={"max_attempts": 1, "mode": "standard"},
    )
    existing_config = probe_kwargs.get("config")
    if existing_config is not None and hasattr(existing_config, "merge"):
        probe_kwargs["config"] = existing_config.merge(probe_config)
    else:
        probe_kwargs["config"] = probe_config
    client = boto3.client("s3", **probe_kwargs)
    client.list_objects_v2(Bucket=bucket, MaxKeys=1)


def storage_readiness_ready(storage_readiness: dict) -> bool:
    statuses = [
        entry.get("status")
        for entry in storage_readiness.values()
        if isinstance(entry, dict)
    ]
    return "degraded" not in statuses and "ready" in statuses


def storage_readiness_degraded(storage_readiness: dict) -> bool:
    return any(
        entry.get("status") == "degraded"
        for entry in storage_readiness.values()
        if isinstance(entry, dict)
    )


def get_storage_readiness() -> dict:
    return {
        "local": get_local_shadowfs_readiness(),
        "s3": get_s3_storage_readiness(),
    }


async def get_storage_readiness_async() -> dict:
    return {
        "local": get_local_shadowfs_readiness(),
        "s3": await asyncio.to_thread(get_s3_storage_readiness),
    }


def get_llm_role_statuses_sync() -> dict[str, str]:
    """Return broker role status without exposing provider credentials."""
    if not ML_LLM_BROKER_URL or not ML_LLM_BROKER_TOKEN:
        return _role_status_fallback("not_configured")
    try:
        with httpx.Client(timeout=2.0) as client:
            response = client.get(f"{ML_LLM_BROKER_URL}/roles", headers=_broker_headers())
            response.raise_for_status()
            return _normalize_role_statuses(response.json())
    except Exception as e:
        _config_logger.warning("LLM broker status unavailable: %s", e)
        return _role_status_fallback("unavailable")


async def get_llm_role_statuses() -> dict[str, str]:
    """Async variant for FastAPI health handlers."""
    if not ML_LLM_BROKER_URL or not ML_LLM_BROKER_TOKEN:
        return _role_status_fallback("not_configured")
    try:
        async with httpx.AsyncClient(timeout=2.0) as client:
            response = await client.get(f"{ML_LLM_BROKER_URL}/roles", headers=_broker_headers())
            response.raise_for_status()
            return _normalize_role_statuses(response.json())
    except Exception as e:
        _config_logger.warning("LLM broker status unavailable: %s", e)
        return _role_status_fallback("unavailable")


async def get_llm_broker_readiness() -> dict:
    """Return readiness for configured brokered LLM roles.

    Empty role env vars explicitly disable brokered LLM/OCR readiness. If at
    least one role is configured, every configured role must be broker-ready.
    """
    return llm_broker_readiness_from_statuses(await get_llm_role_statuses())


async def call_llm_broker(
    role: str,
    task_type: str,
    messages: list,
    timeout: float = 120.0,
) -> str:
    """Call the API-owned LLM broker for a configured role."""
    if not ML_LLM_BROKER_URL or not ML_LLM_BROKER_TOKEN or not role:
        raise LLMRoleNotConfigured(role or "unknown")
    payload = {
        "role": role,
        "task_type": task_type,
        "messages": messages,
    }
    async with httpx.AsyncClient(timeout=timeout) as client:
        response = await client.post(
            f"{ML_LLM_BROKER_URL}/chat",
            headers=_broker_headers(),
            json=payload,
        )
    if response.status_code == 424:
        body = response.json()
        raise LLMRoleNotConfigured(body.get("role", role), body.get("status", "not_configured"))
    response.raise_for_status()
    body = response.json()
    if body.get("status") != "ok":
        raise RuntimeError(f"LLM broker returned {body.get('status', 'unknown')}")
    return (body.get("text") or "").strip()
