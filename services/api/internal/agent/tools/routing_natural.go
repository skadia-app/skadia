package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/osm"
	graphpkg "github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/google/uuid"
)

const maxRouteResolverCandidates = 20
const routeResolverRefTTL = 30 * time.Minute

type routePoint struct {
	Ref string   `json:"ref,omitempty"`
	Lat *float64 `json:"lat,omitempty"`
	Lng *float64 `json:"lng,omitempty"`
}

type resolveRoutePlaceArgs struct {
	Query string      `json:"query"`
	Kind  string      `json:"kind,omitempty"`
	Near  *routePoint `json:"near,omitempty"`
	BBox  *routeBBox  `json:"bbox,omitempty"`
	Limit *int        `json:"limit,omitempty"`
}

type resolveRouteFeatureArgs struct {
	Query       string      `json:"query"`
	FeatureKind string      `json:"feature_kind,omitempty"`
	Near        *routePoint `json:"near,omitempty"`
	BBox        *routeBBox  `json:"bbox,omitempty"`
	Relation    string      `json:"relation,omitempty"`
	Limit       *int        `json:"limit,omitempty"`
}

type routeBBox struct {
	MinLat float64 `json:"min_lat"`
	MinLng float64 `json:"min_lng"`
	MaxLat float64 `json:"max_lat"`
	MaxLng float64 `json:"max_lng"`
}

type getRoutingCoverageArgs struct {
	PlaceRefs   []string     `json:"place_refs,omitempty"`
	FeatureRefs []string     `json:"feature_refs,omitempty"`
	Points      []routePoint `json:"points,omitempty"`
	BBox        *routeBBox   `json:"bbox,omitempty"`
	ProfileKey  string       `json:"profile_key,omitempty"`
	Intent      string       `json:"intent,omitempty"`
}

type planRouteBetweenArgs struct {
	From          routePoint     `json:"from"`
	To            routePoint     `json:"to"`
	Via           []routePoint   `json:"via,omitempty"`
	ProfileKey    string         `json:"profile_key"`
	ProfileParams map[string]any `json:"profile_params,omitempty"`
	Constraints   map[string]any `json:"constraints,omitempty"`
	NameHint      string         `json:"name_hint,omitempty"`
}

type suggestRouteGoalArgs struct {
	Start              routePoint     `json:"start"`
	End                *routePoint    `json:"end,omitempty"`
	RoundTrip          *bool          `json:"round_trip,omitempty"`
	TargetDistanceM    *float64       `json:"target_distance_m,omitempty"`
	TargetAscentM      *float64       `json:"target_ascent_m,omitempty"`
	AscentTolerancePct *float64       `json:"ascent_tolerance_pct,omitempty"`
	ElevationMode      string         `json:"elevation_mode,omitempty"`
	SportQuery         string         `json:"sport_query,omitempty"`
	ProfileKey         string         `json:"profile_key,omitempty"`
	Preferences        map[string]any `json:"preferences,omitempty"`
	Difficulty         map[string]any `json:"difficulty,omitempty"`
	AlongFeatureRefs   []string       `json:"along_feature_refs,omitempty"`
	AvoidFeatureRefs   []string       `json:"avoid_feature_refs,omitempty"`
	NearFeatureRefs    []string       `json:"near_feature_refs,omitempty"`
	TargetFeatureRefs  []string       `json:"target_feature_refs,omitempty"`
	LandscapePrefs     map[string]any `json:"landscape_prefs,omitempty"`
	SurfaceTendency    string         `json:"surface_tendency,omitempty"`
	Modifiers          map[string]any `json:"modifiers,omitempty"`
	MaxVariants        *int           `json:"max_variants,omitempty"`
}

type routeResolverRef struct {
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	SourceID    string    `json:"source_id,omitempty"`
	SourceOSMID string    `json:"source_osm_id,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	Name        string    `json:"name"`
	Lat         float64   `json:"lat"`
	Lng         float64   `json:"lng"`
	CreatedAt   time.Time `json:"created_at"`
}

var routeResolverRefCache sync.Map

func ResolveRoutePlaceTool() *Tool {
	return &Tool{
		Name:        "resolve_route_place",
		Description: "Resolve a named place, street, or POI from Skadia's imported map data into candidate route points. Use before asking the user for coordinates.",
		InputSchema: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":2},"kind":{"type":"string","enum":["all","place","street","poi"]},"near":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"bbox":{"type":"object","required":["min_lat","min_lng","max_lat","max_lng"],"properties":{"min_lat":{"type":"number"},"min_lng":{"type":"number"},"max_lat":{"type":"number"},"max_lng":{"type":"number"}},"additionalProperties":false},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: false,
		ExecuteRead: executeResolveRoutePlace,
		RequiredCapabilities: []string{
			capabilities.KeyLocalPOISearch,
		},
	}
}

func ResolveUserLocationTool() *Tool {
	return &Tool{
		Name:        "resolve_user_location",
		Description: "Resolve the current user's saved location tags into route-point refs with coordinates. Only user-owned location tags are returned.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"category":{"type":"string"},"near":{"type":"object","properties":{"lat":{"type":"number"},"lng":{"type":"number"},"ref":{"type":"string"}},"additionalProperties":false},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: true,
		ExecuteRead: executeResolveUserLocation,
	}
}

func ResolveRouteFeatureTool() *Tool {
	return &Tool{
		Name:        "resolve_route_feature",
		Description: "Resolve lakes, rivers, forests, summits, passes, trails, or shared OSM POIs for route constraints such as target, pass-near, along, or avoid.",
		InputSchema: json.RawMessage(`{"type":"object","required":["query"],"properties":{"query":{"type":"string","minLength":2},"feature_kind":{"type":"string","enum":["all","water","river","lake","forest","summit","pass","trail","poi"]},"near":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"bbox":{"type":"object","required":["min_lat","min_lng","max_lat","max_lng"],"properties":{"min_lat":{"type":"number"},"min_lng":{"type":"number"},"max_lat":{"type":"number"},"max_lng":{"type":"number"}},"additionalProperties":false},"relation":{"type":"string","enum":["target","pass_near","along","avoid"]},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: false,
		ExecuteRead: executeResolveRouteFeature,
		RequiredCapabilities: []string{
			capabilities.KeyLocalPOISearch,
		},
	}
}

func GetRoutingCoverageTool() *Tool {
	return &Tool{
		Name:             "get_routing_coverage",
		Description:      "Check imported map region, BRouter tile, DEM cell, and semantic coverage hints for route points before promising a route.",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"place_refs":{"type":"array","items":{"type":"string"}},"feature_refs":{"type":"array","items":{"type":"string"}},"points":{"type":"array","items":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false}},"bbox":{"type":"object","required":["min_lat","min_lng","max_lat","max_lng"],"properties":{"min_lat":{"type":"number"},"min_lng":{"type":"number"},"max_lat":{"type":"number"},"max_lng":{"type":"number"}},"additionalProperties":false},"profile_key":{"type":"string"},"intent":{"type":"string","enum":["resolve_only","direct","round_trip","goal","diagnostics"]}},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 120,
		Timeout:          5 * time.Second,
		IsUserOwned:      false,
		ExecuteRead:      executeGetRoutingCoverage,
		RequiredCapabilities: []string{
			capabilities.KeyCoverage,
		},
	}
}

