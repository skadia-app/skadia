"""Sport taxonomy prompt and tool-contract tests."""
import json
from datetime import date

from agent.tools import get_tool_definitions
from chat.assembler import assemble_system_prompt
from chat.profiles import DEFAULT_PROFILES
from chat.router import get_tools_for_domains, route_query
from chat.skill_loader import load_skills


DEPRECATED_TOP_LEVEL_SPORTS = [
    "mountain" + "_biking",
    "trail" + "_running",
    "open_water" + "_swimming",
]
LEGACY_ELECTRIC_FIELD = "is_" + "ebike"


def test_mtb_query_round_trip_uses_canonical_tuple():
    query = "Zeig mir alle MTB-Touren"

    route = route_query(query, DEFAULT_PROFILES["small"])
    tool_names = get_tools_for_domains(route.domains, DEFAULT_PROFILES["small"])
    query_tool = get_tool_definitions(["query_activities"])[0]
    properties = query_tool["function"]["parameters"]["properties"]

    expected_tool_call = {
        "name": "query_activities",
        "arguments": {"sport_type": "cycling", "sub_type": "mountain"},
    }

    assert "activities" in route.domains
    assert "query_activities" in tool_names
    assert "cycling" in properties["sport_type"]["enum"]
    assert "sub_type" in properties
    assert expected_tool_call["arguments"]["sport_type"] == "cycling"
    assert expected_tool_call["arguments"]["sub_type"] == "mountain"
    assert expected_tool_call["arguments"].get("sport_type") != DEPRECATED_TOP_LEVEL_SPORTS[0]


def test_prompt_maps_specific_sports_to_tuple_fields():
    skills = load_skills()
    prompt = assemble_system_prompt(
        DEFAULT_PROFILES["small"],
        [skills["activities"], skills["geo"]],
        today=date(2026, 4, 29),
    )

    assert "MTB/Mountainbike -> sport_type='cycling', sub_type='mountain'" in _ascii(prompt)
    assert "Gravel -> sport_type='cycling', sub_type='gravel_cycling'" in _ascii(prompt)
    assert "Trail -> sport_type='running', sub_type='trail'" in _ascii(prompt)
    assert "E-Bike -> sport_type='cycling', is_electric=true" in _ascii(prompt)
    assert "Ski allgemein -> sport_type='skiing'" in _ascii(prompt)
    assert "Alpin-Ski -> sport_type='alpine_skiing'" in _ascii(prompt)
    assert "Kraft allgemein -> sport_type='strength'" in _ascii(prompt)
    assert "Krafttraining -> sport_type='strength_training'" in _ascii(prompt)
    for sport_type in DEPRECATED_TOP_LEVEL_SPORTS:
        assert sport_type not in prompt
    assert LEGACY_ELECTRIC_FIELD not in prompt


def test_tool_descriptions_do_not_recommend_deprecated_top_level_sports():
    tool_text = json.dumps(get_tool_definitions(None), ensure_ascii=False)

    assert "sport_type='cycling' AND sub_type='mountain'" in tool_text
    assert "sport_type='cycling' AND sub_type='gravel_cycling'" in tool_text
    assert "sport_type='running' AND sub_type='trail'" in tool_text
    assert "sport_type='swimming' AND sub_type='open_water'" in tool_text
    assert "sport_type='cycling' AND sub_type='indoor_cycling'" in tool_text
    assert "sport_type='skiing'" in tool_text
    assert "sport_type='strength'" in tool_text
    assert "is_electric=true" in tool_text
    assert "is_indoor=true" in tool_text
    assert "is_virtual=true" in tool_text
    assert "is_commute=true" in tool_text
    for sport_type in DEPRECATED_TOP_LEVEL_SPORTS:
        assert f"sport_type='{sport_type}'" not in tool_text
    assert LEGACY_ELECTRIC_FIELD not in tool_text


def test_tool_descriptions_explain_exact_no_subtype_as_null_storage():
    tool_text = json.dumps(get_tool_definitions(None), ensure_ascii=False)

    assert "stored as NULL" in tool_text
    assert "Use '' for exact no-subtype rows" not in tool_text


def _ascii(value: str) -> str:
    return value.replace("→", "->")
