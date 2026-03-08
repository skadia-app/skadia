"""Tests for tool definitions and execution."""
import pytest
from agent.tools import TOOL_DEFINITIONS, ALL_TOOL_DEFINITIONS, get_tool_definitions


def test_all_tools_defined():
    names = {t["function"]["name"] for t in ALL_TOOL_DEFINITIONS}
    expected = {
        "query_activities", "aggregate_stats", "get_health_status",
        "get_training_load", "compare_periods", "get_gear_stats",
        "query_geo_features", "query_segments", "get_analytics",
        "search_routes", "query_data",
    }
    assert names == expected


def test_tool_descriptions_have_anti_patterns():
    """Every tool description should say when NOT to use it."""
    for tool in ALL_TOOL_DEFINITIONS:
        desc = tool["function"]["description"]
        assert "Do NOT" in desc or "not" in desc.lower() or len(ALL_TOOL_DEFINITIONS) < 8, \
            f"Tool {tool['function']['name']} missing anti-pattern in description"


def test_sport_type_uses_enum():
    """sport_type parameters should use enum, not free text."""
    for tool in ALL_TOOL_DEFINITIONS:
        params = tool["function"].get("parameters", {}).get("properties", {})
        if "sport_type" in params:
            assert "enum" in params["sport_type"], \
                f"Tool {tool['function']['name']}: sport_type should use enum"


def test_get_tool_definitions_filters():
    """get_tool_definitions should filter by tool names."""
    filtered = get_tool_definitions(["query_activities", "aggregate_stats"])
    assert len(filtered) == 2
    names = {t["function"]["name"] for t in filtered}
    assert names == {"query_activities", "aggregate_stats"}


def test_get_tool_definitions_all():
    """Passing None returns all tools."""
    all_tools = get_tool_definitions(None)
    assert len(all_tools) == len(ALL_TOOL_DEFINITIONS)