func AnalyzeRoutePointsTool() *Tool {
	return &Tool{
		Name:             "analyze_route_points",
		Description:      "Resolve route-point refs or lat/lng points and compute straight-line route diagnostics, required local tiles, and long-leg waypoint requirements before expensive routing.",
		InputSchema:      json.RawMessage(`{"type":"object","required":["points"],"properties":{"points":{"type":"array","minItems":2,"items":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false}},"intent":{"type":"string","enum":["distance_only","direct_route","goal_route"]}},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 120,
		Timeout:          5 * time.Second,
		IsUserOwned:      true,
		ExecuteRead:      executeAnalyzeRoutePoints,
	}
}

func GetAgentServiceStatusTool() *Tool {
	return &Tool{
		Name:             "get_agent_service_status",
		Description:      "Return cheap status for agent geocoding, weather, routing, coverage, long-distance policy, and difficulty support before promising route work.",
		InputSchema:      json.RawMessage(`{"type":"object","properties":{"services":{"type":"array","items":{"type":"string","enum":["geocoding","weather","routing","coverage"]}},"points":{"type":"array","items":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false}},"bbox":{"type":"object","required":["min_lat","min_lng","max_lat","max_lng"],"properties":{"min_lat":{"type":"number"},"min_lng":{"type":"number"},"max_lat":{"type":"number"},"max_lng":{"type":"number"}},"additionalProperties":false},"profile_key":{"type":"string"}},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 120,
		Timeout:          5 * time.Second,
		IsUserOwned:      true,
		Capability:       &ToolCapability{Area: "routing", PlannerSelectable: true},
		ExecuteRead:      executeGetAgentServiceStatus,
	}
}

func PlanRouteBetweenTool() *Tool {
	return &Tool{
		Name:        "plan_route_between",
		Description: "Preview an A-to-B route using resolver refs or named lat/lng objects. The profile_key must come from list_routing_profiles.",
		InputSchema: json.RawMessage(`{"type":"object","required":["from","to","profile_key"],"properties":{"from":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"to":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"via":{"type":"array","items":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false}},"profile_key":{"type":"string"},"profile_params":{"type":"object","additionalProperties":{"type":["string","number","boolean"]}},"constraints":{"type":"object"},"name_hint":{"type":"string"}},"additionalProperties":false}`),
		Permission:  PermissionRead,
		IsUserOwned: false,
		ExecuteRead: executePlanRouteBetween,
		RequiredCapabilities: []string{
			capabilities.KeyRouting,
			capabilities.KeyBRouter,
		},
	}
}

func SuggestRouteGoalTool() *Tool {
	return &Tool{
		Name:             "suggest_route_goal",
		Description:      "Explain whether Skadia can suggest a goal-based or round-trip route from a start point. Returns structured unsupported details when route-suggestion coverage is missing.",
		InputSchema:      json.RawMessage(`{"type":"object","required":["start"],"properties":{"start":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"end":{"type":"object","properties":{"ref":{"type":"string"},"lat":{"type":"number"},"lng":{"type":"number"}},"additionalProperties":false},"round_trip":{"type":"boolean"},"target_distance_m":{"type":"number","minimum":1},"target_ascent_m":{"type":"number","minimum":0},"ascent_tolerance_pct":{"type":"number","minimum":0},"elevation_mode":{"type":"string","enum":["flat","moderate","climby","avoid_steep","custom"]},"sport_query":{"type":"string"},"profile_key":{"type":"string"},"preferences":{"type":"object"},"difficulty":{"type":"object","properties":{"max_sac_scale":{"oneOf":[{"type":"string","enum":["T1","T2","T3","T4","T5","T6"]},{"type":"integer","minimum":0,"maximum":6}]},"max_mtb_scale":{"oneOf":[{"type":"string","enum":["S0","S1","S2","S3","S4","S5"]},{"type":"integer","minimum":0,"maximum":5}]},"max_via_ferrata":{"oneOf":[{"type":"string","enum":["K1","K2","K3","K4","K5","K6"]},{"type":"integer","minimum":1,"maximum":6}]},"climbing_grade":{"type":"string"},"difficulty_intent":{"type":"string","enum":["avoid","allow","seek"]}},"additionalProperties":false},"target_feature_refs":{"type":"array","items":{"type":"string"}},"near_feature_refs":{"type":"array","items":{"type":"string"}},"along_feature_refs":{"type":"array","items":{"type":"string"}},"avoid_feature_refs":{"type":"array","items":{"type":"string"}},"landscape_prefs":{"type":"object"},"surface_tendency":{"type":"string"},"modifiers":{"type":"object"},"max_variants":{"type":"integer","minimum":1,"maximum":10}},"additionalProperties":false}`),
		Permission:       PermissionRead,
		RateLimitPerHour: 30,
		Timeout:          75 * time.Second,
		IsUserOwned:      false,
		ExecuteRead:      executeSuggestRouteGoal,
		RequiredCapabilities: []string{
			capabilities.KeyRouteSuggestion,
		},
	}
}

func executeResolveRoutePlace(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args resolveRoutePlaceArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode resolve_route_place args: %w", err)
	}
	kind := normalizedPlaceKind(args.Kind)
	query := strings.TrimSpace(args.Query)
	if len(query) < 2 {
		return nil, fmt.Errorf("query must be at least 2 characters")
	}
	limit := clampIntPtr(args.Limit, 5, 1, maxRouteResolverCandidates)
	sql, params := buildResolveRoutePlaceSQL(query, kind, args.Near, args.BBox, limit)
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	candidates := addRefsToRouteCandidates(rows, "place")
	return map[string]any{
		"query":            query,
		"kind":             kind,
		"candidates":       candidates,
		"count":            len(candidates),
		"ambiguous":        len(candidates) > 1,
		"not_found_reason": notFoundReason(len(candidates), "imported map data has no matching place, street, or POI"),
	}, nil
}

