"""Tests for skill module loader."""
import pytest
from chat.skill_loader import load_skills, Skill


def test_load_all_skills():
    skills = load_skills()
    assert len(skills) == 6
    assert "activities" in skills
    assert "health" in skills
    assert "geo" in skills


def test_skill_has_required_fields():
    skills = load_skills()
    for name, skill in skills.items():
        assert skill.name == name
        assert skill.description
        assert skill.schema_excerpt
        assert skill.domain_knowledge
        assert isinstance(skill.tools, list)
        assert len(skill.tools) > 0
        assert isinstance(skill.few_shot_examples, list)
        assert len(skill.few_shot_examples) > 0


def test_skill_few_shot_format():
    skills = load_skills()
    for skill in skills.values():
        for ex in skill.few_shot_examples:
            assert "q" in ex
            assert "tool" in ex
            assert "answer" in ex


def test_skill_context_string():
    skills = load_skills()
    ctx = skills["geo"].to_context_string()
    assert "activity_geo_tags" in ctx
    assert "Gipfel bestiegen" in ctx
    assert len(ctx) < 2000  # should be compact
