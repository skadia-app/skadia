"""ML Service configuration.

Reads from environment variables.
"""
import logging
import os

import httpx

_config_logger = logging.getLogger(__name__)

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
    "http://api:8080/api/v1/internal/ai-broker",
).rstrip("/")
ML_LLM_BROKER_TOKEN = os.getenv("ML_LLM_BROKER_TOKEN", os.getenv("ML_CALLBACK_SECRET", ""))
ML_DOCUMENT_ROLE = os.getenv("ML_DOCUMENT_ROLE", "document_extract")
ML_MARKDOWN_ROLE = os.getenv("ML_MARKDOWN_ROLE", "markdown_extract")
ML_VISION_ROLE = os.getenv("ML_VISION_ROLE", "vision")
ML_LLM_ROLES = (ML_DOCUMENT_ROLE, ML_MARKDOWN_ROLE, ML_VISION_ROLE)

NATS_URL = os.getenv("NATS_URL", "nats://nats:4222")
HF_HOME = os.getenv("HF_HOME", "/data/models/huggingface")

ML_CALLBACK_URL = os.getenv("ML_CALLBACK_URL", "")
ML_CALLBACK_SECRET = os.getenv("ML_CALLBACK_SECRET", "")


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


def _role_status_fallback(status: str) -> dict[str, str]:
    return {role: status for role in ML_LLM_ROLES if role}


def _normalize_role_statuses(payload: dict) -> dict[str, str]:
    roles = payload.get("roles", {})
    statuses: dict[str, str] = {}
    for role in ML_LLM_ROLES:
        if not role:
            continue
        value = roles.get(role, "not_configured")
        if isinstance(value, dict):
            value = value.get("status", "not_configured")
        if value not in {"configured", "not_configured", "unavailable"}:
            value = "unavailable"
        statuses[role] = value
    return statuses


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
