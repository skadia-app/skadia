package routing

import (
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

const costBlocked float32 = 10_000 // effectively impassable

// EdgeCostFactors are pure costfactor multipliers (NOT pre-multiplied by distance).
// The A*-Loop computes: distM * interpolated_costfactor.
// Blocked edges: Flat >= costBlocked (10000).
type EdgeCostFactors struct {
	Flat     float32 // costfactor for flat terrain
	Up       float32 // uphillcostfactor (0 = use Flat)
	Down     float32 // downhillcostfactor (0 = use Flat)
	WayClass uint8   // 0=normal, 1=ferry, 2=mainroad — for initialcost tracking
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

	// Edge cost function (closure-based for Phase 4 access to config)
	EdgeCost func(g *graph.Graph, edgeIdx uint32) EdgeCostFactors
}

// RouteRequest defines start/destination for a routing query.
type RouteRequest struct {
	FromLat float64
	FromLon float64
	ToLat   float64
	ToLon   float64
}

// DefaultTrekkingConfig returns a CostConfig for trekking/hiking.
func DefaultTrekkingConfig() *CostConfig {
	cfg := &CostConfig{
		UphillCost:        1.0,
		DownhillCost:      0.0,
		UphillCutoff:      0,
		DownhillCutoff:    0,
		ElevPenaltyBuffer: 5.0,
		ElevMaxBuffer:     10.0,
		ElevBufferReduce:  0.0,
		Pass0Coefficient:  1.5,
	}
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	cfg.EdgeCost = trekkingEdgeCost
	return cfg
}

// trekkingEdgeCost computes edge cost factors for trekking profile.
// Returns costfactors (NOT pre-multiplied by distance).
func trekkingEdgeCost(g *graph.Graph, edgeIdx uint32) EdgeCostFactors {
	tags := g.EdgeTags1[edgeIdx]

	hw := graph.DecodeHighway(tags)
	sf := graph.DecodeSurface(tags)
	access := graph.DecodeAccess(tags)

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
