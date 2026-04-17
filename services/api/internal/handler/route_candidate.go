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

	// HasCoverage is true iff at least one of the five merged coverage values
	// comes from real coverage data (i.e., at least one edge along the route
	// had non-zero tags2 coverage bits).
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
type LabeledRoute struct {
	Candidate       RouteCandidate
	VariantID       string
	VariantLabelKey string
	Descriptors     []VariantDescriptor
}
