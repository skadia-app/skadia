"""Tests for Phase 11.5 gear skill definitions and tool registrations.

Covers:
  - Task 11.5.1: inventory verification (tool names, domains, routing)
  - Task 11.5.2: read-tool presence checks
    - TestSkillCanListBikes
    - TestSkillCanGetComponentDetail
    - TestSkillCanQueryServiceEvents
    - TestSkillCanQueryGearSummary
"""
import inspect
import pytest
from agent.tools import ALL_TOOL_DEFINITIONS, TOOL_DEFINITIONS, get_tool_definitions, _gear_summary, _get_gear_stats
from chat.router import DOMAIN_TOOLS, route_query
from chat.skill_loader import load_skills
from chat.profiles import DEFAULT_PROFILES


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _tool_names(tool_list):
    return {t["function"]["name"] for t in tool_list}


def _get_tool_def(name):
    for t in ALL_TOOL_DEFINITIONS:
        if t["function"]["name"] == name:
            return t
    return None


# ---------------------------------------------------------------------------
# Task 11.5.1 — inventory: new read tools registered in ALL_TOOL_DEFINITIONS
# ---------------------------------------------------------------------------

class TestSkillInventory:
    """Verify that all gear read tools are registered and domain-mapped."""

    def test_gear_list_tool_registered(self):
        names = _tool_names(ALL_TOOL_DEFINITIONS)
        assert "gear_list" in names, "gear_list tool must be in ALL_TOOL_DEFINITIONS"

    def test_gear_component_detail_tool_registered(self):
        names = _tool_names(ALL_TOOL_DEFINITIONS)
        assert "gear_component_detail" in names

    def test_gear_service_events_tool_registered(self):
        names = _tool_names(ALL_TOOL_DEFINITIONS)
        assert "gear_service_events" in names

    def test_gear_summary_tool_registered(self):
        names = _tool_names(ALL_TOOL_DEFINITIONS)
        assert "gear_summary" in names

    def test_new_gear_tools_in_domain_tools(self):
        gear_tools = set(DOMAIN_TOOLS.get("gear", []))
        assert "gear_list" in gear_tools
        assert "gear_component_detail" in gear_tools
        assert "gear_service_events" in gear_tools
        assert "gear_summary" in gear_tools

    def test_new_gear_tools_in_get_tool_definitions_all(self):
        all_tools = get_tool_definitions(None)
        names = _tool_names(all_tools)
        assert "gear_list" in names
        assert "gear_component_detail" in names
        assert "gear_service_events" in names
        assert "gear_summary" in names


# ---------------------------------------------------------------------------
# Task 11.5.2 — TestSkillCanListBikes
# ---------------------------------------------------------------------------

class TestSkillCanListBikes:
    """gear_list tool covers 'Wieviele Schuhe besitze ich?' and bike listing."""

    def test_gear_list_has_status_param(self):
        t = _get_tool_def("gear_list")
        assert t is not None
        props = t["function"]["parameters"]["properties"]
        assert "status" in props, "gear_list must expose status filter"

    def test_gear_list_status_has_enum(self):
        t = _get_tool_def("gear_list")
        props = t["function"]["parameters"]["properties"]
        assert "enum" in props["status"]
        assert set(props["status"]["enum"]) >= {"active", "retired", "stored"}

    def test_gear_list_has_type_param(self):
        t = _get_tool_def("gear_list")
        props = t["function"]["parameters"]["properties"]
        assert "type" in props, "gear_list must expose type filter for bike/shoe queries"

    def test_gear_list_has_scope_param(self):
        t = _get_tool_def("gear_list")
        props = t["function"]["parameters"]["properties"]
        assert "scope" in props

    def test_gear_list_description_mentions_use_cases(self):
        t = _get_tool_def("gear_list")
        desc = t["function"]["description"].lower()
        # Must describe the disambiguation / list use-case
        assert "list" in desc or "liste" in desc or "ausrüstung" in desc

    def test_gear_routed_when_query_about_bikes(self):
        profile = DEFAULT_PROFILES["small"]
        result = route_query("Wieviele Fahrräder habe ich?", profile)
        assert "gear" in result.domains

    def test_gear_list_tool_returned_for_gear_domain(self):
        tools = get_tool_definitions(DOMAIN_TOOLS.get("gear", []))
        names = _tool_names(tools)
        assert "gear_list" in names


# ---------------------------------------------------------------------------
# Task 11.5.2 — TestSkillCanGetComponentDetail
# ---------------------------------------------------------------------------

