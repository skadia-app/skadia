"""Tests for model capability profiles."""
import pytest
from chat.profiles import ModelProfile, DEFAULT_PROFILES, get_profile, detect_model_size


def test_default_profiles_exist():
    assert "tiny" in DEFAULT_PROFILES
    assert "small" in DEFAULT_PROFILES
    assert "medium" in DEFAULT_PROFILES
    assert "large" in DEFAULT_PROFILES


def test_small_profile_values():
    p = DEFAULT_PROFILES["small"]
    assert p.max_tools == 6
    assert p.use_router is True
    assert p.max_tools_per_turn == 1
    assert p.temperature == 0.3
    assert p.enable_sql_agent is True
    assert p.reasoning_mode == "guided"


def test_large_profile_values():
    p = DEFAULT_PROFILES["large"]
    assert p.max_tools == 20
    assert p.use_router is False
    assert p.max_tools_per_turn == 5
    assert p.temperature == 0.7
    assert p.reasoning_mode == "free"


def test_detect_model_size():
    assert detect_model_size("qwen3.5-9b") == "small"
    assert detect_model_size("qwen3.5-35b") == "medium"
    assert detect_model_size("llama-3.1-8b") == "small"
    assert detect_model_size("llama-3.1-70b") == "large"
    assert detect_model_size("claude-opus-4-6") == "large"
    assert detect_model_size("gpt-4o") == "large"
    assert detect_model_size("qwen3.5-3b") == "tiny"
    assert detect_model_size("phi-4-mini") == "tiny"
    assert detect_model_size("unknown-model") == "small"  # safe default


def test_get_profile_with_overrides():
    p = get_profile("small", overrides={"temperature": 0.5, "max_tools": 8})
    assert p.temperature == 0.5
    assert p.max_tools == 8
    assert p.use_router is True  # unchanged


def test_get_profile_auto_detect():
    p = get_profile("auto", model_name="claude-opus-4-6")
    assert p.max_tools == 20
    assert p.use_router is False


def test_profile_to_dict_roundtrip():
    p = DEFAULT_PROFILES["small"]
    d = p.to_dict()
    p2 = ModelProfile.from_dict(d)
    assert p == p2
