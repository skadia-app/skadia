"""Model capability profiles for chat assistant.

Configures behavior based on LLM size (tiny/small/medium/large).
Each field can be overridden via UI settings (ml_user_settings).
"""
from __future__ import annotations

import re
from dataclasses import dataclass, field, asdict


@dataclass
class ModelProfile:
    """All configurable aspects of the chat assistant."""

    # Tools & Routing
    max_tools: int = 6
    use_router: bool = True
    max_tools_per_turn: int = 1
    tool_result_limit: int = 7

    # Schema & SQL
    schema_mode: str = "filtered"  # "filtered" | "full"
    enable_sql_agent: bool = True
    sql_complexity: str = "simple"  # "simple" | "moderate" | "advanced"
    max_sql_retries: int = 1

    # Prompt & Context
    few_shot_count: int = 3
    system_prompt_mode: str = "compact"  # "compact" | "standard" | "rich"
    max_context_tokens: int = 4000
    history_messages: int = 10

    # Reasoning & Model Behavior
    reasoning_mode: str = "guided"  # "guided" | "free" | "react"
    temperature: float = 0.3
    top_p: float = 0.8
    top_k: int = 20
    presence_penalty: float = 1.5
    max_response_tokens: int = 1000

    def to_dict(self) -> dict:
        return asdict(self)

    @classmethod
    def from_dict(cls, d: dict) -> ModelProfile:
        valid_fields = {f.name for f in cls.__dataclass_fields__.values()}
        return cls(**{k: v for k, v in d.items() if k in valid_fields})


DEFAULT_PROFILES: dict[str, ModelProfile] = {
    "tiny": ModelProfile(
        max_tools=4, use_router=True, max_tools_per_turn=1, tool_result_limit=5,
        schema_mode="filtered", enable_sql_agent=False, sql_complexity="simple", max_sql_retries=0,
        few_shot_count=3, system_prompt_mode="compact", max_context_tokens=2000, history_messages=5,
        reasoning_mode="guided", temperature=0.2, top_p=0.8, top_k=20, presence_penalty=1.5, max_response_tokens=500,
    ),
    "small": ModelProfile(
        max_tools=6, use_router=True, max_tools_per_turn=3, tool_result_limit=12,
        schema_mode="filtered", enable_sql_agent=True, sql_complexity="simple", max_sql_retries=1,
        few_shot_count=3, system_prompt_mode="compact", max_context_tokens=12000, history_messages=10,
        reasoning_mode="guided", temperature=0.3, top_p=0.8, top_k=20, presence_penalty=1.5, max_response_tokens=1000,
    ),
    "medium": ModelProfile(
        max_tools=10, use_router=True, max_tools_per_turn=2, tool_result_limit=15,
        schema_mode="filtered", enable_sql_agent=True, sql_complexity="moderate", max_sql_retries=2,
        few_shot_count=2, system_prompt_mode="standard", max_context_tokens=8000, history_messages=20,
        reasoning_mode="free", temperature=0.5, top_p=0.9, top_k=40, presence_penalty=1.0, max_response_tokens=2000,
    ),
    "large": ModelProfile(
        max_tools=20, use_router=False, max_tools_per_turn=5, tool_result_limit=50,
        schema_mode="full", enable_sql_agent=True, sql_complexity="advanced", max_sql_retries=2,
        few_shot_count=0, system_prompt_mode="rich", max_context_tokens=16000, history_messages=30,
        reasoning_mode="free", temperature=0.7, top_p=0.95, top_k=50, presence_penalty=0.5, max_response_tokens=4000,
    ),
}

# Regex patterns to detect model size from name
_SIZE_PATTERNS: list[tuple[str, str]] = [
    # API models → large
    (r"claude|gpt-4|gemini|command-r-plus", "large"),
    # Explicit size in name
    (r"(\d+)b", "_extract"),  # handled by _extract_size
]

_API_MODEL_NAMES = {"claude", "gpt-4", "gpt-4o", "gemini", "command-r"}


def detect_model_size(model_name: str) -> str:
    """Detect model size category from model name string."""
    name = model_name.lower().replace("-", "").replace("_", "")
    name_raw = model_name.lower()

    # API models are always "large"
    for api_name in _API_MODEL_NAMES:
        if api_name.replace("-", "") in name:
            return "large"

    # Size keywords in name → tiny
    if re.search(r"\b(mini|nano|micro|tiny)\b", name_raw):
        return "tiny"

    # Extract parameter count — use raw name (with separators) so that
    # version numbers like "3.5" in "qwen3.5-9b" are not concatenated
    # with the actual param count.
    match = re.search(r"(?<![.\d])(\d+)b(?!\w)", name_raw)
    if match:
        params = int(match.group(1))
        if params <= 4:
            return "tiny"
        if params <= 12:
            return "small"
        if params <= 40:
            return "medium"
        return "large"

    return "small"  # safe default


def get_profile(
    profile_name: str,
    model_name: str | None = None,
    overrides: dict | None = None,
) -> ModelProfile:
    """Get a model profile, optionally with overrides.

    Args:
        profile_name: "tiny", "small", "medium", "large", or "auto"
        model_name: Model name for auto-detection (required if profile_name="auto")
        overrides: Dict of field overrides from user settings
    """
    if profile_name == "auto":
        if not model_name:
            profile_name = "small"
        else:
            profile_name = detect_model_size(model_name)

    base = DEFAULT_PROFILES.get(profile_name, DEFAULT_PROFILES["small"])
    profile = ModelProfile(**base.to_dict())  # copy

    if overrides:
        for key, value in overrides.items():
            if hasattr(profile, key):
                setattr(profile, key, value)

    return profile
