package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type getAgentServiceStatusArgs struct {
	Services   []string     `json:"services,omitempty"`
	Points     []routePoint `json:"points,omitempty"`
	BBox       *routeBBox   `json:"bbox,omitempty"`
	ProfileKey string       `json:"profile_key,omitempty"`
}

func executeGetAgentServiceStatus(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getAgentServiceStatusArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("decode get_agent_service_status args: %w", err)
		}
	}
	serviceSet := selectedStatusServices(args.Services)
	result := map[string]any{
		"blocking_reasons":      []string{},
		"recommended_next_step": "resolve locations, analyze route points, then check coverage before planning",
		"long_distance_policy":  longDistancePolicyStatus(),
		"difficulty_support":    difficultySupportStatus(args.ProfileKey),
	}
	if serviceSet["geocoding"] {
		result["geocoding"] = geocodingStatus()
	}
	if serviceSet["weather"] {
		result["weather"] = weatherStatus()
	}
	if serviceSet["routing"] {
		result["routing"] = routingStatus(deps)
	}
	if serviceSet["coverage"] {
		coverage, reasons := statusCoverage(ctx, deps, userID, args)
		result["coverage"] = coverage
		if len(reasons) > 0 {
			result["blocking_reasons"] = reasons
			result["recommended_next_step"] = "install or build missing local route coverage before promising route generation"
		}
	}
	return result, nil
}

func selectedStatusServices(values []string) map[string]bool {
	if len(values) == 0 {
		return map[string]bool{"geocoding": true, "weather": true, "routing": true, "coverage": true}
	}
	out := map[string]bool{}
	for _, value := range values {
		switch value {
		case "geocoding", "weather", "routing", "coverage":
			out[value] = true
		}
	}
	return out
}

func geocodingStatus() map[string]any {
	return map[string]any{
		"local_search_status":          "available_when_osm_imported",
		"external_status":              "unconfigured",
		"privacy_policy":               "not_wired",
		"external_read_consent_status": "not_wired",
		"rate_limit":                   map[string]any{"status": "unavailable", "limit_per_hour": nil, "remaining": nil, "reset_at": nil},
	}
}

func weatherStatus() map[string]any {
	return map[string]any{
		"stored_read_status":           "available_for_owned_activity_weather",
		"live_status":                  "unconfigured",
		"privacy_policy":               "not_wired",
		"external_read_consent_status": "not_wired",
		"rate_limit":                   map[string]any{"status": "unavailable", "limit_per_hour": nil, "remaining": nil, "reset_at": nil},
	}
}

func routingStatus(deps *Deps) map[string]any {
	brouterURL := ""
	if deps != nil {
		brouterURL = strings.TrimSpace(deps.BRouterURL)
	}
	if brouterURL == "" {
		brouterURL = strings.TrimSpace(os.Getenv("BROUTER_URL"))
	}
	brouterStatus := "unconfigured"
	if brouterURL != "" {
		brouterStatus = "configured"
	}
	return map[string]any{
		"brouter_status":          brouterStatus,
		"badrouter_status":        "unconfigured_in_agent_runtime",
		"profile_support_summary": "built-in and DB profile listing is available through list_routing_profiles; route-suggestion execution is not injected into agent tools yet",
	}
}

func longDistancePolicyStatus() map[string]any {
	return map[string]any{
		"max_direct_leg_m":         float64(maxDirectRouteLegM()),
		"waypoint_insertion":       "user_supplied_only",
		"blocks_long_direct_route": true,
		"model_override_allowed":   false,
	}
}

func difficultySupportStatus(profileKey string) map[string]any {
	return map[string]any{
		"profile_key":           profileKey,
		"sac":                   map[string]any{"status": "supported_when_tagged_coverage_exists", "backend_field": "max_sac_scale"},
		"mtb":                   map[string]any{"status": "supported_when_tagged_coverage_exists", "backend_field": "max_mtb_scale"},
		"via_ferrata":           map[string]any{"status": "unsupported_in_agent_goal_request", "backend_field": nil},
		"climbing":              map[string]any{"status": "metadata_only_not_route_optimization"},
		"coverage_verification": "area_dependent_file_presence_only",
	}
}

func statusCoverage(ctx context.Context, deps *Deps, userID string, args getAgentServiceStatusArgs) (map[string]any, []string) {
	if args.BBox == nil && len(args.Points) == 0 {
		return map[string]any{"status": "not_checked", "reason": "points_or_bbox_not_supplied"}, nil
	}
	raw, err := json.Marshal(getRoutingCoverageArgs{Points: args.Points, BBox: args.BBox, ProfileKey: args.ProfileKey, Intent: "diagnostics"})
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}, []string{"coverage_status_failed"}
	}
	coverage, err := executeGetRoutingCoverage(ctx, deps, userID, raw)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error()}, []string{"coverage_status_failed"}
	}
	m, ok := coverage.(map[string]any)
	if !ok {
		return map[string]any{"status": "error", "error": "unexpected coverage result"}, []string{"coverage_status_failed"}
	}
	reasons := []string{}
	if reason := stringValue(m["blocking_reason"]); reason != "" {
		reasons = append(reasons, reason)
	}
	return m, reasons
}

func rawHasJSONKey(raw json.RawMessage, key string) bool {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return false
	}
	_, ok := obj[key]
	return ok
}

func suggestRouteGoalUnsupportedConstraints(args suggestRouteGoalArgs) []string {
	out := []string{}
	if args.End != nil || (args.RoundTrip != nil && !*args.RoundTrip) {
		out = append(out, "fixed_end_goal")
	}
	if len(args.AlongFeatureRefs) > 0 {
		out = append(out, "along_feature_refs")
	}
	if len(args.AvoidFeatureRefs) > 0 {
		out = append(out, "avoid_feature_refs")
	}
	if args.Difficulty != nil {
		if _, ok := args.Difficulty["max_via_ferrata"]; ok {
			out = append(out, "max_via_ferrata")
		}
		if _, ok := args.Difficulty["climbing_grade"]; ok {
			out = append(out, "climbing_grade")
		}
	}
	return out
}

func suggestRouteGoalUnsupportedReason(status string) string {
	if status == "unsupported_constraint" {
		return "one or more requested constraints are not implemented by the route-suggestion backend exposed to agent tools"
	}
	return "round-trip route suggestions are not service-backed in the agent runtime yet; use plan_route_between for fixed A-B previews and keep unsupported constraints explicit"
}

func suggestRouteGoalLongRouteBlock(ctx context.Context, deps *Deps, userID string, args suggestRouteGoalArgs) (map[string]any, bool) {
	raw, err := json.Marshal(analyzeRoutePointsArgs{Points: []routePoint{args.Start, *args.End}, Intent: "direct_route"})
	if err != nil {
		return nil, false
	}
	result, err := executeAnalyzeRoutePoints(ctx, deps, userID, raw)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error(), "variants": []any{}}, true
	}
	analysis := result.(map[string]any)
	if analysis["long_route_status"] != "requires_waypoints" {
		return nil, false
	}
	return map[string]any{
		"status":                  "unsupported_constraint",
		"unsupported_constraints": []string{"fixed_end_goal", "max_direct_leg_exceeded"},
		"analysis":                analysis,
		"variants":                []any{},
		"applied_constraints":     []string{},
		"post_validation":         map[string]any{"status": "not_run"},
	}, true
}

func addStrings(dst map[string]bool, raw any) {
	for _, value := range anyStringSlice(raw) {
		dst[value] = true
	}
}

func anyStringSlice(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func sortedSetStrings(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