class TestSkillCanGetComponentDetail:
    """gear_component_detail covers 'Wieviel ist die Kette schon gelaufen?'"""

    def test_component_detail_has_gear_id_param(self):
        t = _get_tool_def("gear_component_detail")
        assert t is not None
        props = t["function"]["parameters"]["properties"]
        assert "gear_id" in props, "gear_component_detail needs gear_id to filter components"

    def test_component_detail_has_component_type_param(self):
        t = _get_tool_def("gear_component_detail")
        props = t["function"]["parameters"]["properties"]
        assert "component_type" in props, "component_type filter needed for 'Kette' queries"

    def test_component_detail_has_include_retired_param(self):
        t = _get_tool_def("gear_component_detail")
        props = t["function"]["parameters"]["properties"]
        assert "include_retired" in props

    def test_component_detail_description_mentions_components(self):
        t = _get_tool_def("gear_component_detail")
        desc = t["function"]["description"].lower()
        assert "component" in desc or "komponente" in desc or "kette" in desc

    def test_component_detail_in_gear_domain_tools(self):
        gear_tools = set(DOMAIN_TOOLS.get("gear", []))
        assert "gear_component_detail" in gear_tools


# ---------------------------------------------------------------------------
# Task 11.5.2 — TestSkillCanQueryServiceEvents
# ---------------------------------------------------------------------------

class TestSkillCanQueryServiceEvents:
    """gear_service_events covers 'Wann wurde mein MTB zuletzt gewartet?'"""

    def test_service_events_has_gear_id_param(self):
        t = _get_tool_def("gear_service_events")
        assert t is not None
        props = t["function"]["parameters"]["properties"]
        assert "gear_id" in props

    def test_service_events_has_limit_param(self):
        t = _get_tool_def("gear_service_events")
        props = t["function"]["parameters"]["properties"]
        assert "limit" in props

    def test_service_events_has_order_param(self):
        t = _get_tool_def("gear_service_events")
        props = t["function"]["parameters"]["properties"]
        assert "order" in props
        assert "enum" in props["order"]
        assert set(props["order"]["enum"]) >= {"asc", "desc"}

    def test_service_events_description_mentions_service(self):
        t = _get_tool_def("gear_service_events")
        desc = t["function"]["description"].lower()
        assert "service" in desc or "wartung" in desc or "maintenance" in desc

    def test_service_events_in_gear_domain_tools(self):
        gear_tools = set(DOMAIN_TOOLS.get("gear", []))
        assert "gear_service_events" in gear_tools


# ---------------------------------------------------------------------------
# Task 11.5.2 — TestSkillCanQueryGearSummary
# ---------------------------------------------------------------------------

class TestSkillCanQueryGearSummary:
    """gear_summary covers 'Wieviele km mit allen Rädern letztes Jahr?'"""

    def test_gear_summary_has_scope_param(self):
        t = _get_tool_def("gear_summary")
        assert t is not None
        props = t["function"]["parameters"]["properties"]
        assert "scope" in props

    def test_gear_summary_has_type_key_param(self):
        t = _get_tool_def("gear_summary")
        props = t["function"]["parameters"]["properties"]
        assert "type_key" in props

    def test_gear_summary_has_since_param(self):
        t = _get_tool_def("gear_summary")
        props = t["function"]["parameters"]["properties"]
        assert "since" in props

    def test_gear_summary_has_until_param(self):
        t = _get_tool_def("gear_summary")
        props = t["function"]["parameters"]["properties"]
        assert "until" in props

    def test_gear_summary_has_include_retired_param(self):
        t = _get_tool_def("gear_summary")
        props = t["function"]["parameters"]["properties"]
        assert "include_retired" in props

    def test_gear_summary_description_mentions_aggregation(self):
        t = _get_tool_def("gear_summary")
        desc = t["function"]["description"].lower()
        assert any(k in desc for k in ("summ", "gesamt", "total", "aggregat", "km")), \
            "gear_summary description must describe aggregation use-case"

    def test_gear_summary_in_gear_domain_tools(self):
        gear_tools = set(DOMAIN_TOOLS.get("gear", []))
        assert "gear_summary" in gear_tools


# ---------------------------------------------------------------------------
# Schema excerpt (gear.yaml) reflects rebuild schema fields
# ---------------------------------------------------------------------------