func executeResolveRouteFeature(ctx context.Context, deps *Deps, _ string, raw json.RawMessage) (any, error) {
	var args resolveRouteFeatureArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode resolve_route_feature args: %w", err)
	}
	query := strings.TrimSpace(args.Query)
	if len(query) < 2 {
		return nil, fmt.Errorf("query must be at least 2 characters")
	}
	featureKind := normalizedFeatureKind(args.FeatureKind)
	relation := strings.TrimSpace(args.Relation)
	if relation == "" {
		relation = "target"
	}
	limit := clampIntPtr(args.Limit, 5, 1, maxRouteResolverCandidates)
	sql, params := buildResolveRouteFeatureSQL(query, featureKind, args.Near, args.BBox, limit)
	rows, err := queryRows(ctx, deps, sql, params...)
	if err != nil {
		return nil, err
	}
	features := addRefsToRouteCandidates(rows, "feature")
	return map[string]any{
		"query":            query,
		"feature_kind":     featureKind,
		"relation":         relation,
		"features":         features,
		"count":            len(features),
		"ambiguous":        len(features) > 1,
		"not_found_reason": notFoundReason(len(features), "imported map data has no matching route feature"),
	}, nil
}

func executeGetRoutingCoverage(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args getRoutingCoverageArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode get_routing_coverage args: %w", err)
	}
	points, invalidRefs := routeCoveragePointsForUser(args, userID)
	bbox := args.BBox
	if bbox == nil && len(points) > 0 {
		b := bboxForPoints(points)
		bbox = &b
	}
	regions, importedRegionVerified := regionsForBBox(ctx, deps, bbox)
	intent := defaultString(args.Intent, "diagnostics")
	requiredTiles := []string{}
	if bbox != nil {
		requiredTiles = osm.BRouterTileIDs([]osm.Region{{MinLon: bbox.MinLng, MinLat: bbox.MinLat, MaxLon: bbox.MaxLng, MaxLat: bbox.MaxLat}})
	}
	demCells := demCellsForBBox(bbox)
	segmentsDir, presentTiles, missingTiles := brouterSegmentPresence(deps, requiredTiles)
	demDir, presentDEMCells, missingDEMCells := demCellPresence(deps, demCells)
	graphTileIDs, graphTileListTruncated := graphTileIDsForBBox(bbox)
	routingTilesDir, presentGraphTiles, missingGraphTiles := badRouterTilePresence(deps, graphTileIDs)
	coverageBaseDir, presentLayers, missingLayers := coverageLayerPresence(deps, graphTileIDs)
	canResolve := bbox != nil && len(regions) > 0 && importedRegionVerified
	canPlan := bbox != nil && len(requiredTiles) > 0 && len(missingTiles) == 0
	badRouterAvailable := len(graphTileIDs) > 0 && len(missingGraphTiles) == 0 && !graphTileListTruncated
	canSuggestGoal := deps != nil && deps.RouteGoalSuggester != nil && badRouterAvailable
	profileStatus := routingCoverageProfileStatus(ctx, deps, userID, args.ProfileKey)
	blockingReason := ""
	nextAdminAction := ""
	if len(invalidRefs) > 0 {
		canResolve = false
		canPlan = false
		canSuggestGoal = false
		blockingReason = "invalid_or_expired_resolver_ref"
		nextAdminAction = "resolve the named place or feature again"
	} else if bbox == nil {
		blockingReason = "no_points_or_bbox"
		nextAdminAction = "call resolve_route_place or provide explicit lat/lng points"
	} else if !boolValue(profileStatus["valid"]) {
		canPlan = false
		canSuggestGoal = false
		blockingReason = "invalid_or_incompatible_profile"
		nextAdminAction = "call list_routing_profiles and choose a compatible profile_key"
	} else if intent == "direct" && len(missingTiles) > 0 {
		blockingReason = "missing_brouter_segments"
		nextAdminAction = "download or build the missing BRouter rd5 segment files"
	} else if intent == "goal" && deps != nil && deps.RouteGoalSuggester == nil {
		blockingReason = "route_suggester_unavailable"
		nextAdminAction = "start the API with routing graph support before asking for goal route suggestions"
	} else if intent == "goal" && len(missingGraphTiles) > 0 {
		blockingReason = "missing_badrouter_tiles"
		nextAdminAction = "build routing tiles for the requested area"
	} else if !canResolve && intent == "resolve_only" {
		blockingReason = "missing_imported_osm_region"
		nextAdminAction = "import OSM data for the requested region"
	}
	return map[string]any{
		"intent":            intent,
		"profile_key":       args.ProfileKey,
		"profile":           profileStatus,
		"points":            points,
		"invalid_refs":      invalidRefs,
		"bbox":              bbox,
		"regions":           regions,
		"place_search":      map[string]any{"available": canResolve, "source": "imported_osm", "verified": importedRegionVerified},
		"brouter":           map[string]any{"available": canPlan, "service_status": "configured_by_default_or_env", "segments_dir": segmentsDir, "required_rd5_tiles": requiredTiles, "present_rd5_tiles": presentTiles, "missing_rd5_tiles": missingTiles},
		"badrouter":         map[string]any{"available": badRouterAvailable, "tiles_dir": routingTilesDir, "required_tiles": graphTileLabels(graphTileIDs), "present_tiles": presentGraphTiles, "missing_tiles": missingGraphTiles, "tile_count": len(graphTileIDs), "tile_list_truncated": graphTileListTruncated, "verification": "file_presence_only"},
		"dem":               map[string]any{"dem_data_dir": demDir, "required_hgt_cells": demCells, "present_hgt_cells": presentDEMCells, "missing_hgt_cells": missingDEMCells, "status": demCoverageStatus(demCells, missingDEMCells)},
		"surface_highway":   map[string]any{"available": "unknown", "status": "depends on imported OSM way tags and route graph coverage"},
		"coverage_layers":   map[string]any{"available": len(presentLayers) > 0 && !graphTileListTruncated, "coverage_base_dir": coverageBaseDir, "required_layers": routeCoverageLayerNames(), "present_layers": presentLayers, "missing_layers": missingLayers, "tile_count": len(graphTileIDs), "tile_list_truncated": graphTileListTruncated, "verification": "file_presence_only"},
		"can_resolve_place": canResolve,
		"can_plan_direct":   canPlan,
		"can_suggest_goal":  canSuggestGoal,
		"blocking_reason":   blockingReason,
		"next_admin_action": nextAdminAction,
	}, nil
}

