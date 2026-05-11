"""End-to-end smoke tests for the 7 gear user-example questions (Phase 11.5.4).

These tests verify that the correct tool is routed for each natural-language
question. They do NOT need a real DB or LLM — they test the deterministic
routing layer only (keyword router + domain→tool mapping).

Tests that require real containers (API + DB running) are marked with
pytest.mark.skip and annotated so they can be enabled when containers are up.
"""
import pytest
from agent.tools import ALL_TOOL_DEFINITIONS, get_tool_definitions
from chat.router import DOMAIN_TOOLS, route_query, get_tools_for_domains
from chat.profiles import DEFAULT_PROFILES


def _tool_registered(name: str) -> bool:
    return any(t["function"]["name"] == name for t in ALL_TOOL_DEFINITIONS)


SMALL_PROFILE = DEFAULT_PROFILES["small"]


# ---------------------------------------------------------------------------
# Q1: "Wieviel ist die derzeitige Kette schon gelaufen?"
# Expected: gear domain → gear_list (find bike) → gear_component_detail
# ---------------------------------------------------------------------------

class TestQ1ChainDistance:
    def test_gear_routed_for_kette_query(self):
        result = route_query("Wieviel ist die derzeitige Kette schon gelaufen?", SMALL_PROFILE)
        assert "gear" in result.domains

    def test_gear_component_detail_available_in_gear_domain(self):
        tools = get_tools_for_domains(["gear"], SMALL_PROFILE)
        assert "gear_component_detail" in tools

    def test_gear_component_detail_has_gear_id_required(self):
        defs = get_tool_definitions(["gear_component_detail"])
        assert len(defs) == 1
        # gear_id is required for component detail
        assert "gear_id" in defs[0]["function"]["parameters"]["required"]


# ---------------------------------------------------------------------------
# Q2: "Wann habe ich mein Mountainbike zuletzt geserviced?"
# Expected: gear domain → gear_list?type=bike.mtb → gear_service_events
# ---------------------------------------------------------------------------

class TestQ2LastService:
    def test_gear_routed_for_mtb_service_query(self):
        result = route_query("Wann habe ich mein Mountainbike zuletzt geserviced?", SMALL_PROFILE)
        assert "gear" in result.domains

    def test_gear_service_events_in_gear_domain_tools(self):
        tools = get_tools_for_domains(["gear"], SMALL_PROFILE)
        assert "gear_service_events" in tools

    def test_gear_service_events_supports_order_desc(self):
        defs = get_tool_definitions(["gear_service_events"])
        order_prop = defs[0]["function"]["parameters"]["properties"]["order"]
        assert "desc" in order_prop["enum"]

    def test_gear_service_events_supports_limit_1(self):
        defs = get_tool_definitions(["gear_service_events"])
        limit_prop = defs[0]["function"]["parameters"]["properties"]["limit"]
        assert limit_prop["type"] == "integer"


# ---------------------------------------------------------------------------
# Q3: "Wieviele Schuhe besitze ich eigentlich?"
# Expected: gear domain → gear_list?scope=shoes&status=active
# ---------------------------------------------------------------------------

class TestQ3ShoeCount:
    def test_gear_routed_for_schuhe_query(self):
        result = route_query("Wieviele Schuhe besitze ich eigentlich?", SMALL_PROFILE)
        assert "gear" in result.domains

    def test_gear_list_in_gear_domain_tools(self):
        tools = get_tools_for_domains(["gear"], SMALL_PROFILE)
        assert "gear_list" in tools

    def test_gear_list_has_scope_and_status_filters(self):
        defs = get_tool_definitions(["gear_list"])
        props = defs[0]["function"]["parameters"]["properties"]
        assert "scope" in props
        assert "status" in props


# ---------------------------------------------------------------------------
# Q4: "Wieviele Kilometer bin ich mit allen Rädern letztes Jahr gefahren?"
# Expected: gear domain → gear_summary?scope=bike&since=...&until=...
# ---------------------------------------------------------------------------

