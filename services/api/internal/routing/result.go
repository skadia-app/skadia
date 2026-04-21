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

	// EdgeCoordStart[i] = index in Coordinates[] where edge i begins (the network
	// node of that edge). len(EdgeCoordStart) == len(EdgeHandles). The last edge's
	// coord-range ends at len(Coordinates)-1 (destination coord).
	// Populated analog zu EdgeHandles in retrackUnified und BuildRouteResultFromEdges.
	// json:"-" — intern für Handler-seitige Range-Berechnung, nicht im API-Response.
	EdgeCoordStart []int    `json:"-"`
	EdgeTags1      []uint64 `json:"-"` // Tags1 pro Edge — SAC/MTB-Stufen-Ranges
	EdgeTags2      []uint64 `json:"-"` // Tags2 pro Edge — Forest/Protected/Traffic-Ranges

	// LastEdgeCoordEnd is the coordinate index (exclusive upper bound) of the last
	// real network edge — i.e. len(Coordinates) at the moment the destination node
	// was appended, BEFORE any dst virtual-edge (VE) air-line tail is concatenated.
	//
	// Use this as the lastCoordIdx argument to computeAllRanges instead of
	// len(Coordinates)-1 to prevent range-highlights from bleeding into the
	// straight-line approach segment appended by the dst-VE path (F2 fix).
	// Zero means no dst-VE was appended (dst-VE is nil); callers should fall back to
	// len(Coordinates)-1 when this field is zero.
	LastEdgeCoordEnd int `json:"-"`
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
	// F26 additions
	MtbScaleUphill  string `json:"mtb_scale_uphill,omitempty"`
	ViaFerrataScale string `json:"via_ferrata_scale,omitempty"`
	TrailVisibility string `json:"trail_visibility,omitempty"`
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

// ToGeoJSONWithExtras ist identisch zu ToGeoJSON, merged aber zusaetzlich
// das extras-Map in die Properties. Wird von RouteCalculate verwendet,
// um forest_pct / sac_ranges / vf_ranges / summits_passed anzureichern,
// ohne ToGeoJSON-Consumer (SuggestRoutes via result.go-mergeLegs-Flow) zu
// brechen. buildProperties bleibt Single-Source-of-Truth fuer Kern-Properties.
func (r *RouteResult) ToGeoJSONWithExtras(extras map[string]any) json.RawMessage {
	coords := make([][]float64, len(r.Coordinates))
	for i, c := range r.Coordinates {
		coords[i] = []float64{c.Lon, c.Lat, c.Ele} // GeoJSON: [lon, lat, ele]
	}

	props := r.buildProperties()
	for k, v := range extras {
		props[k] = v
	}

	feature := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{{
			"type": "Feature",
			"geometry": map[string]any{
				"type":        "LineString",
				"coordinates": coords,
			},
			"properties": props,
		}},
	}

	b, err := json.Marshal(feature)
	if err != nil {
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

		// EdgeCoordStart, EdgeTags1, EdgeTags2 are per-edge slices that MUST stay
		// length-synchronous (invariant: len(EdgeTags1) == len(EdgeTags2) == len(EdgeCoordStart)).
		// contiguousRangesFromMatch / computeScaleRanges iterate over edgeTags* and
		// index edgeCoordStart[i] directly — a length mismatch causes an out-of-range panic.
		//
		// Guard: a follow-leg with no Coordinates has no real edges to contribute.
		// Appending its tags (or CoordStart entries) without a corresponding coord anchor
		// would break the invariant and produce bogus map-range data.
		// EdgeHandles is intentionally NOT guarded here: its consumers (aggregateLegOutdoor,
		// aggregateLeg2Coverage, ExtractWayIDsFromHandles) iterate handles independently
		// via reader.EdgeTags*(handle) and do not correlate with EdgeCoordStart by index.
		if i == 0 {
			result.EdgeCoordStart = append(result.EdgeCoordStart, leg.EdgeCoordStart...)
			result.EdgeTags1 = append(result.EdgeTags1, leg.EdgeTags1...)
			result.EdgeTags2 = append(result.EdgeTags2, leg.EdgeTags2...)
		} else if len(leg.Coordinates) > 0 {
			for _, idx := range leg.EdgeCoordStart {
				result.EdgeCoordStart = append(result.EdgeCoordStart, idx+segOffset)
			}
			result.EdgeTags1 = append(result.EdgeTags1, leg.EdgeTags1...)
			result.EdgeTags2 = append(result.EdgeTags2, leg.EdgeTags2...)
		}
		// Empty follow-leg (len(Coordinates)==0): EdgeCoordStart/Tags1/Tags2 skipped above.

		// Propagate LastEdgeCoordEnd from the last leg (F2: dst-VE bleed fix).
		// The merged result inherits the last leg's last-real-edge boundary, adjusted
		// for the coordinate offset that this leg contributes to the merged slice.
		// Non-last legs are intermediate — their LastEdgeCoordEnd is irrelevant;
		// only the final leg determines where the dst-VE tail begins (if any).
		if leg.LastEdgeCoordEnd > 0 {
			result.LastEdgeCoordEnd = leg.LastEdgeCoordEnd + segOffset
		}
	}
	return result
}