func executePlanRouteBetween(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args planRouteBetweenArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode plan_route_between args: %w", err)
	}
	points := []routePoint{args.From}
	points = append(points, args.Via...)
	points = append(points, args.To)
	waypoints := make([][]float64, 0, len(points))
	resolvedNames := make([]string, 0, len(points))
	for _, point := range points {
		resolved, err := resolveRoutePointForUser(point, userID)
		if err != nil {
			return map[string]any{"error": err.Error(), "needs_resolution": true, "fallback": true}, nil
		}
		waypoints = append(waypoints, []float64{resolved.Lng, resolved.Lat})
		resolvedNames = append(resolvedNames, resolved.Name)
	}
	if analysisResult, blocked := routeBetweenLongRouteBlock(ctx, deps, userID, points); blocked {
		return analysisResult, nil
	}
	if len(args.Constraints) > 0 {
		return map[string]any{
			"error":                  "semantic constraints are not yet supported by direct BRouter preview",
			"unsupported_constraint": true,
			"supported_path":         "resolve_route_place/resolve_route_feature + get_routing_coverage + plan_route_between without constraints",
			"fallback":               true,
		}, nil
	}
	if coverageResult, blocked := routeBetweenCoverageBlock(ctx, deps, userID, points, args.ProfileKey); blocked {
		return coverageResult, nil
	}
	payload, err := json.Marshal(planRouteArgs{Waypoints: waypoints, Profile: args.ProfileKey, ProfileParams: args.ProfileParams})
	if err != nil {
		return nil, err
	}
	result, err := executePlanRoute(ctx, deps, userID, payload)
	if err != nil {
		return nil, err
	}
	if m, ok := result.(map[string]any); ok {
		m["resolved_points"] = resolvedNames
		if args.NameHint != "" {
			m["name_hint"] = args.NameHint
		}
		m["agent_tool"] = "plan_route_between"
	}
	return result, nil
}

func routeBetweenLongRouteBlock(ctx context.Context, deps *Deps, userID string, points []routePoint) (map[string]any, bool) {
	raw, err := json.Marshal(analyzeRoutePointsArgs{Points: points, Intent: "direct_route"})
	if err != nil {
		return nil, false
	}
	result, err := executeAnalyzeRoutePoints(ctx, deps, userID, raw)
	if err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, true
	}
	analysis, ok := result.(map[string]any)
	if !ok || analysis["long_route_status"] != "requires_waypoints" {
		return nil, false
	}
	return map[string]any{
		"error":                    "direct route leg exceeds the configured max direct-leg distance",
		"fallback":                 true,
		"long_route_status":        "requires_waypoints",
		"analysis":                 analysis,
		"blocking_reason":          "max_direct_leg_exceeded",
		"recommended_next_step":    analysis["recommended_next_step"],
		"unsupported_constraint":   true,
		"requires_waypoints":       true,
		"suggested_waypoint_count": analysis["suggested_intermediate_point_count"],
	}, true
}

func executeSuggestRouteGoal(ctx context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	if rawHasJSONKey(raw, "target_duration_min") {
		return map[string]any{
			"status":                  "unsupported_constraint",
			"unsupported_constraints": []string{"target_duration_min"},
			"unsupported_reason":      "duration is not a supported route-planning input; use distance, ascent, difficulty, route type, or profile constraints",
			"clarification_question":  "Welche Distanz, welchen Anstieg, welche Schwierigkeit oder welches Profil soll die Route haben?",
			"variants":                []any{},
			"applied_constraints":     []string{},
			"post_validation":         map[string]any{"status": "not_run"},
		}, nil
	}
	var args suggestRouteGoalArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode suggest_route_goal args: %w", err)
	}
	start, err := resolveRoutePointForUser(args.Start, userID)
	if err != nil {
		return map[string]any{"error": err.Error(), "needs_resolution": true, "fallback": true}, nil
	}
	unsupported := suggestRouteGoalUnsupportedConstraints(args)
	status := "unsupported_backend_path"
	if len(unsupported) > 0 {
		status = "unsupported_constraint"
	}
	if args.End != nil {
		if analysis, blocked := suggestRouteGoalLongRouteBlock(ctx, deps, userID, args); blocked {
			return analysis, nil
		}
	}
	if status == "unsupported_constraint" || deps == nil || deps.RouteGoalSuggester == nil {
		return map[string]any{
			"start":                   start,
			"profile_key":             args.ProfileKey,
			"round_trip":              valueOrDefaultBool(args.RoundTrip, true),
			"target_distance_m":       args.TargetDistanceM,
			"target_ascent_m":         args.TargetAscentM,
			"status":                  status,
			"unsupported_constraints": unsupported,
			"applied_constraints":     []string{},
			"unsupported_reason":      suggestRouteGoalUnsupportedReason(status),
			"variants":                []any{},
			"coverage":                map[string]any{"status": "not_checked"},
			"post_validation":         map[string]any{"status": "not_run"},
		}, nil
	}
	if coverage, blocked := suggestRouteGoalCoverageBlock(ctx, deps, userID, args); blocked {
		return coverage, nil
	}
	req, err := buildRouteGoalSuggestionRequest(ctx, deps, userID, start, args)
	if err != nil {
		return map[string]any{"status": "error", "error": err.Error(), "variants": []any{}}, nil
	}
	resp, err := deps.RouteGoalSuggester(ctx, userID, req)
	if err != nil {
		return map[string]any{
			"start":               start,
			"profile_key":         args.ProfileKey,
			"round_trip":          req.RoundTrip,
			"target_distance_m":   args.TargetDistanceM,
			"target_ascent_m":     args.TargetAscentM,
			"status":              "backend_error",
			"error":               err.Error(),
			"variants":            []any{},
			"coverage":            map[string]any{"status": "not_checked"},
			"post_validation":     map[string]any{"status": "not_run"},
			"clarification_hint":  "route suggestion backend could not generate variants; report the backend status before asking for new constraints",
			"applied_constraints": []string{},
		}, nil
	}
	return map[string]any{
		"start":                   start,
		"profile_key":             args.ProfileKey,
		"round_trip":              req.RoundTrip,
		"target_distance_m":       args.TargetDistanceM,
		"target_ascent_m":         args.TargetAscentM,
		"status":                  "ok",
		"unsupported_constraints": unsupported,
		"applied_constraints":     routeGoalAppliedConstraints(req),
		"saveable":                false,
		"save_note":               "goal suggestions are read-only previews here; saving a route requires a separate route save action",
		"variants":                resp.Variants,
		"coverage":                map[string]any{"available": resp.CoverageAvailable, "partial_search": resp.PartialSearch},
		"post_validation":         map[string]any{"status": "backend_validated"},
	}, nil
}

