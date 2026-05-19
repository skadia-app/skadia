package agent

import "testing"

func TestClassifyAgentIntentReadOnlyQuestions(t *testing.T) {
	profile := GetProfile("small", "", nil)
	tests := []struct {
		name       string
		query      string
		wantDomain string
		wantOp     IntentOperation
	}{
		{name: "last activity english", query: "Can you tell me the name of my last activity?", wantDomain: "activities", wantOp: IntentRead},
		{name: "segment exists", query: "Existiert ein Segment mit einem frei gewaehlten Testnamen?", wantDomain: "segments", wantOp: IntentRead},
		{name: "weather status", query: "Welche Wetterdaten kannst du fuer einen Ort abfragen?", wantDomain: "routing", wantOp: IntentRead},
		{name: "place distance", query: "Wie weit ist synthetischer Ort A von synthetischer Ort B entfernt?", wantDomain: "routing", wantOp: IntentRead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyAgentIntent(tt.query, &profile)
			if got.Operation != tt.wantOp {
				t.Fatalf("operation = %s, want %s; intent=%#v", got.Operation, tt.wantOp, got)
			}
			if !containsToolName(got.Domains, tt.wantDomain) {
				t.Fatalf("domains = %#v, want %q; intent=%#v", got.Domains, tt.wantDomain, got)
			}
			if got.WriteRequested {
				t.Fatalf("WriteRequested = true for read-only query %q", tt.query)
			}
		})
	}
}

func TestBuildToolPlanDoesNotExposeWritesForReadOnlyQuestion(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Existiert ein Segment namens synthetischer Test?", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_gear", "create_tag", "delete_tag", "update_activity_metadata"},
	})
	for _, name := range plan.Tools {
		switch name {
		case "create_gear", "create_tag", "delete_tag", "update_activity_metadata":
			t.Fatalf("read-only plan exposed write tool %q: %#v", name, plan.Tools)
		}
	}
	if len(plan.Tools) > profile.MaxTools {
		t.Fatalf("tools = %#v, max %d", plan.Tools, profile.MaxTools)
	}
	for _, name := range []string{"create_gear", "create_tag", "delete_tag", "update_activity_metadata"} {
		if !hasToolOmission(plan.OmittedTools, name, "write_intent_not_requested") {
			t.Fatalf("omitted tools = %#v, want %s omitted because write intent was not requested", plan.OmittedTools, name)
		}
	}
}

func TestBuildToolPlanRoutesCapabilityQuestionToManifestOnly(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Welche Tools und Funktionen stehen dir zur Verfuegung?", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_gear", "delete_tag", "bulk_tag_activities"},
	})
	if plan.Intent.Reason != "capability-question" {
		t.Fatalf("reason = %q, want capability-question; plan=%#v", plan.Intent.Reason, plan)
	}
	if len(plan.Tools) != 1 || plan.Tools[0] != "get_agent_capability_manifest" {
		t.Fatalf("tools = %#v, want manifest only", plan.Tools)
	}
}

func TestBuildToolPlanExposesOnlyRelevantWriteTools(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Lege ein Fahrrad mit dem Namen TestGear an", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_gear", "create_tag", "delete_tag"},
	})
	if !containsToolName(plan.Tools, "create_gear") {
		t.Fatalf("tools = %#v, want create_gear", plan.Tools)
	}
	if containsToolName(plan.Tools, "create_tag") || containsToolName(plan.Tools, "delete_tag") {
		t.Fatalf("tools = %#v, did not want unrelated tag writes", plan.Tools)
	}
}

func TestBuildToolPlanUsesComponentToolForExistingGearComponent(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Fuege meinem Test-Fahrrad eine neue Kette hinzu", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_gear", "add_gear_component", "create_chain_rotation"},
	})
	if !containsToolName(plan.Tools, "add_gear_component") {
		t.Fatalf("tools = %#v, want add_gear_component", plan.Tools)
	}
	if containsToolName(plan.Tools, "create_gear") {
		t.Fatalf("tools = %#v, did not want create_gear for a component on existing gear", plan.Tools)
	}
}

func TestBuildToolPlanIncludesSetupToolsForGearWithChainRotation(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Erstelle ein neues Test-Rennrad mit zwei Ketten in Rotation", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"create_gear", "add_gear_component", "create_chain_rotation"},
	})
	for _, want := range []string{"create_gear", "add_gear_component", "create_chain_rotation"} {
		if !containsToolName(plan.Tools, want) {
			t.Fatalf("tools = %#v, want %s", plan.Tools, want)
		}
	}
}

func TestBuildToolPlanScopesUpdateWritesToEntity(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Aendere die Metadaten meines Test-Fahrrads", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"update_gear_metadata", "update_tag", "update_automation_rule", "rename_activity"},
	})
	if !containsToolName(plan.Tools, "update_gear_metadata") {
		t.Fatalf("tools = %#v, want update_gear_metadata", plan.Tools)
	}
	for _, reject := range []string{"update_tag", "update_automation_rule", "rename_activity"} {
		if containsToolName(plan.Tools, reject) {
			t.Fatalf("tools = %#v, did not want unrelated update write %q", plan.Tools, reject)
		}
	}
}

func hasToolOmission(values []ToolOmission, name, reason string) bool {
	for _, value := range values {
		if value.Name == name && value.Reason == reason {
			return true
		}
	}
	return false
}

func TestBuildToolPlanUsesBulkTagToolForBulkTagRequest(t *testing.T) {
	profile := GetProfile("small", "", nil)
	plan := BuildToolPlan("Fuege Tag Kurz bei allen Radfahrten unter 10 km hinzu", &profile, &ChatPreferences{
		ChatWriteToolsEnabled: true,
		ChatAllowedWriteTools: []string{"bulk_tag_activities", "delete_tag", "create_gear"},
	})
	if plan.Intent.Operation != IntentBulkUpdate {
		t.Fatalf("operation = %s, want bulk_update; intent=%#v", plan.Intent.Operation, plan.Intent)
	}
	if !containsToolName(plan.Tools, "bulk_tag_activities") {
		t.Fatalf("tools = %#v, want bulk_tag_activities", plan.Tools)
	}
	if containsToolName(plan.Tools, "delete_tag") || containsToolName(plan.Tools, "create_gear") {
		t.Fatalf("tools = %#v, did not want unrelated writes", plan.Tools)
	}
}

func TestBuildToolPlanKeepsRoutingRequiredToolsOverMaxTools(t *testing.T) {
	profile := GetProfile("small", "", map[string]any{"max_tools": 4})
	query := "Plane eine Route von synthetischem Startort zu synthetischem Zielort und nenne die Luftlinie."
	plan := BuildToolPlan(query, &profile, &ChatPreferences{})
	for _, want := range []string{"get_agent_service_status", "geocode_place", "resolve_user_location", "resolve_route_place", "analyze_route_points", "get_routing_coverage", "list_routing_profiles", "plan_route_between"} {
		if !containsToolName(plan.Tools, want) {
			t.Fatalf("tools = %#v, want required routing tool %q", plan.Tools, want)
		}
	}
}
