package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/osm"
)

const defaultMaxDirectRouteLegM = 100000

type analyzeRoutePointsArgs struct {
	Points []routePoint `json:"points"`
	Intent string       `json:"intent,omitempty"`
}

func executeAnalyzeRoutePoints(_ context.Context, deps *Deps, userID string, raw json.RawMessage) (any, error) {
	var args analyzeRoutePointsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("decode analyze_route_points args: %w", err)
	}
	points := make([]routeResolverRef, 0, len(args.Points))
	invalidRefs := []string{}
	for _, point := range args.Points {
		resolved, err := resolveRoutePointForUser(point, userID)
		if err != nil {
			if point.Ref != "" {
				invalidRefs = append(invalidRefs, point.Ref)
			}
			continue
		}
		points = append(points, resolved)
	}
	if len(invalidRefs) > 0 || len(points) < 2 {
		reasons := []string{}
		if len(invalidRefs) > 0 {
			reasons = append(reasons, "invalid_or_foreign_route_ref")
		}
		if len(points) < 2 {
			reasons = append(reasons, "at_least_two_points_required")
		}
		return map[string]any{
			"intent":                             defaultString(args.Intent, "distance_only"),
			"long_route_status":                  "invalid_points",
			"blocking_reasons":                   reasons,
			"invalid_refs":                       invalidRefs,
			"legs":                               []map[string]any{},
			"total_straight_line_m":              float64(0),
			"max_leg_m":                          float64(0),
			"max_direct_leg_m":                   float64(maxDirectRouteLegM()),
			"suggested_intermediate_point_count": 0,
			"recommended_next_step":              "resolve route points again before route planning",
		}, nil
	}

	maxDirect := maxDirectRouteLegM()
	legs := make([]map[string]any, 0, len(points)-1)
	total := 0.0
	maxLeg := 0.0
	suggested := 0
	agg := newRouteCoverageAggregate()
	for i := 0; i < len(points)-1; i++ {
		from := points[i]
		to := points[i+1]
		distance := geo.DistanceHaversine(from.Lat, from.Lng, to.Lat, to.Lng)
		total += distance
		maxLeg = math.Max(maxLeg, distance)
		if distance > float64(maxDirect) {
			suggested += int(math.Ceil(distance/float64(maxDirect))) - 1
		}
		legBBox := bboxForPoints([]routeResolverRef{from, to})
		leg := routePointLegDiagnostics(deps, from, to, legBBox, distance)
		legs = append(legs, leg)
		agg.add(leg)
	}
	status := "ok"
	reasons := []string{}
	next := "route points are within the direct-leg policy"
	if maxLeg > float64(maxDirect) {
		status = "requires_waypoints"
		reasons = append(reasons, "max_direct_leg_exceeded")
		next = "ask the user for intermediate waypoints before direct route planning"
	}
	return map[string]any{
		"intent":                             defaultString(args.Intent, "distance_only"),
		"legs":                               legs,
		"total_straight_line_m":              total,
		"max_leg_m":                          maxLeg,
		"max_direct_leg_m":                   float64(maxDirect),
		"long_route_status":                  status,
		"suggested_intermediate_point_count": suggested,
		"coverage_matrix":                    agg.result(),
		"blocking_reasons":                   reasons,
		"recommended_next_step":              next,
	}, nil
}

func routePointLegDiagnostics(deps *Deps, from, to routeResolverRef, bbox routeBBox, distance float64) map[string]any {
	requiredTiles := osm.BRouterTileIDs([]osm.Region{{MinLon: bbox.MinLng, MinLat: bbox.MinLat, MaxLon: bbox.MaxLng, MaxLat: bbox.MaxLat}})
	segmentsDir, presentTiles, missingTiles := brouterSegmentPresence(deps, requiredTiles)
	requiredDEM := demCellsForBBox(&bbox)
	demDir, presentDEM, missingDEM := demCellPresence(deps, requiredDEM)
	graphIDs, truncated := graphTileIDsForBBox(&bbox)
	requiredGraph := graphTileLabels(graphIDs)
	routingDir, presentGraph, missingGraph := badRouterTilePresence(deps, graphIDs)
	coverageDir, presentLayers, missingLayers := coverageLayerPresence(deps, graphIDs)
	return map[string]any{
		"from":                     routeLegPoint(from),
		"to":                       routeLegPoint(to),
		"straight_line_distance_m": distance,
		"brouter": map[string]any{
			"segments_dir":       segmentsDir,
			"required_rd5_tiles": requiredTiles,
			"present_rd5_tiles":  presentTiles,
			"missing_rd5_tiles":  missingTiles,
			"verification":       "file_presence_only",
		},
		"dem": map[string]any{
			"dem_dir":            demDir,
			"required_hgt_cells": requiredDEM,
			"present_hgt_cells":  presentDEM,
			"missing_hgt_cells":  missingDEM,
			"status":             demCoverageStatus(requiredDEM, missingDEM),
			"verification":       "file_presence_only",
		},
		"badrouter": map[string]any{
			"routing_tiles_dir":   routingDir,
			"required_tiles":      requiredGraph,
			"present_tiles":       presentGraph,
			"missing_tiles":       missingGraph,
			"tile_list_truncated": truncated,
			"verification":        "file_presence_only",
		},
		"coverage_layers": map[string]any{
			"coverage_base_dir":   coverageDir,
			"required_layers":     routeCoverageLayerNames(),
			"present_layers":      presentLayers,
			"missing_layers":      missingLayers,
			"tile_list_truncated": truncated,
			"verification":        "file_presence_only",
		},
	}
}