func suggestRouteGoalCoverageBlock(ctx context.Context, deps *Deps, userID string, args suggestRouteGoalArgs) (map[string]any, bool) {
	raw, err := json.Marshal(getRoutingCoverageArgs{
		Points:     []routePoint{args.Start},
		ProfileKey: args.ProfileKey,
		Intent:     "goal",
	})
	if err != nil {
		return nil, false
	}
	result, err := executeGetRoutingCoverage(ctx, deps, userID, raw)
	if err != nil {
		return map[string]any{"status": "coverage_error", "error": err.Error(), "variants": []any{}}, true
	}
	coverage, ok := result.(map[string]any)
	if !ok || coverage["can_suggest_goal"] == true {
		return nil, false
	}
	return map[string]any{
		"status":            "coverage_blocked",
		"coverage":          coverage,
		"blocking_reason":   coverage["blocking_reason"],
		"next_admin_action": coverage["next_admin_action"],
		"variants":          []any{},
		"post_validation":   map[string]any{"status": "not_run"},
	}, true
}

func routeBetweenCoverageBlock(ctx context.Context, deps *Deps, userID string, points []routePoint, profileKey string) (map[string]any, bool) {
	raw, err := json.Marshal(getRoutingCoverageArgs{
		Points:     points,
		ProfileKey: profileKey,
		Intent:     "direct",
	})
	if err != nil {
		return nil, false
	}
	result, err := executeGetRoutingCoverage(ctx, deps, userID, raw)
	if err != nil {
		return map[string]any{"error": err.Error(), "fallback": true}, true
	}
	coverage, ok := result.(map[string]any)
	if !ok {
		return nil, false
	}
	if coverage["can_plan_direct"] == true {
		return nil, false
	}
	return map[string]any{
		"error":             "routing coverage is incomplete for these points",
		"fallback":          true,
		"coverage":          coverage,
		"blocking_reason":   coverage["blocking_reason"],
		"next_admin_action": coverage["next_admin_action"],
	}, true
}

func buildResolveRoutePlaceSQL(query, kind string, near *routePoint, bbox *routeBBox, limit int) (string, []any) {
	params := []any{query}
	nearClause, nearOrder := routeNearSQL(near, &params)
	bboxClause := routeBBoxSQL(bbox, &params, "geom")
	limitParam := addParam(&params, limit)

	selects := []string{}
	if kind == "all" || kind == "street" {
		selects = append(selects, fmt.Sprintf(`
SELECT 'street' AS kind, name, highway AS category, highway AS subtype,
       id::text AS source_id, osm_id::text AS source_osm_id,
       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
       similarity(name, $1) AS similarity, %s AS distance_m, 'osm_ways' AS source,
       NULL::float8 AS ele_m, 'line' AS geometry_kind,
       jsonb_build_object('min_lat', ST_YMin(ST_Envelope(geom)), 'min_lng', ST_XMin(ST_Envelope(geom)), 'max_lat', ST_YMax(ST_Envelope(geom)), 'max_lng', ST_XMax(ST_Envelope(geom))) AS bbox,
       ST_Length(geom::geography) AS length_m, NULL::float8 AS area_m
FROM osm_ways
WHERE name %% $1 AND name IS NOT NULL%s`, nearOrder.distanceExpr, bboxClause))
	}
	if kind == "all" || kind == "place" || kind == "poi" {
		categoryFilter := ""
		if kind == "place" {
			categoryFilter = " AND category IN ('settlement','summit','pass')"
		}
		selects = append(selects, fmt.Sprintf(`
SELECT category AS kind, name, category, subtype,
       id::text AS source_id, osm_id::text AS source_osm_id,
       ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
       similarity(name, $1) AS similarity, %s AS distance_m, 'osm_pois' AS source,
       ele AS ele_m,
       CASE WHEN GeometryType(geom) IN ('LINESTRING','MULTILINESTRING') THEN 'line'
            WHEN GeometryType(geom) IN ('POLYGON','MULTIPOLYGON') THEN 'polygon'
            ELSE 'point' END AS geometry_kind,
       jsonb_build_object('min_lat', ST_YMin(ST_Envelope(geom)), 'min_lng', ST_XMin(ST_Envelope(geom)), 'max_lat', ST_YMax(ST_Envelope(geom)), 'max_lng', ST_XMax(ST_Envelope(geom))) AS bbox,
       CASE WHEN GeometryType(geom) IN ('LINESTRING','MULTILINESTRING') THEN ST_Length(geom::geography) ELSE NULL::float8 END AS length_m,
       CASE WHEN GeometryType(geom) IN ('POLYGON','MULTIPOLYGON') THEN ST_Area(geom::geography) ELSE NULL::float8 END AS area_m
FROM osm_pois
WHERE name %% $1 AND name IS NOT NULL%s%s`, nearOrder.distanceExpr, categoryFilter, bboxClause))
	}
	sql := "SELECT * FROM (" + strings.Join(selects, "\nUNION ALL\n") + ") r" + nearClause + fmt.Sprintf(" ORDER BY %s, similarity DESC LIMIT %s", nearOrder.orderExpr, limitParam)
	return sql, params
}

