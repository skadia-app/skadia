package handler

import "github.com/skadia-app/skadia/services/api/internal/routing"

// RouteCandidate is a fully-constructed round-trip with merged metrics.
type RouteCandidate struct {
	NodeID uint64
	ViaLat float64
	ViaLon float64

	Leg1   *routing.RouteResult
	Leg2   *routing.RouteResult
	Merged *routing.RouteResult

	DistanceM float64
	AscentM   float64
	DescentM  float64
	AvgFlat   float32

	// Distance-weighted averages across merged route.
	// 0 means "either no coverage data OR actually zero value" — see HasCoverage.
	MergedForestPct      float32
	MergedRiverProximity float32
	MergedProtectedArea  float32
	MergedNoiseScore     float32
	MergedTrafficClass   float32

	// HasCoverage is true iff at least one coverage layer (forest, water,
	// protected area, noise, or traffic) returned a real measurement for
	// this route. False means all Merged* values are zero only because no
	// coverage data exists — not because the route scored zero on merit.
	HasCoverage bool

	LoopQuality LoopQuality
	OverlapPct  float64
}

// VariantDescriptor describes a single distinctive feature of a route.
type VariantDescriptor struct {
	Key          string  `json:"key"`
	Magnitude    float64 `json:"magnitude"`
	DisplayValue string  `json:"display_value,omitempty"`
}

// LabeledRoute is a RouteCandidate plus its assigned variant_id and descriptors.
//
// TODO(route-variants-v2): produced by assignLabels (Phase 3). Not yet wired
// into the handler response path.
type LabeledRoute struct {
	Candidate       RouteCandidate
	VariantID       string
	VariantLabelKey string
	Descriptors     []VariantDescriptor
}
