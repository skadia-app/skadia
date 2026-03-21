package routing

// LandmarkCostConfig defines the elevation parameters for one landmark set.
type LandmarkCostConfig struct {
	SetID            LandmarkSetID
	UphillCost       float32
	DownhillCost     float32
	UphillCutoffPct  float32 // slope percentage (e.g. 3.0 = 3%)
	DownhillCutoffPct float32
	ElevPenaltyBuffer float32 // meters
	ElevMaxBuffer     float32 // meters
	ElevBufferReduce  float32 // mm drain per mm distance
}

// LandmarkCostConfigs defines the 4 elevation configurations for ALT landmark sets.
// All 4 share the same edge-cost closure (hiking base, enrichment=0) but differ
// in elevation hysteresis parameters. ElevBufferReduce = 0.01 ensures admissibility.
var LandmarkCostConfigs = [NumLandmarkSets]LandmarkCostConfig{
	{LandmarkSetA, 7, 7, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetB, 7, 0, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetC, 0, 7, 3.0, 3.0, 5, 10, 0.01},
	{LandmarkSetD, 0, 0, 3.0, 3.0, 5, 10, 0.01},
}

// BuildLandmarkCostConfig creates a CostConfig from a LandmarkCostConfig.
// The EdgeCost closure is shared across all sets (pure infrastructure cost).
func BuildLandmarkCostConfig(lcc LandmarkCostConfig, edgeCostFn func(tags1, tags2 uint64) EdgeCostFactors) *CostConfig {
	cfg := &CostConfig{
		UphillCost:        lcc.UphillCost,
		DownhillCost:      lcc.DownhillCost,
		UphillCutoff:      lcc.UphillCutoffPct / 100.0,
		DownhillCutoff:    lcc.DownhillCutoffPct / 100.0,
		ElevPenaltyBuffer: lcc.ElevPenaltyBuffer,
		ElevMaxBuffer:     lcc.ElevMaxBuffer,
		ElevBufferReduce:  lcc.ElevBufferReduce,
		TurnCost:          0, // no turn cost for landmarks
		Pass0Coefficient:  0, // unused in Dijkstra
		EdgeCost:          edgeCostFn,
	}
	cfg.MaxLatentCost = cfg.ElevMaxBuffer * max(cfg.UphillCost, cfg.DownhillCost)
	return cfg
}



