package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// RouteDuplicateResult describes a found duplicate route.
type RouteDuplicateResult struct {
	ExistingRouteID string
	Confidence      string // "source_id", "geometry_hash", "hausdorff"
}

// CheckRouteDuplicate performs 3-tier route duplicate detection.
// Tier 1: source_platform + source_id (exact match, fast)
// Tier 2: geometry_hash computed in DB (detects same-geometry re-imports)
// Tier 3: Hausdorff distance + length difference (configurable thresholds)
func CheckRouteDuplicate(
	ctx context.Context,
	db *database.DB,
	userID string,
	sourcePlatform, sourceID string,
	geojson string,
	totalDistanceM float64,
	hausdorffThresholdM float64,
	lengthDiffThresholdKm float64,
) (*RouteDuplicateResult, error) {

	// Tier 1: Platform ID (cheapest — single indexed lookup)
	if sourcePlatform != "" && sourceID != "" {
		existingID, err := db.CheckRouteBySource(ctx, userID, sourcePlatform, sourceID)
		if err != nil {
			return nil, fmt.Errorf("route dup tier-1 source: %w", err)
		}
		if existingID != nil {
			return &RouteDuplicateResult{ExistingRouteID: *existingID, Confidence: "source_id"}, nil
		}
	}

	// Tier 2: Geometry Hash (hash computed in DB to avoid float rounding mismatch)
	existingID, err := db.CheckRouteByGeometryHashFromGeoJSON(ctx, userID, geojson)
	if err != nil {
		return nil, fmt.Errorf("route dup tier-2 hash: %w", err)
	}
	if existingID != nil {
		return &RouteDuplicateResult{ExistingRouteID: *existingID, Confidence: "geometry_hash"}, nil
	}

	// Tier 3: Hausdorff + Length (spatial proximity check)
	candidates, err := db.FindRouteCandidatesByBBox(ctx, userID, geojson)
	if err != nil {
		return nil, fmt.Errorf("route dup tier-3 bbox: %w", err)
	}

	newTrackLS := geo.SimplifyDouglasPeucker(latLngToLineString(geoJSONToLatLngs(geojson)), 20)
	lengthDiffM := lengthDiffThresholdKm * 1000

	for _, c := range candidates {
		// Length filter first (cheap — avoids Hausdorff for obviously different routes)
		if c.TotalDistanceM != nil && totalDistanceM > 0 {
			if math.Abs(*c.TotalDistanceM-totalDistanceM) > lengthDiffM {
				continue
			}
		}
		// Hausdorff distance (expensive — O(n*m))
		candidateTrackLS := geo.SimplifyDouglasPeucker(latLngToLineString(geoJSONToLatLngs(c.GeoJSON)), 20)
		dist := geo.HausdorffDistance(newTrackLS, candidateTrackLS)
		if dist <= hausdorffThresholdM {
			return &RouteDuplicateResult{ExistingRouteID: c.ID, Confidence: "hausdorff"}, nil
		}
	}

	return nil, nil
}

// geoJSONToLatLngs extracts coordinate pairs from a GeoJSON LineString or MultiLineString.
// Returns empty slice on parse error (graceful degradation).
func geoJSONToLatLngs(geojson string) []LatLng {
	var g struct {
		Type        string `json:"type"`
		Coordinates any    `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(geojson), &g); err != nil {
		return nil
	}

	switch g.Type {
	case "LineString":
		var coords [][]float64
		b, _ := json.Marshal(g.Coordinates)
		if err := json.Unmarshal(b, &coords); err != nil {
			return nil
		}
		return coordsToLatLngs(coords)
	case "MultiLineString":
		var lines [][][]float64
		b, _ := json.Marshal(g.Coordinates)
		if err := json.Unmarshal(b, &lines); err != nil {
			return nil
		}
		var result []LatLng
		for _, line := range lines {
			result = append(result, coordsToLatLngs(line)...)
		}
		return result
	}
	return nil
}

func coordsToLatLngs(coords [][]float64) []LatLng {
	pts := make([]LatLng, 0, len(coords))
	for _, c := range coords {
		if len(c) >= 2 {
			pts = append(pts, LatLng{Lat: c[1], Lng: c[0]}) // GeoJSON is [lng, lat]
		}
	}
	return pts
}
