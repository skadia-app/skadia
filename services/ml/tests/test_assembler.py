"""Tests for context assembler."""
import pytest
from datetime import date
from chat.assembler import assemble_system_prompt
from chat.profiles import DEFAULT_PROFILES
from chat.skill_loader import load_skills


def test_compact_prompt_contains_rules():
    prompt = assemble_system_prompt(DEFAULT_PROFILES["small"], [], today=date(2026, 3, 8))
    assert "08.03.2026" in prompt
    assert "Trainingsanalyst" in prompt
    assert "Regeln" in prompt
    assert "Referenzwerte" in prompt


def test_prompt_with_skills():
    skills = load_skills()
    geo_skill = skills["geo"]
    prompt = assemble_system_prompt(DEFAULT_PROFILES["small"], [geo_skill], today=date(2026, 3, 8))
    assert "activity_geo_tags" in prompt
    assert "Gipfel bestiegen" in prompt


def test_prompt_with_few_shots():
    skills = load_skills()
    prompt = assemble_system_prompt(DEFAULT_PROFILES["small"], [skills["activities"]])
    assert "Beispiele" in prompt
    assert "aggregate_stats" in prompt


def test_prompt_without_few_shots_for_large():
    prompt = assemble_system_prompt(DEFAULT_PROFILES["large"], [])
    assert "Beispiele" not in prompt  # large profile has few_shot_count=0


def test_rich_prompt_has_training_theory():
    prompt = assemble_system_prompt(DEFAULT_PROFILES["large"], [])
    assert "Grundlagenausdauer" in prompt
    assert "Periodisierung" in prompt


def test_tool_guide_only_for_routed():
    small_prompt = assemble_system_prompt(DEFAULT_PROFILES["small"], [])
    large_prompt = assemble_system_prompt(DEFAULT_PROFILES["large"], [])
    assert "Werkzeug-Auswahl" in small_prompt
    assert "Werkzeug-Auswahl" not in large_prompt
