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
	for i := range 10 {
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

func TestHysteresis_ProfileUpDownUp(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Simulate a 2km edge: up 100m, flat, down 50m, flat, up 30m (net +80m, work 180m)
	// Profile: [0, 1000, 1000, 500, 500, 800] (elevation in dm, 6 points)
	// Segments: 5 segments of 400m each
	elevProfile := []int32{0, 1000, 1000, 500, 500, 800}
	segDist := []int32{400_000, 400_000, 400_000, 400_000, 400_000} // mm

	newState, elevCost := propagateHysteresisProfile(state, elevProfile, segDist, cfg)

	if elevCost == 0 {
		t.Error("profile with 180m elevation work should have non-zero cost")
	}

	// Compare with single-delta (wrong) calculation
	singleDelta := int32((800 - 0) * 100) // 80m in mm
	singleDist := int32(2_000_000)         // 2km in mm
	_, singleCost := propagateHysteresis(PathState{}, singleDelta, singleDist, cfg)

	if elevCost <= singleCost {
		t.Errorf("profile cost %f should be > single-delta cost %f (180m vs 80m work)",
			elevCost, singleCost)
	}

	_ = newState
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
