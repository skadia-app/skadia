"""ML Service configuration.

Reads from environment variables. Provides LLM client initialization.
"""
import logging
import os
from typing import Optional
from openai import AsyncOpenAI

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

LLM_BASE_URL = os.getenv("LLM_BASE_URL", "http://litellm:4000/v1")
LLM_API_KEY = os.getenv("LLM_API_KEY", "sk-dummy")

NATS_URL = os.getenv("NATS_URL", "nats://nats:4222")
HF_HOME = os.getenv("HF_HOME", "/data/models/huggingface")

ML_CALLBACK_URL = os.getenv("ML_CALLBACK_URL", "")
ML_CALLBACK_SECRET = os.getenv("ML_CALLBACK_SECRET", "")

_llm_client: Optional[AsyncOpenAI] = None


def get_llm_client() -> Optional[AsyncOpenAI]:
    """Get async OpenAI-compatible client pointing to LiteLLM."""
    global _llm_client
    if _llm_client is None:
        try:
            _llm_client = AsyncOpenAI(
                base_url=LLM_BASE_URL,
                api_key=LLM_API_KEY,
            )
        except Exception:
            return None
    return _llm_client
