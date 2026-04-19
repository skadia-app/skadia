package routing

import (
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

const costBlocked float32 = 10_000 // effectively impassable

// tvSeverity maps trail_visibility enum (0..6) to additive penalty multipliers.
// 0=unknown, 1=excellent, 2=good → 0.
// 3=intermediate→1, 4=bad→2, 5=horrible→3, 6=no→4.
var tvSeverity = [7]float32{0, 0, 0, 1, 2, 3, 4}

// Type aliases for graph package landmark types (avoids import cycle).
type LandmarkSetID = graph.LandmarkSetID

const (
	LandmarkSetA    = graph.LandmarkSetA
	LandmarkSetB    = graph.LandmarkSetB
	LandmarkSetC    = graph.LandmarkSetC
	LandmarkSetD    = graph.LandmarkSetD
	NumLandmarkSets = graph.NumLandmarkSets
	LandmarksPerSet = graph.LandmarksPerSet
)

// EdgeCostFactors are pure costfactor multipliers (NOT pre-multiplied by distance).
// The A*-Loop computes: distM * interpolated_costfactor.
// Blocked edges: Flat >= costBlocked (10000).
type EdgeCostFactors struct {
	Flat     float32 // costfactor for flat terrain
	Up       float32 // uphillcostfactor (0 = use Flat)
	Down     float32 // downhillcostfactor (0 = use Flat)
	WayClass uint8   // 0=normal, 1=ferry, 2=mainroad — for initialcost tracking
}

// CostMin returns the minimum of Flat, Up, Down — the most optimistic
// cost factor for backward search. Zero Up/Down are treated as Flat.
// Defined on routing.EdgeCostFactors (returned by cfg.EdgeCost()),
// NOT on graph.EdgeCostFactors (used in UnifiedCSR).
func (ecf EdgeCostFactors) CostMin() float32 {
	up := ecf.Up
	if up == 0 {
		up = ecf.Flat
	}
	down := ecf.Down
	if down == 0 {
		down = ecf.Flat
	}
	return min(ecf.Flat, up, down)
}

// CostConfig holds the routing cost configuration.
// EdgeCost is a closure — Phase 4 creates it from SportProfile.
type CostConfig struct {
	// Elevation Hysteresis
	UphillCost        float32
	DownhillCost      float32
	UphillCutoff      float32
	DownhillCutoff    float32
	ElevPenaltyBuffer float32 // meters
	ElevMaxBuffer     float32 // meters
	ElevBufferReduce  float32 // buffer decay rate: mm drain per mm distance (BRouter elevationbufferreduce * 0.01)

	// Turn cost (cost per turn, applied in A* loop)
	TurnCost float32

	// A* control
	Pass0Coefficient float32

	// Precomputed from above
	MaxLatentCost float32

	// AvgCostPerKm is the calibrated average BadRouter cost per km of air distance.
	// Used to convert time-based isochrone budgets to cost budgets.
	AvgCostPerKm float32

	// SnapThresholdM controls when approach segments are emitted. If the snap
	// distance exceeds this value (in meters), a straight-line approach segment
	// is added. Zero means default (100m).
	SnapThresholdM float32

	// Edge cost function — receives packed tag words directly, no Graph pointer needed.
	// Enables GraphReader abstraction where only tags are available (Phase 2+).
	EdgeCost func(tags1, tags2 uint64) EdgeCostFactors

	// WayPenalties maps OSM way ID to a cost multiplier (must be >= 1.0).
	// Nil means no penalties active (fast path in lookupWayPenalty).
	WayPenalties map[int64]float32

	// EdgeCostRaw is the unpenalized edge cost closure. Used for metrics
	// accumulation during retrack so that QualitySum reflects true path
	// quality, not penalty-distorted cost.
	// When WayPenalties is nil, EdgeCost and EdgeCostRaw are the same closure.
	EdgeCostRaw func(tags1, tags2 uint64) EdgeCostFactors
}

// RouteRequest defines start/destination for a routing query.
type RouteRequest struct {
	FromLat float64
	FromLon float64
	ToLat   float64
	ToLon   float64
}

// DefaultTrekkingConfig returns a CostConfig for trekking/hiking.
// Deprecated: Use DefaultCostConfig() or BuildCostConfig() instead.
func DefaultTrekkingConfig() *CostConfig {
	return DefaultCostConfig()
}

// trekkingEdgeCost computes edge cost factors for trekking profile.
// Returns costfactors (NOT pre-multiplied by distance).
func trekkingEdgeCost(tags1, _ uint64) EdgeCostFactors {
	hw := graph.DecodeHighway(tags1)
	sf := graph.DecodeSurface(tags1)
	access := graph.DecodeAccess(tags1)

	// Block unknown (0), motorways/trunks (1-4) and no-access
	if hw <= 4 {
		return EdgeCostFactors{Flat: costBlocked}
	}
	if access == 2 { // no
		return EdgeCostFactors{Flat: costBlocked}
	}

	hwFactor := highwayCostFactor(hw)
	sfFactor := surfaceCostFactor(sf)

	return EdgeCostFactors{Flat: hwFactor * sfFactor}
}

// highwayCostFactor returns the cost multiplier for a highway type (lower = preferred).
func highwayCostFactor(hw uint8) float32 {
	switch hw {
	case 15: // track
		return 1.0
	case 16: // path
		return 1.0
	case 17: // footway
		return 1.1
	case 19: // bridleway
		return 1.2
	case 18: // cycleway
		return 1.3
	case 20: // steps
		return 1.5
	case 13: // unclassified
		return 1.5
	case 11: // residential
		return 2.0
	case 12: // living_street
		return 2.0
	case 14: // service
		return 2.0
	case 21: // pedestrian
		return 1.2
	case 9, 10: // tertiary, tertiary_link
		return 2.5
	case 7, 8: // secondary, secondary_link
		return 3.0
	case 5, 6: // primary, primary_link
		return 4.0
	case 22: // via_ferrata
		return 1.5
	default:
		return 2.0
	}
}

// surfaceCostFactor returns the cost multiplier for a surface type.
func surfaceCostFactor(sf uint8) float32 {
	switch sf {
	case 0: // unknown
		return 1.0
	case 1, 2, 3: // asphalt, concrete, paving_stones
		return 1.2
	case 8, 9: // compacted, fine_gravel
		return 1.0
	case 10, 11: // gravel, pebblestone
		return 1.1
	case 12, 13, 16: // dirt, earth, ground
		return 1.0
	case 14: // grass
		return 1.3
	case 17: // mud
		return 2.0
	case 18: // sand
		return 2.0
	case 22, 23: // rock, stone
		return 1.4
	case 20: // snow
		return 3.0
	case 21: // ice
		return 5.0
	default:
		return 1.1
	}
}
