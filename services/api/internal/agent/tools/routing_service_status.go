package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/geosearch"
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
		result["geocoding"] = geocodingStatus(deps, userID)
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

func geocodingStatus(deps *Deps, userID string) map[string]any {
	externalStatus := "disabled"
	if geocodePlacePolicy(deps) == "allowed" {
		externalStatus = "unconfigured"
		if geocodePlaceExternal(deps) != nil {
			externalStatus = "configured"
		}
	}
	quota := geocodePlaceQuota(deps).Check(userID)
	return map[string]any{
		"local_search_status":          "available_when_osm_imported",
		"external_status":              externalStatus,
		"privacy_policy":               string(geocodePlacePolicy(deps)),
		"external_read_consent_status": "policy_owned_by_tool",
		"rate_limit":                   geosearchQuotaStatusMap(quota),
	}
}

func weatherStatus() map[string]any {
	cfg := liveWeatherConfigFromEnv()
	liveStatus := "disabled"
	switch cfg.Policy {
	case "allowed":
		liveStatus = "unconfigured"
		if cfg.ProviderEnabled {
			liveStatus = "configured"
		}
	case "manual_confirm":
		liveStatus = "manual_confirmation_not_supported"
	case "blocked":
		liveStatus = "skipped_by_privacy_policy"
	}
	return map[string]any{
		"stored_read_status":           "available_for_owned_activity_weather",
		"live_status":                  liveStatus,
		"privacy_policy":               cfg.Policy,
		"external_read_consent_status": "policy_owned_by_tool",
		"rate_limit":                   liveWeatherStatusRateLimit(cfg),
	}
}

func geosearchQuotaStatusMap(quota geosearch.QuotaState) map[string]any {
	return map[string]any{
		"status":          quota.Status,
		"limit_per_hour":  quota.LimitPerHour,
		"remaining":       quota.Remaining,
		"reset_at":        quota.ResetAt,
		"blocking_reason": quota.BlockingReason,
	}
}

func liveWeatherStatusRateLimit(cfg liveWeatherConfig) map[string]any {
	if cfg.LimitPerHour <= 0 {
		return map[string]any{"status": "unlimited", "limit_per_hour": nil, "remaining": nil, "reset_at": nil}
	}
	return map[string]any{"status": "available", "limit_per_hour": cfg.LimitPerHour, "remaining": cfg.LimitPerHour, "reset_at": nil}
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
	badrouterStatus := "unconfigured_in_agent_runtime"
	routeSuggestionStatus := "not_injected"
	if deps != nil && deps.RouteGoalSuggester != nil {
		badrouterStatus = "configured_via_route_suggester"
		routeSuggestionStatus = "configured_round_trip_only"
	}
	return map[string]any{
		"brouter_status":          brouterStatus,
		"badrouter_status":        badrouterStatus,
		"route_suggestion_status": routeSuggestionStatus,
		"profile_support_summary": "built-in and DB profile listing is available through list_routing_profiles; route-suggestion supports round-trip variants when injected",
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

func routingCoverageProfileStatus(ctx context.Context, deps *Deps, userID, profileKey string) map[string]any {
	if strings.TrimSpace(profileKey) == "" {
		return map[string]any{"valid": true, "status": "not_supplied"}
	}
	if routeSuggestionProfileSupported(profileKey) {
		return map[string]any{"valid": true, "status": "ok", "key": profileKey, "engine": "badrouter", "base_profile": profileKey}
	}
	profile, err := resolveRoutingProfileForPreview(ctx, deps, userID, profileKey)
	if err != nil {
		return map[string]any{"valid": false, "status": "invalid", "error": err.Error()}
	}
	return map[string]any{
		"valid":        true,
		"status":       "ok",
		"key":          profile.Key,
		"engine":       profile.Engine,
		"base_profile": profile.BaseProfile,
	}
}

func buildRouteGoalSuggestionRequest(ctx context.Context, deps *Deps, userID string, start routeResolverRef, args suggestRouteGoalArgs) (RouteGoalSuggestionRequest, error) {
	profile, err := routeGoalSuggestionProfile(ctx, deps, userID, args.ProfileKey)
	if err != nil {
		return RouteGoalSuggestionRequest{}, err
	}
	return RouteGoalSuggestionRequest{
		StartLat:             start.Lat,
		StartLng:             start.Lng,
		RoundTrip:            valueOrDefaultBool(args.RoundTrip, true),
		TargetDistanceM:      floatPtrValue(args.TargetDistanceM),
		TargetAscentM:        args.TargetAscentM,
		AscentTolerancePct:   args.AscentTolerancePct,
		SportType:            strings.TrimSpace(args.SportQuery),
		Profile:              profile,
		SurfaceTendency:      strings.TrimSpace(args.SurfaceTendency),
		MaxSacScale:          difficultyScaleInt(args.Difficulty, "max_sac_scale", "T"),
		MaxMtbScale:          difficultyScaleInt(args.Difficulty, "max_mtb_scale", "S"),
		LandscapePrefs:       routeGoalLandscapePrefs(args.LandscapePrefs),
		Modifiers:            routeGoalModifiers(args.Modifiers),
		MaxVariants:          args.MaxVariants,
		ProfileSportOverride: true,
	}, nil
}

func routeGoalSuggestionProfile(ctx context.Context, deps *Deps, userID, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("profile_key is required; call list_routing_profiles first")
	}
	profile, err := resolveRoutingProfileForPreview(ctx, deps, userID, key)
	if err != nil {
		return key, nil
	}
	if profile.Engine == "badrouter" && strings.TrimSpace(profile.Label) != "" {
		return profile.Label, nil
	}
	if strings.TrimSpace(profile.BaseProfile) != "" {
		return profile.BaseProfile, nil
	}
	return key, nil
}

func floatPtrValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func difficultyScaleInt(values map[string]any, key, prefix string) *int {
	if values == nil {
		return nil
	}
	raw, ok := values[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		n := v
		return &n
	case string:
		text := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(v), prefix))
		n, err := strconv.Atoi(text)
		if err != nil {
			return nil
		}
		return &n
	default:
		return nil
	}
}

