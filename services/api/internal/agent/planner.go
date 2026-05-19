package agent

import (
	"slices"
	"strings"
)

type ToolPlan struct {
	Intent        AgentIntent
	RequiredTools []string
	OptionalTools []string
	Tools         []string
	OmittedTools  []ToolOmission
	Reason        string
}

func BuildToolPlan(query string, profile *ModelProfile, prefs *ChatPreferences) ToolPlan {
	if isCapabilityQuestion(query) {
		intent := AgentIntent{
			Operation:      IntentRead,
			Domains:        []string{"capabilities"},
			Entities:       []IntentEntity{EntityUnknown},
			WriteRequested: false,
			Reason:         "capability-question",
		}
		return ToolPlan{
			Intent:        intent,
			RequiredTools: []string{"get_agent_capability_manifest"},
			Tools:         []string{"get_agent_capability_manifest"},
			Reason:        intent.Reason,
		}
	}
	intent := ClassifyAgentIntent(query, profile)
	if isWeatherCapabilityQuestion(query) {
		intent.Domains = []string{"routing"}
		intent.Entities = []IntentEntity{EntityWeather}
		return ToolPlan{
			Intent:        intent,
			RequiredTools: []string{"get_agent_service_status"},
			Tools:         []string{"get_agent_service_status"},
			Reason:        "weather-capability-question",
		}
	}
	required, optional := readToolsForIntent(query, intent, profile)
	writeTools, writeOmissions := writeToolsForIntent(query, intent, prefs)
	if intent.WriteRequested {
		required = appendUniqueToolNames(required, writeTools...)
	}
	limit := 6
	if profile != nil && profile.MaxTools > 0 {
		limit = profile.MaxTools
	}
	tools, omittedNames := mergeToolPlanNames(required, optional, limit)
	omitted := append(toolLimitOmissions(omittedNames), writeOmissions...)
	return ToolPlan{
		Intent:        intent,
		RequiredTools: required,
		OptionalTools: optional,
		Tools:         tools,
		OmittedTools:  omitted,
		Reason:        intent.Reason,
	}
}

func readToolsForIntent(query string, intent AgentIntent, profile *ModelProfile) ([]string, []string) {
	q := strings.ToLower(query)
	if containsToolName(intent.Domains, "routing") {
		required := routingRequiredTools(q, profile)
		optional := routingOptionalTools(q, profile)
		return required, optional
	}
	if containsToolName(intent.Domains, "segments") {
		required := []string{"query_segments"}
		optional := []string{"resolve_entity"}
		if profileAllowsSQL(profile) {
			optional = append(optional, "run_sql")
		}
		return required, optional
	}
	if containsToolName(intent.Domains, "documents") {
		return []string{"search_documents"}, []string{"get_gear_documents", "find_gear", "resolve_entity"}
	}
	if containsToolName(intent.Domains, "photos") {
		return []string{"search_photos_by_content"}, nil
	}
	if containsToolName(intent.Domains, "gear") {
		required := []string{"find_gear"}
		optional := []string{"get_gear_stats", "get_gear_for_activity", "get_gear_documents", "query_gear_maintenance"}
		if profileAllowsSQL(profile) {
			optional = append(optional, "run_sql")
		}
		return required, optional
	}
	if containsToolName(intent.Domains, "health") {
		return []string{"get_health_status"}, []string{"get_analytics"}
	}
	if containsToolName(intent.Domains, "training") {
		return []string{"get_training_load"}, []string{"aggregate_time_series", "compare_periods", "get_analytics"}
	}
	if containsToolName(intent.Domains, "geo") {
		return []string{"query_geo_features"}, []string{"query_activity_geo_context", "search_routes", "resolve_entity"}
	}

	required := activityRequiredTools(q)
	optional := activityOptionalTools(q, profile)
	return required, optional
}

func routingRequiredTools(q string, profile *ModelProfile) []string {
	tools := []string{"get_agent_service_status", "geocode_place", "resolve_user_location"}
	switch {
	case isDistanceOnlyRoutingQuery(q):
		tools = appendUniqueToolNames(tools, "resolve_route_place", "analyze_route_points")
	case isPointToPointRoutingQuery(q):
		tools = appendUniqueToolNames(tools, "resolve_route_place", "analyze_route_points", "get_routing_coverage", "list_routing_profiles", "plan_route_between")
	case isRoutingGoalQuery(q):
		tools = appendUniqueToolNames(tools, "resolve_route_feature", "analyze_route_points", "get_routing_coverage", "list_routing_profiles", "suggest_route_goal", "plan_route")
	default:
		tools = appendUniqueToolNames(tools, "resolve_route_place", "analyze_route_points")
	}
	if isRoutingWeatherQuery(q) {
		tools = appendUniqueToolNames(tools, "query_weather_at_point_time")
	}
	if isRoutingCoverageQuery(q) {
		tools = appendUniqueToolNames(tools, "get_routing_coverage", "query_coverage")
	}
	return tools
}