func routeLegPoint(point routeResolverRef) map[string]any {
	return map[string]any{
		"ref_name": point.Name,
		"source":   point.Source,
		"lat":      point.Lat,
		"lng":      point.Lng,
	}
}

func maxDirectRouteLegM() int {
	raw := strings.TrimSpace(os.Getenv("SKADIA_AGENT_MAX_DIRECT_LEG_M"))
	if raw == "" {
		return defaultMaxDirectRouteLegM
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return defaultMaxDirectRouteLegM
	}
	return v
}

type routeCoverageAggregate struct {
	requiredRD5   map[string]bool
	presentRD5    map[string]bool
	missingRD5    map[string]bool
	requiredDEM   map[string]bool
	presentDEM    map[string]bool
	missingDEM    map[string]bool
	requiredGraph map[string]bool
	presentGraph  map[string]bool
	missingGraph  map[string]bool
	presentLayers map[string]bool
	missingLayers map[string]bool
}

func newRouteCoverageAggregate() *routeCoverageAggregate {
	return &routeCoverageAggregate{
		requiredRD5:   map[string]bool{},
		presentRD5:    map[string]bool{},
		missingRD5:    map[string]bool{},
		requiredDEM:   map[string]bool{},
		presentDEM:    map[string]bool{},
		missingDEM:    map[string]bool{},
		requiredGraph: map[string]bool{},
		presentGraph:  map[string]bool{},
		missingGraph:  map[string]bool{},
		presentLayers: map[string]bool{},
		missingLayers: map[string]bool{},
	}
}

func (a *routeCoverageAggregate) add(leg map[string]any) {
	addStrings(a.requiredRD5, leg["brouter"].(map[string]any)["required_rd5_tiles"])
	addStrings(a.presentRD5, leg["brouter"].(map[string]any)["present_rd5_tiles"])
	addStrings(a.missingRD5, leg["brouter"].(map[string]any)["missing_rd5_tiles"])
	addStrings(a.requiredDEM, leg["dem"].(map[string]any)["required_hgt_cells"])
	addStrings(a.presentDEM, leg["dem"].(map[string]any)["present_hgt_cells"])
	addStrings(a.missingDEM, leg["dem"].(map[string]any)["missing_hgt_cells"])
	addStrings(a.requiredGraph, leg["badrouter"].(map[string]any)["required_tiles"])
	addStrings(a.presentGraph, leg["badrouter"].(map[string]any)["present_tiles"])
	addStrings(a.missingGraph, leg["badrouter"].(map[string]any)["missing_tiles"])
	addStrings(a.presentLayers, leg["coverage_layers"].(map[string]any)["present_layers"])
	addStrings(a.missingLayers, leg["coverage_layers"].(map[string]any)["missing_layers"])
}

func (a *routeCoverageAggregate) result() map[string]any {
	return map[string]any{
		"brouter": map[string]any{
			"required_rd5_tiles": sortedSetStrings(a.requiredRD5),
			"present_rd5_tiles":  sortedSetStrings(a.presentRD5),
			"missing_rd5_tiles":  sortedSetStrings(a.missingRD5),
		},
		"dem": map[string]any{
			"required_hgt_cells": sortedSetStrings(a.requiredDEM),
			"present_hgt_cells":  sortedSetStrings(a.presentDEM),
			"missing_hgt_cells":  sortedSetStrings(a.missingDEM),
		},
		"badrouter": map[string]any{
			"required_tiles": sortedSetStrings(a.requiredGraph),
			"present_tiles":  sortedSetStrings(a.presentGraph),
			"missing_tiles":  sortedSetStrings(a.missingGraph),
		},
		"coverage_layers": map[string]any{
			"present_layers": sortedSetStrings(a.presentLayers),
			"missing_layers": sortedSetStrings(a.missingLayers),
		},
	}
}
