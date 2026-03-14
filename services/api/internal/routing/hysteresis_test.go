package routing

import (
	"math"
	"testing"
)

func defaultHystCfg() *CostConfig {
	return &CostConfig{
		UphillCostFactor:   1.0,
		DownhillCostFactor: 0.0,
		UphillCutoff:       0,
		DownhillCutoff:     0,
		ElevPenaltyBuffer:  5.0,
		ElevMaxBuffer:      10.0,
	}
}

func TestHysteresis_FlatTerrain_NoAccumulation(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}
	// 100m flat edge, 0m elevation change (100m = 100_000 mm)
	newState, elevCost := propagateHysteresis(state, 0, 100_000, cfg)
	if newState.Ehbu != 0 || newState.Ehbd != 0 {
		t.Errorf("flat terrain: buffers should be 0, got ehbu=%d ehbd=%d",
			newState.Ehbu, newState.Ehbd)
	}
	if elevCost != 0 {
		t.Errorf("flat terrain: elevCost should be 0, got %f", elevCost)
	}
}

func TestHysteresis_SteadyClimb_BufferFills(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Climb 3m over 100m edge → 3m in mm = 3_000
	state, elevCost := propagateHysteresis(state, 3_000, 100_000, cfg)
	if state.Ehbu != 3_000 {
		t.Errorf("after 3m climb: ehbu should be 3000, got %d", state.Ehbu)
	}
	if elevCost != 0 {
		t.Errorf("first climb: elevCost should be 0 (below penalty buffer), got %f", elevCost)
	}

	// Climb another 3m → total 6m, exceeds PenaltyBuffer (5m)
	state, elevCost = propagateHysteresis(state, 3_000, 100_000, cfg)
	if state.Ehbu != 6_000 {
		t.Errorf("after 6m climb: ehbu should be 6000, got %d", state.Ehbu)
	}
	// Cost for excess above 5m: (6-5) * 1.0 = 1.0
	if math.Abs(float64(elevCost)-1.0) > 0.01 {
		t.Errorf("elevCost should be ~1.0, got %f", elevCost)
	}
}

func TestHysteresis_BufferClamped_AtMax(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Climb 15m in one step — should clamp at 10m (ElevMaxBuffer)
	state, _ = propagateHysteresis(state, 15_000, 100_000, cfg)
	maxBuf := int32(cfg.ElevMaxBuffer * 1e3)
	if state.Ehbu != maxBuf {
		t.Errorf("buffer should be clamped at %d, got %d", maxBuf, state.Ehbu)
	}
}

func TestHysteresis_GPSNoise_Absorbed(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Simulate ±1m GPS noise over 10 edges
	for i := 0; i < 10; i++ {
		delta := int32(1_000) // +1m in mm
		if i%2 == 1 {
			delta = -1_000 // -1m
		}
		state, _ = propagateHysteresis(state, delta, 50_000, cfg)
	}
	// After equal up/down, buffers should be small (noise absorbed)
	if state.Ehbu > 2_000 || state.Ehbd > 2_000 {
		t.Errorf("GPS noise should be absorbed, got ehbu=%d ehbd=%d",
			state.Ehbu, state.Ehbd)
	}
}

func TestElevationCorrection(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{Ehbu: 7_000, Ehbd: 0} // 7m uphill buffer (7000 mm)
	correction := state.elevationCorrection(cfg)
	// 7m * 1.0 (UphillCostFactor) = 7.0
	if math.Abs(float64(correction)-7.0) > 0.01 {
		t.Errorf("correction should be ~7.0, got %f", correction)
	}
}

func TestIsWorse_UnvisitedNode(t *testing.T) {
	cfg := defaultHystCfg()
	bestCost := make([]float32, 10)
	nodeGen := make([]uint32, 10)
	// Node 5 never visited (nodeGen[5]=0, gen=1)
	result := isWorse(5, 100.0, PathState{}, bestCost, nodeGen, 1, cfg)
	if result {
		t.Error("unvisited node should not be worse")
	}
}

func TestIsWorse_BetterCost(t *testing.T) {
	cfg := defaultHystCfg()
	bestCost := []float32{0, 0, 0, 0, 0, 50.0}
	nodeGen := []uint32{0, 0, 0, 0, 0, 1}
	// New cost 30 + correction 0 = 30 < 50 → not worse
	result := isWorse(5, 30.0, PathState{}, bestCost, nodeGen, 1, cfg)
	if result {
		t.Error("lower cost should not be worse")
	}
}

func TestIsWorse_WorseCost(t *testing.T) {
	cfg := defaultHystCfg()
	bestCost := []float32{0, 0, 0, 0, 0, 50.0}
	nodeGen := []uint32{0, 0, 0, 0, 0, 1}
	// New cost 60 + correction 0 = 60 >= 50 → worse
	result := isWorse(5, 60.0, PathState{}, bestCost, nodeGen, 1, cfg)
	if !result {
		t.Error("higher cost should be worse")
	}
}