class TestQ4BikeKmLastYear:
    def test_gear_routed_for_jahreskilometer_query(self):
        result = route_query("Wieviele Kilometer bin ich mit allen Rädern letztes Jahr gefahren?", SMALL_PROFILE)
        assert "gear" in result.domains

    def test_gear_summary_in_gear_domain_tools(self):
        tools = get_tools_for_domains(["gear"], SMALL_PROFILE)
        assert "gear_summary" in tools

    def test_gear_summary_has_date_range_params(self):
        defs = get_tool_definitions(["gear_summary"])
        props = defs[0]["function"]["parameters"]["properties"]
        assert "since" in props
        assert "until" in props
        assert "scope" in props


# ---------------------------------------------------------------------------
# Q5: "Leg mir ein neues Gravel-Bike namens X an"
# Write tool — deferred (requires JWT propagation, Task 11.5.3 skipped).
# ---------------------------------------------------------------------------

@pytest.mark.skip(reason="Write tools deferred: Task 11.5.3 requires JWT in ChatRequest (Task 11.5.0 DONE_WITH_CONCERNS)")
class TestQ5CreateGravelBike:
    def test_gear_create_tool_registered(self):
        assert _tool_registered("gear_create")


# ---------------------------------------------------------------------------
# Q6: "Tausch die Kette an meinem MTB"
# Write tool — deferred (same reason as Q5).
# ---------------------------------------------------------------------------

@pytest.mark.skip(reason="Write tools deferred: Task 11.5.3 requires JWT in ChatRequest")
class TestQ6SwapChain:
    def test_gear_swap_component_tool_registered(self):
        assert _tool_registered("gear_swap_component")


# ---------------------------------------------------------------------------
# Q7: "Ich hab gestern die Bremsbeläge gewechselt"
# Write tool — deferred (same reason as Q5).
# ---------------------------------------------------------------------------

@pytest.mark.skip(reason="Write tools deferred: Task 11.5.3 requires JWT in ChatRequest")
class TestQ7AddServiceEvent:
    def test_gear_add_service_event_tool_registered(self):
        assert _tool_registered("gear_add_service_event")


# ---------------------------------------------------------------------------
# Integration smoke — requires real API + DB containers (skipped by default)
# ---------------------------------------------------------------------------

@pytest.mark.skip(
    reason=(
        "Requires running containers (skadia-api-1, skadia-ml-1) and "
        "per-worktree test DB. Run manually: "
        "./scripts/test_db.sh ensure && eval \"$(./scripts/test_db.sh emit-url)\" "
        "&& python3 -m pytest tests/test_gear_use_cases.py -k integration -v"
    )
)
class TestGearToolsIntegration:
    """Container-dependent smoke tests.

    Fixture requirements:
    - TEST_DATABASE_URL pointing at a per-worktree test DB (gear tables migrated)
    - At least one gear item in the DB for the test user
    - API container running (for idempotency endpoint tests)
    """

    async def test_gear_list_executes_against_db(self, db_pool, test_user_id):
        from agent.tools import execute_tool
        result = await execute_tool(db_pool, test_user_id, "gear_list", {})
        assert "gear" in result
        assert "count" in result

    async def test_gear_summary_executes_against_db(self, db_pool, test_user_id):
        from agent.tools import execute_tool
        result = await execute_tool(db_pool, test_user_id, "gear_summary", {})
        assert "total_distance_km" in result
        assert "gear_count" in result

    async def test_gear_component_detail_returns_error_for_unknown_gear(self, db_pool, test_user_id):
        from agent.tools import execute_tool
        import uuid
        result = await execute_tool(db_pool, test_user_id, "gear_component_detail",
                                     {"gear_id": str(uuid.uuid4())})
        assert "error" in result

    async def test_gear_service_events_returns_error_without_ids(self, db_pool, test_user_id):
        from agent.tools import execute_tool
        result = await execute_tool(db_pool, test_user_id, "gear_service_events", {})
        assert "error" in result
