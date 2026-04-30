"""Tests for ML activity tool sport tuple and modifier filters."""

import asyncio
from datetime import date
from uuid import uuid4

from agent.tools import _aggregate_stats, _query_activities, _query_segments, get_tool_definitions


class CapturePool:
    def __init__(self):
        self.calls = []

    async def fetch(self, query, *params):
        self.calls.append((query, params))
        return []


def test_query_activities_filters_by_modifiers_and_exact_no_subtype():
    pool = CapturePool()

    asyncio.run(
        _query_activities(
            pool,
            uuid4(),
            sport_type="cycling",
            sub_type="",
            is_electric=True,
            is_indoor=False,
            is_virtual=False,
            is_commute=True,
        )
    )

    query, params = pool.calls[0]
    assert "a.sport_type = $" in query
    assert "a.sub_type IS NULL" in query
    assert "a.is_electric = $" in query
    assert "a.is_indoor = $" in query
    assert "a.is_virtual = $" in query
    assert "a.is_commute = $" in query
    assert True in params
    assert False in params


def test_aggregate_stats_filters_by_subtype_and_modifiers():
    pool = CapturePool()

    asyncio.run(
        _aggregate_stats(
            pool,
            uuid4(),
            metric="count",
            sport_type="cycling",
            sub_type="mountain",
            is_electric=True,
            is_indoor=False,
            is_virtual=False,
            is_commute=False,
        )
    )

    query, _ = pool.calls[0]
    assert "sport_type = $" in query
    assert "sub_type = $" in query
    assert "is_electric = $" in query
    assert "is_indoor = $" in query
    assert "is_virtual = $" in query
    assert "is_commute = $" in query


def test_aggregate_stats_can_group_by_sport_tuple():
    pool = CapturePool()

    asyncio.run(
        _aggregate_stats(
            pool,
            uuid4(),
            metric="count",
            group_by="sport_tuple",
        )
    )

    query, _ = pool.calls[0]
    assert "sport_type, sub_type" in query
    assert "GROUP BY sport_type, sub_type" in query


def test_activity_tools_expose_modifier_parameters():
    tools = {
        tool["function"]["name"]: tool["function"]["parameters"]["properties"]
        for tool in get_tool_definitions(None)
    }

    for name in ("query_activities", "aggregate_stats", "compare_periods", "query_geo_features", "search_routes"):
        assert "sub_type" in tools[name]
        assert "is_electric" in tools[name]
        assert "is_indoor" in tools[name]
        assert "is_virtual" in tools[name]
        assert "is_commute" in tools[name]

    assert "sport_tuple" in tools["aggregate_stats"]["group_by"]["enum"]
    assert "paddling" in tools["query_activities"]["sport_type"]["enum"]
    assert "paddle_boarding" not in tools["query_activities"]["sport_type"]["enum"]


def test_query_segments_filters_by_sport_tuple_modifiers_and_leaderboard():
    pool = CapturePool()

    asyncio.run(
        _query_segments(
            pool,
            uuid4(),
            sport_type="cycling",
            sub_type="mountain",
            is_electric=True,
            is_indoor=False,
            is_virtual=False,
            is_commute=False,
            in_leaderboard=True,
        )
    )

    query, params = pool.calls[0]
    assert "JOIN activities a ON a.id = se.activity_id" in query
    assert "se.sport_type = $" in query
    assert "se.sub_type = $" in query
    assert "a.is_electric = $" in query
    assert "a.is_indoor = $" in query
    assert "a.is_virtual = $" in query
    assert "a.is_commute = $" in query
    assert "se.in_leaderboard = $" in query
    assert True in params
    assert False in params


def test_query_segments_tool_exposes_sport_tuple_modifier_and_leaderboard_filters():
    tools = {
        tool["function"]["name"]: tool["function"]["parameters"]["properties"]
        for tool in get_tool_definitions(["query_segments"])
    }

    properties = tools["query_segments"]
    for name in ("sport_type", "sub_type", "is_electric", "is_indoor", "is_virtual", "is_commute", "in_leaderboard"):
        assert name in properties