func routeGoalLandscapePrefs(values map[string]any) map[string]int {
	out := map[string]int{}
	for _, key := range []string{"forest", "water", "protected", "low_noise", "low_traffic"} {
		if n, ok := intFromAny(values[key]); ok {
			if n < 0 {
				n = 0
			}
			if n > 10 {
				n = 10
			}
			out[key] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func routeGoalModifiers(values map[string]any) RouteGoalSuggestionModifiers {
	return RouteGoalSuggestionModifiers{
		IsElectric: boolPtrFromAny(values["is_electric"]),
		IsIndoor:   boolPtrFromAny(values["is_indoor"]),
		IsVirtual:  boolPtrFromAny(values["is_virtual"]),
		IsCommute:  boolPtrFromAny(values["is_commute"]),
	}
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}

func boolPtrFromAny(value any) *bool {
	v, ok := value.(bool)
	if !ok {
		return nil
	}
	return &v
}

func routeGoalAppliedConstraints(req RouteGoalSuggestionRequest) []string {
	out := []string{"round_trip"}
	if req.TargetDistanceM > 0 {
		out = append(out, "target_distance_m")
	}
	if req.TargetAscentM != nil {
		out = append(out, "target_ascent_m")
	}
	if req.MaxSacScale != nil {
		out = append(out, "max_sac_scale")
	}
	if req.MaxMtbScale != nil {
		out = append(out, "max_mtb_scale")
	}
	if len(req.LandscapePrefs) > 0 {
		out = append(out, "landscape_prefs")
	}
	if req.SurfaceTendency != "" {
		out = append(out, "surface_tendency")
	}
	return out
}

func suggestRouteGoalUnsupportedConstraints(args suggestRouteGoalArgs) []string {
	out := []string{}
	if args.End != nil || (args.RoundTrip != nil && !*args.RoundTrip) {
		out = append(out, "fixed_end_goal")
	}
	if strings.TrimSpace(args.ElevationMode) != "" {
		out = append(out, "elevation_mode")
	}
	if len(args.Preferences) > 0 {
		out = append(out, "preferences")
	}
	if len(args.TargetFeatureRefs) > 0 {
		out = append(out, "target_feature_refs")
	}
	if len(args.NearFeatureRefs) > 0 {
		out = append(out, "near_feature_refs")
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