func buildResolveRouteFeatureSQL(query, featureKind string, near *routePoint, bbox *routeBBox, limit int) (string, []any) {
	params := []any{query}
	nearClause, nearOrder := routeNearSQL(near, &params)
	bboxClause := routeBBoxSQL(bbox, &params, "geom")
	categoryClause := featureKindCategoryClause(featureKind)
	limitParam := addParam(&params, limit)
	selects := []string{}
	if featureKind != "trail" {
		selects = append(selects, fmt.Sprintf(`
  SELECT category AS kind, name, category, subtype,
         id::text AS source_id, osm_id::text AS source_osm_id,
         ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
         similarity(name, $1) AS similarity, %s AS distance_m, 'osm_pois' AS source,
         ele AS ele_m,
         CASE WHEN GeometryType(geom) IN ('LINESTRING','MULTILINESTRING') THEN 'line'
              WHEN GeometryType(geom) IN ('POLYGON','MULTIPOLYGON') THEN 'polygon'
              ELSE 'point' END AS geometry_kind,
         jsonb_build_object('min_lat', ST_YMin(ST_Envelope(geom)), 'min_lng', ST_XMin(ST_Envelope(geom)), 'max_lat', ST_YMax(ST_Envelope(geom)), 'max_lng', ST_XMax(ST_Envelope(geom))) AS bbox,
         CASE WHEN GeometryType(geom) IN ('LINESTRING','MULTILINESTRING') THEN ST_Length(geom::geography) ELSE NULL::float8 END AS length_m,
         CASE WHEN GeometryType(geom) IN ('POLYGON','MULTIPOLYGON') THEN ST_Area(geom::geography) ELSE NULL::float8 END AS area_m
  FROM osm_pois
  WHERE name %% $1 AND name IS NOT NULL%s%s`, nearOrder.distanceExpr, categoryClause, bboxClause))
	}
	if featureKind == "all" || featureKind == "trail" {
		selects = append(selects, fmt.Sprintf(`
  SELECT 'trail' AS kind, COALESCE(name, ref) AS name, 'trail' AS category, route AS subtype,
         id::text AS source_id, osm_id::text AS source_osm_id,
         ST_Y(ST_Centroid(geom)) AS lat, ST_X(ST_Centroid(geom)) AS lng,
         similarity(COALESCE(name, ref, ''), $1) AS similarity, %s AS distance_m, 'osm_routes' AS source,
         NULL::float8 AS ele_m, 'line' AS geometry_kind,
         jsonb_build_object('min_lat', ST_YMin(ST_Envelope(geom)), 'min_lng', ST_XMin(ST_Envelope(geom)), 'max_lat', ST_YMax(ST_Envelope(geom)), 'max_lng', ST_XMax(ST_Envelope(geom))) AS bbox,
         COALESCE(distance::float8 * 1000.0, ST_Length(geom::geography)) AS length_m,
         NULL::float8 AS area_m
  FROM osm_routes
  WHERE (name %% $1 OR ref %% $1)
    AND route IN ('hiking','bicycle','mtb','foot','running','via_ferrata')%s`, nearOrder.distanceExpr, bboxClause))
	}
	sql := fmt.Sprintf(`
SELECT * FROM (
%s
) r%s
ORDER BY %s, similarity DESC
LIMIT %s`, strings.Join(selects, "\nUNION ALL\n"), nearClause, nearOrder.orderExpr, limitParam)
	return sql, params
}

type routeNearOrder struct {
	distanceExpr string
	orderExpr    string
}

func routeNearSQL(near *routePoint, params *[]any) (string, routeNearOrder) {
	if near == nil || near.Lat == nil || near.Lng == nil || !validLatLng(*near.Lat, *near.Lng) {
		return "", routeNearOrder{distanceExpr: "NULL::float8", orderExpr: "similarity DESC"}
	}
	lngParam := addParam(params, *near.Lng)
	latParam := addParam(params, *near.Lat)
	expr := fmt.Sprintf("ST_Distance(geom::geography, ST_SetSRID(ST_Point(%s,%s),4326)::geography)", lngParam, latParam)
	return "", routeNearOrder{distanceExpr: expr, orderExpr: "distance_m NULLS LAST"}
}

func routeBBoxSQL(bbox *routeBBox, params *[]any, geomColumn string) string {
	if bbox == nil || !validBBox(*bbox) {
		return ""
	}
	minLng := addParam(params, bbox.MinLng)
	minLat := addParam(params, bbox.MinLat)
	maxLng := addParam(params, bbox.MaxLng)
	maxLat := addParam(params, bbox.MaxLat)
	return fmt.Sprintf(" AND %s && ST_MakeEnvelope(%s,%s,%s,%s,4326)", geomColumn, minLng, minLat, maxLng, maxLat)
}

func featureKindCategoryClause(kind string) string {
	switch kind {
	case "water":
		return " AND category IN ('still_water','flowing_water')"
	case "river":
		return " AND category = 'flowing_water'"
	case "lake":
		return " AND category = 'still_water'"
	case "forest":
		return " AND category = 'forest'"
	case "summit":
		return " AND category = 'summit'"
	case "pass":
		return " AND category = 'pass'"
	case "poi":
		return " AND category NOT IN ('settlement')"
	default:
		return ""
	}
}

func addParam(params *[]any, value any) string {
	*params = append(*params, value)
	return fmt.Sprintf("$%d", len(*params))
}

func addRefsToRouteCandidates(rows []map[string]any, refKind string) []map[string]any {
	for _, row := range rows {
		lat := numberValue(row["lat"])
		lng := numberValue(row["lng"])
		name := stringValue(row["name"])
		kind := stringValue(row["kind"])
		row["ref"] = encodeRouteResolverRef(routeResolverRef{
			Kind:        kind,
			Source:      stringValue(row["source"]),
			SourceID:    stringValue(row["source_id"]),
			SourceOSMID: stringValue(row["source_osm_id"]),
			Name:        name,
			Lat:         lat,
			Lng:         lng,
			CreatedAt:   time.Now(),
		})
		row["confidence"] = routeResolverConfidence(numberValue(row["similarity"]), numberValue(row["distance_m"]))
		if refKind == "feature" && row["feature_kind"] == nil {
			row["feature_kind"] = kind
		}
	}
	return rows
}

func encodeRouteResolverRef(ref routeResolverRef) string {
	token := uuid.NewString()
	if ref.CreatedAt.IsZero() {
		ref.CreatedAt = time.Now()
	}
	routeResolverRefCache.Store(token, ref)
	return "route-ref:" + token
}

