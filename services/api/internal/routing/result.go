package routing

import (
	"encoding/json"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// RouteResult holds the complete routing result.
type RouteResult struct {
	Coordinates  []LatLonEle    `json:"-"`
	DistanceM    float64        `json:"distance_m"`
	AscentM      float64        `json:"ascent_m"`
	DescentM     float64        `json:"descent_m"`
	Cost         float64        `json:"cost"`
	Segments     []RouteSegment `json:"segments"`
	PassStats    PassStats      `json:"stats"`
	ApproachFrom []LatLonEle    `json:"-"` // Straight line: click point → edge projection (start)
	ApproachTo   []LatLonEle    `json:"-"` // Straight line: edge projection → click point (end)

	// Accumulators for Quality-Gate AvgFlat computation.
	// Populated during retrack (findRouteUnified) or reconstruction (Pareto).
	DistSum    float32 `json:"-"`
	QualitySum float32 `json:"-"` // Σ (edge.dist × edge.flat_factor_raw)
	AscentSum  float32 `json:"-"`
	DescentSum float32 `json:"-"`

	// EdgeHandles stores the edge handle sequence of the route.
	// Used for WayID-based overlap detection in round-trip quality gate.
	// Populated by retrackUnified; empty for approach segments.
	EdgeHandles []uint64 `json:"-"`
}

// AvgFlat returns the distance-weighted average flat cost factor of this route.
func (r *RouteResult) AvgFlat() float32 {
	if r.DistSum == 0 {
		return 0
	}
	return r.QualitySum / r.DistSum
}

// LatLonEle is a coordinate with elevation.
type LatLonEle struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
	Ele float64 `json:"ele"`
}

// RouteSegment describes a contiguous section with the same OSM way properties.
type RouteSegment struct {
	FromIdx   int    `json:"from_idx"`
	ToIdx     int    `json:"to_idx"`
	Highway   string `json:"highway"`
	Surface   string `json:"surface"`
	SacScale  string `json:"sac_scale,omitempty"`
	MtbScale  string `json:"mtb_scale,omitempty"`
	Tracktype string `json:"tracktype,omitempty"`
	OsmWayID  int64  `json:"osm_way_id"`
}

// PassStats contains timing/expansion metrics for debugging.
type PassStats struct {
	Pass0Expansions int   `json:"pass0_expansions"`
	Pass0DurationMs int64 `json:"pass0_duration_ms"`
	Pass1Expansions int   `json:"pass1_expansions"`
	Pass1DurationMs int64 `json:"pass1_duration_ms"`
	TotalDurationMs int64 `json:"total_duration_ms"`
}

// LatLon is a simple lat/lon pair for via-point routing.
type LatLon struct {
	Lat float64
	Lon float64
}

// ToGeoJSON converts a RouteResult to a GeoJSON FeatureCollection.
func (r *RouteResult) ToGeoJSON() json.RawMessage {
	coords := make([][]float64, len(r.Coordinates))
	for i, c := range r.Coordinates {
		coords[i] = []float64{c.Lon, c.Lat, c.Ele} // GeoJSON: [lon, lat, ele]
	}

	feature := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": r.buildProperties(),
		}},
	}

	b, err := json.Marshal(feature)
	if err != nil {
		// Fallback: return minimal valid GeoJSON
		return json.RawMessage(`{"type":"FeatureCollection","features":[]}`)
	}
	return b
}

func (r *RouteResult) buildProperties() map[string]any {
	props := map[string]any{
		"distance_m": int(r.DistanceM),
		"ascent_m":   int(r.AscentM),
		"descent_m":  int(r.DescentM),
		"cost":       int(r.Cost),
		"segments":   r.Segments,
		"stats":      r.PassStats,
	}
	if len(r.ApproachFrom) > 0 {
		ac := make([][]float64, len(r.ApproachFrom))
		for i, c := range r.ApproachFrom {
			ac[i] = []float64{c.Lon, c.Lat}
		}
		props["approach_from"] = ac
		props["approach_from_m"] = int(haversineSum(r.ApproachFrom))
	}
	if len(r.ApproachTo) > 0 {
		ac := make([][]float64, len(r.ApproachTo))
		for i, c := range r.ApproachTo {
			ac[i] = []float64{c.Lon, c.Lat}
		}
		props["approach_to"] = ac
		props["approach_to_m"] = int(haversineSum(r.ApproachTo))
	}
	return props
}

func haversineSum(pts []LatLonEle) float64 {
	var sum float64
	for i := 1; i < len(pts); i++ {
		sum += geo.DistanceHaversine(pts[i-1].Lat, pts[i-1].Lon, pts[i].Lat, pts[i].Lon)
	}
	return sum
}

// MergeLegs concatenates multiple RouteResults (via-point legs) into one.
// Exported wrapper around mergeLegs for use by handler code.
func MergeLegs(legs []*RouteResult) *RouteResult {
	return mergeLegs(legs)
}

// mergeLegs concatenates multiple RouteResults (via-point legs) into one.
func mergeLegs(legs []*RouteResult) *RouteResult {
	if len(legs) == 0 {
		return &RouteResult{}
	}
	if len(legs) == 1 {
		return legs[0]
	}

	result := &RouteResult{}
	for i, leg := range legs {
		coordsBefore := len(result.Coordinates)

		if i == 0 {
			result.Coordinates = append(result.Coordinates, leg.Coordinates...)
		} else if len(leg.Coordinates) > 0 {
			// Skip first coordinate (duplicate of previous leg's last)
			result.Coordinates = append(result.Coordinates, leg.Coordinates[1:]...)
		}
		result.DistanceM += leg.DistanceM
		result.AscentM += leg.AscentM
		result.DescentM += leg.DescentM
		result.Cost += leg.Cost
		result.DistSum += leg.DistSum
		result.QualitySum += leg.QualitySum
		result.AscentSum += leg.AscentSum
		result.DescentSum += leg.DescentSum

		segOffset := 0
		if i > 0 {
			segOffset = coordsBefore - 1
		}
		for _, seg := range leg.Segments {
			adjusted := seg
			adjusted.FromIdx += segOffset
			adjusted.ToIdx += segOffset
			result.Segments = append(result.Segments, adjusted)
		}

		result.PassStats.Pass0Expansions += leg.PassStats.Pass0Expansions
		result.PassStats.Pass1Expansions += leg.PassStats.Pass1Expansions
		result.PassStats.Pass0DurationMs += leg.PassStats.Pass0DurationMs
		result.PassStats.Pass1DurationMs += leg.PassStats.Pass1DurationMs
		result.PassStats.TotalDurationMs += leg.PassStats.TotalDurationMs
		result.EdgeHandles = append(result.EdgeHandles, leg.EdgeHandles...)
	}
	return result
}