func routingOptionalTools(q string, profile *ModelProfile) []string {
	tools := []string{"resolve_route_feature", "query_geo_features", "search_routes"}
	if isRoutingGoalQuery(q) {
		tools = appendUniqueToolNames(tools, "query_activity_terrain_summary", "search_pois_nearby", "search_trails_by_difficulty")
	}
	if profileAllowsSQL(profile) && isRoutingCoverageQuery(q) {
		tools = append(tools, "run_sql")
	}
	return tools
}

func activityRequiredTools(q string) []string {
	switch {
	case containsAny(q, "trend", "trends", "zeitreihe", "zeitreihen", "pro jahr", "pro monat", "jahresvergleich"):
		return []string{"aggregate_time_series"}
	case containsAny(q, "höchste", "hoechste", "highest", "schnellste", "fastest", "längste", "laengste", "longest", "bestwert", "bestwerte", "rekord", "record"):
		return []string{"query_records"}
	case containsAny(q, "wie viele", "wieviele", "how many", "gesamt", "summe"):
		return []string{"aggregate_stats"}
	default:
		return []string{"query_activities"}
	}
}

func activityOptionalTools(q string, profile *ModelProfile) []string {
	tools := []string{"preview_activity_selection", "get_activity_detail", "aggregate_stats", "aggregate_time_series", "query_records", "compare_periods", "resolve_entity"}
	if containsAny(q, "gear", "ausrüstung", "ausruestung", "fahrrad", "rad", "schuh") {
		tools = appendUniqueToolNames(tools, "get_gear_for_activity")
	}
	if containsAny(q, "job", "auftrag", "background") {
		tools = appendUniqueToolNames(tools, "get_background_job_status")
	}
	if profileAllowsSQL(profile) && isExplicitSQLQuestion(q) {
		tools = appendUniqueToolNames(tools, "run_sql")
	}
	return tools
}

func writeToolsForIntent(query string, intent AgentIntent, prefs *ChatPreferences) ([]string, []ToolOmission) {
	candidates := writeToolCandidatesForIntent(query, intent)
	omitted := []ToolOmission{}
	if !intent.WriteRequested {
		if prefs != nil && prefs.ChatWriteToolsEnabled {
			for _, name := range prefs.ChatAllowedWriteTools {
				omitted = appendToolOmission(omitted, ToolOmission{Name: name, Reason: "write_intent_not_requested"})
			}
		}
		return nil, omitted
	}
	if len(candidates) == 0 {
		return nil, []ToolOmission{{Name: "write_tool", Reason: "unsupported_write_entity"}}
	}
	if prefs == nil || !prefs.ChatWriteToolsEnabled {
		for _, name := range candidates {
			omitted = appendToolOmission(omitted, ToolOmission{Name: name, Reason: "write_permission_disabled"})
		}
		return nil, omitted
	}
	allowed := map[string]bool{}
	for _, name := range prefs.ChatAllowedWriteTools {
		allowed[name] = true
	}
	out := []string{}
	for _, name := range candidates {
		if allowed[name] {
			out = appendUniqueToolNames(out, name)
		} else {
			omitted = appendToolOmission(omitted, ToolOmission{Name: name, Reason: "write_permission_disabled"})
		}
	}
	return out, omitted
}