func decodeRouteResolverRef(raw string) (routeResolverRef, error) {
	if !strings.HasPrefix(raw, "route-ref:") {
		return routeResolverRef{}, fmt.Errorf("unknown resolver ref")
	}
	token := strings.TrimPrefix(raw, "route-ref:")
	parsedToken, err := uuid.Parse(token)
	if err != nil || parsedToken.String() != token {
		return routeResolverRef{}, fmt.Errorf("invalid resolver ref")
	}
	cached, ok := routeResolverRefCache.Load(token)
	if !ok {
		return routeResolverRef{}, fmt.Errorf("resolver ref not found or expired")
	}
	ref, ok := cached.(routeResolverRef)
	if !ok {
		routeResolverRefCache.Delete(token)
		return routeResolverRef{}, fmt.Errorf("resolver ref not found or expired")
	}
	if time.Since(ref.CreatedAt) > routeResolverRefTTL {
		routeResolverRefCache.Delete(token)
		return routeResolverRef{}, fmt.Errorf("resolver ref not found or expired")
	}
	if !validLatLng(ref.Lat, ref.Lng) {
		return routeResolverRef{}, fmt.Errorf("resolver ref has invalid coordinates")
	}
	return ref, nil
}

func decodeRouteResolverRefForUser(raw, userID string) (routeResolverRef, error) {
	ref, err := decodeRouteResolverRef(raw)
	if err != nil {
		return routeResolverRef{}, err
	}
	if ref.Source == "user_location" && (userID == "" || ref.UserID != userID) {
		return routeResolverRef{}, fmt.Errorf("invalid_or_foreign_route_ref")
	}
	return ref, nil
}

func resolveRoutePoint(point routePoint) (routeResolverRef, error) {
	return resolveRoutePointForUser(point, "")
}

func resolveRoutePointForUser(point routePoint, userID string) (routeResolverRef, error) {
	if point.Ref != "" {
		return decodeRouteResolverRefForUser(point.Ref, userID)
	}
	if point.Lat == nil || point.Lng == nil {
		return routeResolverRef{}, fmt.Errorf("route point needs a resolver ref or lat/lng")
	}
	if !validLatLng(*point.Lat, *point.Lng) {
		return routeResolverRef{}, fmt.Errorf("route point coordinates out of range")
	}
	return routeResolverRef{Kind: "coordinate", Source: "user", Name: "coordinate", Lat: *point.Lat, Lng: *point.Lng}, nil
}

func routeCoveragePoints(args getRoutingCoverageArgs) ([]routeResolverRef, []string) {
	return routeCoveragePointsForUser(args, "")
}

func routeCoveragePointsForUser(args getRoutingCoverageArgs, userID string) ([]routeResolverRef, []string) {
	points := make([]routeResolverRef, 0)
	invalidRefs := []string{}
	for _, ref := range append(append([]string{}, args.PlaceRefs...), args.FeatureRefs...) {
		decoded, err := decodeRouteResolverRefForUser(ref, userID)
		if err != nil {
			invalidRefs = append(invalidRefs, ref)
			continue
		}
		points = append(points, decoded)
	}
	for _, point := range args.Points {
		decoded, err := resolveRoutePointForUser(point, userID)
		if err != nil {
			if point.Ref != "" {
				invalidRefs = append(invalidRefs, point.Ref)
			}
			continue
		}
		points = append(points, decoded)
	}
	return points, invalidRefs
}

func bboxForPoints(points []routeResolverRef) routeBBox {
	b := routeBBox{MinLat: 90, MinLng: 180, MaxLat: -90, MaxLng: -180}
	for _, p := range points {
		b.MinLat = math.Min(b.MinLat, p.Lat)
		b.MinLng = math.Min(b.MinLng, p.Lng)
		b.MaxLat = math.Max(b.MaxLat, p.Lat)
		b.MaxLng = math.Max(b.MaxLng, p.Lng)
	}
	const pad = 0.02
	b.MinLat -= pad
	b.MinLng -= pad
	b.MaxLat += pad
	b.MaxLng += pad
	return b
}

func regionsForBBox(ctx context.Context, deps *Deps, bbox *routeBBox) ([]map[string]any, bool) {
	if bbox == nil || !validBBox(*bbox) {
		return []map[string]any{}, false
	}
	catalog := []map[string]any{}
	regionIDs := []string{}
	for _, region := range osm.Regions {
		if region.MaxLat < bbox.MinLat || region.MinLat > bbox.MaxLat || region.MaxLon < bbox.MinLng || region.MinLon > bbox.MaxLng {
			continue
		}
		regionIDs = append(regionIDs, region.ID)
		catalog = append(catalog, map[string]any{"region_id": region.ID, "label": region.Label, "status": "catalog_match", "bbox": map[string]any{"min_lat": region.MinLat, "min_lng": region.MinLon, "max_lat": region.MaxLat, "max_lng": region.MaxLon}})
	}
	if len(regionIDs) == 0 {
		return []map[string]any{}, false
	}
	if deps == nil || (deps.DB == nil && deps.Readonly == nil) {
		for _, row := range catalog {
			row["verified"] = false
		}
		return catalog, true
	}
	rows, err := queryRows(ctx, deps, `
SELECT region_id, label, status, categories, data_date, imported_at, last_updated_at
FROM osm_import_regions
WHERE region_id = ANY($1) AND status IN ('imported', 'needs_redownload')
ORDER BY label`, regionIDs)
	if err != nil || len(rows) == 0 {
		return []map[string]any{}, false
	}
	for _, row := range rows {
		row["verified"] = true
	}
	return rows, true
}

func demCellsForBBox(bbox *routeBBox) []string {
	if bbox == nil || !validBBox(*bbox) {
		return []string{}
	}
	cells := dem.HGTCellsForBBox([4]float64{bbox.MinLat, bbox.MinLng, bbox.MaxLat, bbox.MaxLng})
	out := make([]string, 0, len(cells))
	for _, cell := range cells {
		out = append(out, strings.TrimSuffix(cell.FileName(), ".hgt"))
	}
	return out
}

func brouterSegmentPresence(deps *Deps, tiles []string) (string, []string, []string) {
	dir := configuredDir(depsValue(deps, "brouter_segments"), "BROUTER_SEGMENTS_DIR", "/data/brouter/segments")
	present := []string{}
	missing := []string{}
	for _, tile := range tiles {
		path := filepath.Join(dir, tile+".rd5")
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			present = append(present, tile)
			continue
		}
		missing = append(missing, tile)
	}
	return dir, present, missing
}