class TestGearYamlSchemaExcerpt:
    """gear.yaml schema_excerpt must include rebuild-era fields."""

    def test_schema_excerpt_has_attributes(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "attributes" in excerpt

    def test_schema_excerpt_has_modifiers(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "modifiers" in excerpt

    def test_schema_excerpt_has_applicable_sport_predicates(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "applicable_sport_predicates" in excerpt

    def test_schema_excerpt_has_scope(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "scope" in excerpt

    def test_schema_excerpt_has_is_retired(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "is_retired" in excerpt

    def test_schema_excerpt_has_total_distance_m(self):
        skills = load_skills()
        excerpt = skills["gear"].schema_excerpt
        assert "total_distance_m" in excerpt


# ---------------------------------------------------------------------------
# Gear skill domain knowledge has disambiguation pattern
# ---------------------------------------------------------------------------

class TestGearSkillDomainKnowledge:
    """domain_knowledge must document the name-ambiguity disambiguation pattern."""

    def test_domain_knowledge_has_disambiguation_note(self):
        skills = load_skills()
        dk = skills["gear"].domain_knowledge
        # Check for name ambiguity guidance (gear.name_ambiguous or candidates)
        assert "candidates" in dk or "gear.name_ambiguous" in dk or "name_ambiguous" in dk, \
            "domain_knowledge must document name-ambiguity pattern (gear.name_ambiguous / candidates)"

    def test_domain_knowledge_has_status_values(self):
        skills = load_skills()
        dk = skills["gear"].domain_knowledge
        assert "active" in dk
        assert "retired" in dk

    def test_gear_skill_has_new_tools_in_tools_list(self):
        skills = load_skills()
        tool_names = set(skills["gear"].tools)
        assert "gear_list" in tool_names
        assert "gear_component_detail" in tool_names
        assert "gear_service_events" in tool_names
        assert "gear_summary" in tool_names


# ---------------------------------------------------------------------------
# Critic #13 — gear_list defaults to active-only
# ---------------------------------------------------------------------------

class TestGearListDefaultExcludesRetired:
    """gear_list tool must declare include_retired param (Critic #13)."""

    def test_gear_list_has_include_retired_param(self):
        t = _get_tool_def("gear_list")
        assert t is not None
        props = t["function"]["parameters"]["properties"]
        assert "include_retired" in props, (
            "gear_list must expose include_retired for opt-in retired visibility"
        )

    def test_gear_list_include_retired_is_boolean(self):
        t = _get_tool_def("gear_list")
        props = t["function"]["parameters"]["properties"]
        assert props["include_retired"]["type"] == "boolean"

    def test_gear_list_description_mentions_active_default(self):
        t = _get_tool_def("gear_list")
        desc = t["function"]["description"].lower()
        # Description must indicate that the default excludes retired items.
        assert "retired" in desc, (
            "gear_list description must document the active-only default"
        )


class TestGearListIncludeRetiredOptIn:
    """gear_list status param description must explain include_retired interaction."""

    def test_gear_list_status_description_mentions_include_retired(self):
        t = _get_tool_def("gear_list")
        props = t["function"]["parameters"]["properties"]
        status_desc = props["status"].get("description", "").lower()
        assert "include_retired" in status_desc, (
            "status field description should reference include_retired override"
        )


# ---------------------------------------------------------------------------
# Re-Critic #13 — _gear_summary uses is_retired column, not status string
# ---------------------------------------------------------------------------

class TestGearSummaryUsesIsRetiredColumn:
    """_gear_summary must filter on the boolean is_retired column, not the
    status text column, consistent with _gear_list (Re-Critic #13)."""

    def test_gear_summary_filters_on_is_retired_not_status(self):
        src = inspect.getsource(_gear_summary)
        assert "is_retired" in src, (
            "_gear_summary must reference is_retired column (boolean) for retire filtering"
        )
        assert "status != 'retired'" not in src, (
            "_gear_summary must not filter via status string — use is_retired boolean column"
        )


# Re-Critic Wave-3 #9 — _get_gear_stats uses is_retired column, not status string
# ---------------------------------------------------------------------------

class TestGearStatsUsesIsRetiredColumn:
    """_get_gear_stats must filter on the boolean is_retired column, not the
    status text column, consistent with _gear_summary (Wave-3 Critic #9)."""

    def test_gear_stats_filters_on_is_retired_not_status(self):
        src = inspect.getsource(_get_gear_stats)
        assert "is_retired" in src, (
            "_get_gear_stats must reference is_retired column (boolean) for retire filtering"
        )
        assert "status = 'active'" not in src, (
            "_get_gear_stats must not filter via status string — use is_retired boolean column"
        )