func writeToolCandidatesForIntent(query string, intent AgentIntent) []string {
	candidates := []string{}
	q := strings.ToLower(query)
	switch intent.Operation {
	case IntentCreate:
		componentCreate := containsIntentEntity(intent.Entities, EntityGearPart)
		existingGearComponentCreate := componentCreate && referencesExistingGear(q)
		if containsIntentEntity(intent.Entities, EntityGear) && !existingGearComponentCreate {
			candidates = append(candidates, "create_gear")
		}
		if componentCreate {
			candidates = append(candidates, "add_gear_component")
		}
		if containsIntentEntity(intent.Entities, EntityGearSet) {
			candidates = append(candidates, "create_chain_rotation")
		}
		if containsIntentEntity(intent.Entities, EntityTag) {
			candidates = append(candidates, "create_tag")
		}
		if containsIntentEntity(intent.Entities, EntityRule) {
			candidates = append(candidates, "create_automation_rule")
		}
	case IntentUpdate:
		if containsIntentEntity(intent.Entities, EntityActivity) {
			candidates = append(candidates, "rename_activity", "update_activity_metadata", "set_activity_gear", "tag_activity")
		}
		if containsIntentEntity(intent.Entities, EntityGear) {
			candidates = append(candidates, "update_gear_metadata")
		}
		if containsIntentEntity(intent.Entities, EntityGearPart) {
			candidates = append(candidates, "add_gear_component", "update_gear_component_status", "record_gear_service_event")
		}
		if containsIntentEntity(intent.Entities, EntityTag) {
			candidates = append(candidates, "tag_activity", "update_tag")
		}
		if containsIntentEntity(intent.Entities, EntityRule) {
			candidates = append(candidates, "update_automation_rule")
		}
		if containsIntentEntity(intent.Entities, EntityRoute) {
			candidates = append(candidates, "create_routing_profile_variant")
		}
	case IntentBulkUpdate:
		candidates = append(candidates, "preview_activity_selection")
		if containsIntentEntity(intent.Entities, EntityTag) {
			candidates = append(candidates, "bulk_tag_activities")
		}
		if containsIntentEntity(intent.Entities, EntityActivity) && !containsIntentEntity(intent.Entities, EntityTag) {
			candidates = append(candidates, "update_activity_metadata")
		}
	case IntentDelete:
		if containsIntentEntity(intent.Entities, EntityTag) {
			candidates = append(candidates, "delete_tag")
		}
		if containsIntentEntity(intent.Entities, EntityRule) {
			candidates = append(candidates, "delete_automation_rule")
		}
	case IntentSave:
		candidates = append(candidates, "save_planned_route")
	}
	return appendUniqueToolNames(nil, candidates...)
}

func referencesExistingGear(q string) bool {
	return containsAny(q,
		"meinem ", "meiner ", "meinen ", "meines ", "mein ", "meine ",
		"bestehenden", "vorhandenen", "an meinem", "zu meinem",
	) && !containsAny(q, "neues fahrrad", "neuen fahrrad", "neues rad", "neuen rad", "neues test", "neues rennrad", "neuen rennrad")
}

func mergeToolPlanNames(required, optional []string, limit int) ([]string, []string) {
	if limit <= 0 {
		limit = len(required) + len(optional)
	}
	out := []string{}
	omitted := []string{}
	for _, name := range required {
		if !containsToolName(out, name) {
			out = append(out, name)
		}
	}
	for _, name := range optional {
		if containsToolName(out, name) {
			continue
		}
		if len(out) < limit {
			out = append(out, name)
		} else {
			omitted = append(omitted, name)
		}
	}
	return out, omitted
}

func toolLimitOmissions(names []string) []ToolOmission {
	out := make([]ToolOmission, 0, len(names))
	for _, name := range names {
		out = appendToolOmission(out, ToolOmission{Name: name, Reason: "profile_tool_limit"})
	}
	return out
}

func appendToolOmission(values []ToolOmission, value ToolOmission) []ToolOmission {
	if value.Name == "" {
		return values
	}
	for _, existing := range values {
		if existing.Name == value.Name && existing.Reason == value.Reason {
			return values
		}
	}
	return append(values, value)
}

func containsIntentEntity(values []IntentEntity, want IntentEntity) bool {
	return slices.Contains(values, want)
}

func profileAllowsSQL(profile *ModelProfile) bool {
	return profile != nil && profile.EnableSQLAgent
}

func isExplicitSQLQuestion(q string) bool {
	return containsAny(q, "sql", "datenbank", "database", "tabelle", "schema", "join")
}

func isCapabilityQuestion(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	return containsAny(q,
		"welche tools",
		"welche funktionen",
		"was kannst du",
		"was kann der agent",
		"what tools",
		"what can you do",
		"available tools",
		"capabilities",
	)
}

func isWeatherCapabilityQuestion(query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	return containsAny(q, "wetterdaten", "wettertools", "weather tools", "weather capabilities") &&
		containsAny(q, "kannst du", "können", "koennen", "welche", "what")
}