func demCellPresence(deps *Deps, cells []string) (string, []string, []string) {
	dir := configuredDir(depsValue(deps, "dem"), "DEM_DATA_DIR", "/data/dem")
	searchDirs := dem.HGTDirsForConfig(dir)
	present := []string{}
	missing := []string{}
	for _, cell := range cells {
		found := false
		for _, hgtDir := range searchDirs {
			if info, err := os.Stat(filepath.Join(hgtDir, cell+".hgt")); err == nil && info.Size() > 0 {
				found = true
				break
			}
		}
		if found {
			present = append(present, cell)
		} else {
			missing = append(missing, cell)
		}
	}
	return dir, present, missing
}

func badRouterTilePresence(deps *Deps, tileIDs []uint32) (string, []string, []string) {
	dir := configuredRoutingTilesDir(deps)
	present := []string{}
	missing := []string{}
	for _, tileID := range tileIDs {
		label := graphTileLabel(tileID)
		if info, err := os.Stat(graphpkg.TileFilePath(dir, tileID)); err == nil && info.Size() > 0 {
			present = append(present, label)
			continue
		}
		missing = append(missing, label)
	}
	return dir, present, missing
}

func coverageLayerPresence(deps *Deps, tileIDs []uint32) (string, []string, []string) {
	dir := configuredCoverageBaseDir(deps)
	layers := routeCoverageLayerNames()
	present := []string{}
	missing := []string{}
	for _, layer := range layers {
		allPresent := len(tileIDs) > 0
		for _, tileID := range tileIDs {
			if info, err := os.Stat(filepath.Join(dir, layer, coverageTileFileName(tileID))); err != nil || info.Size() == 0 {
				allPresent = false
				break
			}
		}
		if allPresent {
			present = append(present, layer)
		} else {
			missing = append(missing, layer)
		}
	}
	return dir, present, missing
}

func configuredDir(value, envName, fallback string) string {
	dir := strings.TrimSpace(value)
	if dir == "" {
		dir = strings.TrimSpace(os.Getenv(envName))
	}
	if dir == "" {
		dir = fallback
	}
	return dir
}

func depsValue(deps *Deps, name string) string {
	if deps == nil {
		return ""
	}
	switch name {
	case "brouter_segments":
		return deps.BRouterSegmentsDir
	case "dem":
		return deps.DEMDataDir
	default:
		return ""
	}
}

func configuredRoutingTilesDir(deps *Deps) string {
	if deps != nil {
		if dir := strings.TrimSpace(deps.RoutingTilesDir); dir != "" {
			return dir
		}
		if graphPath := strings.TrimSpace(deps.RoutingGraphPath); graphPath != "" {
			return filepath.Join(filepath.Dir(graphPath), "tiles")
		}
	}
	if dir := strings.TrimSpace(os.Getenv("ROUTING_TILES_DIR")); dir != "" {
		return dir
	}
	if graphPath := strings.TrimSpace(os.Getenv("ROUTING_GRAPH_PATH")); graphPath != "" {
		return filepath.Join(filepath.Dir(graphPath), "tiles")
	}
	return "/data/routing/tiles"
}

func configuredCoverageBaseDir(deps *Deps) string {
	if deps != nil {
		if dir := strings.TrimSpace(deps.CoverageBaseDir); dir != "" {
			return dir
		}
		if graphPath := strings.TrimSpace(deps.RoutingGraphPath); graphPath != "" {
			return filepath.Join(filepath.Dir(graphPath), "coverage")
		}
	}
	if dir := strings.TrimSpace(os.Getenv("COVERAGE_BASE_DIR")); dir != "" {
		return dir
	}
	if graphPath := strings.TrimSpace(os.Getenv("ROUTING_GRAPH_PATH")); graphPath != "" {
		return filepath.Join(filepath.Dir(graphPath), "coverage")
	}
	return "/data/routing/coverage"
}

func graphTileIDsForBBox(bbox *routeBBox) ([]uint32, bool) {
	if bbox == nil || !validBBox(*bbox) {
		return []uint32{}, false
	}
	const maxTiles = 512
	ids := []uint32{}
	for lat := int(math.Floor(bbox.MinLat)); lat <= int(math.Floor(bbox.MaxLat)); lat++ {
		for lng := int(math.Floor(bbox.MinLng)); lng <= int(math.Floor(bbox.MaxLng)); lng++ {
			if len(ids) >= maxTiles {
				return ids, true
			}
			ids = append(ids, graphpkg.TileIDFromCoord(float64(lat)+0.5, float64(lng)+0.5))
		}
	}
	return ids, false
}

func graphTileLabels(tileIDs []uint32) []string {
	labels := make([]string, 0, len(tileIDs))
	for _, tileID := range tileIDs {
		labels = append(labels, graphTileLabel(tileID))
	}
	return labels
}

func graphTileLabel(tileID uint32) string {
	lat, lng := graphpkg.TileLatLon(tileID)
	return fmt.Sprintf("%d_%d", lat, lng)
}

func coverageTileFileName(tileID uint32) string {
	return graphTileLabel(tileID) + ".bin"
}

func routeCoverageLayerNames() []string {
	return []string{"forest", "water", "river_proximity", "protected", "noise", "traffic", "urbanization"}
}

func demCoverageStatus(required, missing []string) string {
	if len(required) == 0 {
		return "not_checked"
	}
	if len(missing) == 0 {
		return "complete"
	}
	return "missing_or_partial"
}

func validBBox(b routeBBox) bool {
	return validLatLng(b.MinLat, b.MinLng) && validLatLng(b.MaxLat, b.MaxLng) && b.MinLat <= b.MaxLat && b.MinLng <= b.MaxLng
}

func normalizedPlaceKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "place", "street", "poi":
		return strings.TrimSpace(kind)
	default:
		return "all"
	}
}

func normalizedFeatureKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "water", "river", "lake", "forest", "summit", "pass", "trail", "poi":
		return strings.TrimSpace(kind)
	default:
		return "all"
	}
}

func routeResolverConfidence(similarity, distanceM float64) string {
	if similarity >= 0.8 && (distanceM == 0 || distanceM < 5000) {
		return "high"
	}
	if similarity >= 0.45 {
		return "medium"
	}
	return "low"
}

func notFoundReason(count int, reason string) any {
	if count > 0 {
		return nil
	}
	return reason
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func valueOrDefaultBool(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
