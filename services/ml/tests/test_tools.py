"""Tests for tool definitions and execution."""
import pytest
from agent.tools import TOOL_DEFINITIONS, ALL_TOOL_DEFINITIONS, get_tool_definitions


def test_all_tools_defined():
    names = {t["function"]["name"] for t in ALL_TOOL_DEFINITIONS}
    expected = {
        "query_activities", "aggregate_stats", "get_health_status",
        "get_training_load", "compare_periods", "get_gear_stats",
        "query_geo_features", "query_segments", "get_analytics",
        "search_routes", "run_sql",
        "get_gear_for_activity", "get_gear_documents",
        "get_activity_detail", "search_photos_by_content",
        "search_pois_nearby", "search_trails_by_difficulty", "plan_route",
        "search_pois_on_route", "get_isochrone", "query_coverage",
    }
    assert names == expected


def test_primary_tool_descriptions_have_anti_patterns():
    """Primary tools should say when NOT to use them (routing guidance)."""
    primary_tools = {
        "query_activities", "aggregate_stats", "get_health_status",
        "get_training_load", "query_geo_features", "query_segments",
        "search_routes",
    }
    for tool in ALL_TOOL_DEFINITIONS:
        name = tool["function"]["name"]
        if name not in primary_tools:
            continue
        desc = tool["function"]["description"]
        assert "Do NOT" in desc or "not" in desc.lower(), \
            f"Tool {name} missing anti-pattern in description"


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
