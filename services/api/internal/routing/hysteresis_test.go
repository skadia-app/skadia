package routing

import (
	"math"
	"testing"
)

func defaultHystCfg() *CostConfig {
	return &CostConfig{
		UphillCost:        1.0,
		DownhillCost:      0.0,
		UphillCutoff:      0,
		DownhillCutoff:    0,
		ElevPenaltyBuffer: 5.0,
		ElevMaxBuffer:     10.0,
	}
}

func TestHysteresis_FlatTerrain_NoAccumulation(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}
	// 100m flat edge, 0m elevation change (100m = 100_000 mm)
	hr := propagateHysteresis(state, 0, 100_000, cfg)
	if hr.State.Ehbu != 0 || hr.State.Ehbd != 0 {
		t.Errorf("flat terrain: buffers should be 0, got ehbu=%d ehbd=%d",
			hr.State.Ehbu, hr.State.Ehbd)
	}
	if hr.ElevCost != 0 {
		t.Errorf("flat terrain: elevCost should be 0, got %f", hr.ElevCost)
	}
	if hr.UpWeight != 0 || hr.DownWeight != 0 {
		t.Errorf("flat terrain: weights should be 0, got up=%f down=%f",
			hr.UpWeight, hr.DownWeight)
	}
}

func TestHysteresis_SteadyClimb_BufferFills(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Climb 3m over 100m edge → 3m in mm = 3_000
	hr := propagateHysteresis(state, 3_000, 100_000, cfg)
	if hr.State.Ehbu != 3_000 {
		t.Errorf("after 3m climb: ehbu should be 3000, got %d", hr.State.Ehbu)
	}
	if hr.ElevCost != 0 {
		t.Errorf("first climb: elevCost should be 0 (below penalty buffer), got %f", hr.ElevCost)
	}

	// Climb another 3m → total 6m, exceeds PenaltyBuffer (5m)
	hr = propagateHysteresis(hr.State, 3_000, 100_000, cfg)
	// With ElevBufferReduce=0, the excess is fully drained. After draining:
	// ehbu was 6000, excess = 6000-5000 = 1000mm, drain 1000mm → ehbu = 5000
	if hr.State.Ehbu != 5_000 {
		t.Errorf("after 6m climb: ehbu should be 5000 (drained excess), got %d", hr.State.Ehbu)
	}
	// Cost for excess above 5m: (1m) * 1.0 = 1.0
	if math.Abs(float64(hr.ElevCost)-1.0) > 0.01 {
		t.Errorf("elevCost should be ~1.0, got %f", hr.ElevCost)
	}
	// UpWeight should be 1.0 (ElevBufferReduce=0 → full activation)
	if hr.UpWeight != 1.0 {
		t.Errorf("UpWeight should be 1.0 with ElevBufferReduce=0, got %f", hr.UpWeight)
	}
}

func TestHysteresis_BufferClamped_AtMax(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{}

	// Climb 15m in one step — should clamp at 10m (ElevMaxBuffer)
	hr := propagateHysteresis(state, 15_000, 100_000, cfg)
	// After clamping at 10000mm, excess = 10000-5000 = 5000mm. With reduce=0
	// the excess is fully drained: ehbu = 10000 - 5000 = 5000 (penBuf).
	if hr.State.Ehbu != 5_000 {
		t.Errorf("buffer should be at penBuf after drain, got %d", hr.State.Ehbu)
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
		hr := propagateHysteresis(state, delta, 50_000, cfg)
		state = hr.State
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

	hr := propagateHysteresisProfile(state, elevProfile, segDist, cfg)

	if hr.ElevCost == 0 {
		t.Error("profile with 180m elevation work should have non-zero cost")
	}

	// Compare with single-delta (wrong) calculation
	singleDelta := int32((800 - 0) * 100) // 80m in mm
	singleDist := int32(2_000_000)        // 2km in mm
	singleHR := propagateHysteresis(PathState{}, singleDelta, singleDist, cfg)

	if hr.ElevCost <= singleHR.ElevCost {
		t.Errorf("profile cost %f should be > single-delta cost %f (180m vs 80m work)",
			hr.ElevCost, singleHR.ElevCost)
	}

	_ = hr.State
}

func TestElevationCorrection(t *testing.T) {
	cfg := defaultHystCfg()
	state := PathState{Ehbu: 7_000, Ehbd: 0} // 7m uphill buffer (7000 mm)
	correction := state.elevationCorrection(cfg)
	// 7m * 1.0 (UphillCost) = 7.0
	if math.Abs(float64(correction)-7.0) > 0.01 {
		t.Errorf("correction should be ~7.0, got %f", correction)
	}
}

func TestHysteresis_ElevBufferReduce_PartialActivation(t *testing.T) {
	cfg := &CostConfig{
		UphillCost:        1.0,
		DownhillCost:      0.0,
		UphillCutoff:      0,
		DownhillCutoff:    0,
		ElevPenaltyBuffer: 5.0,
		ElevMaxBuffer:     10.0,
		ElevBufferReduce:  0.01, // 0.01 mm drain per mm distance
	}

	// Start with buffer at 8m (8000mm), which is 3000mm above penBuf (5000mm).
	state := PathState{Ehbu: 8_000}

	// Edge: 100m = 100_000mm, no elevation change
	hr := propagateHysteresis(state, 0, 100_000, cfg)

	// reduce = 100_000 * 0.01 = 1000mm
	// excess = 8000 - 5000 = 3000mm
	// reduce (1000) < excess (3000) → weight = 1.0 (full activation)
	// drain 1000mm → new ehbu = 8000 - 1000 = 7000
	if hr.State.Ehbu != 7_000 {
		t.Errorf("expected ehbu=7000 after partial drain, got %d", hr.State.Ehbu)
	}
	// Cost = 1000mm / 1000 * 1.0 = 1.0
	if math.Abs(float64(hr.ElevCost)-1.0) > 0.01 {
		t.Errorf("expected elevCost ~1.0, got %f", hr.ElevCost)
	}
	if hr.UpWeight != 1.0 {
		t.Errorf("expected UpWeight=1.0 (reduce < excess), got %f", hr.UpWeight)
	}
}

func TestHysteresis_ElevBufferReduce_ExcessSmall(t *testing.T) {
	cfg := &CostConfig{
		UphillCost:        1.0,
		DownhillCost:      0.0,
		UphillCutoff:      0,
		DownhillCutoff:    0,
		ElevPenaltyBuffer: 5.0,
		ElevMaxBuffer:     10.0,
		ElevBufferReduce:  0.01,
	}

	// Start with buffer at 5.5m (5500mm), only 500mm above penBuf.
	state := PathState{Ehbu: 5_500}

	// Edge: 100m = 100_000mm, no elevation change
	hr := propagateHysteresis(state, 0, 100_000, cfg)

	// reduce = 100_000 * 0.01 = 1000mm
	// excess = 5500 - 5000 = 500mm
	// reduce (1000) > excess (500) → weight = 500/1000 = 0.5
	// drain = excess = 500mm → new ehbu = 5500 - 500 = 5000 (penBuf)
	if hr.State.Ehbu != 5_000 {
		t.Errorf("expected ehbu=5000 after full excess drain, got %d", hr.State.Ehbu)
	}
	// Cost = 500mm / 1000 * 1.0 = 0.5
	if math.Abs(float64(hr.ElevCost)-0.5) > 0.01 {
		t.Errorf("expected elevCost ~0.5, got %f", hr.ElevCost)
	}
	if math.Abs(float64(hr.UpWeight)-0.5) > 0.01 {
		t.Errorf("expected UpWeight ~0.5 (partial activation), got %f", hr.UpWeight)
	}
}

func TestHysteresisResult_WeightsInProfile(t *testing.T) {
	cfg := &CostConfig{
		UphillCost:        1.0,
		DownhillCost:      1.0,
		UphillCutoff:      0,
		DownhillCutoff:    0,
		ElevPenaltyBuffer: 2.0,
		ElevMaxBuffer:     20.0,
		ElevBufferReduce:  0.01,
	}

	// Profile: steady 10m climb over 2 segments of 500m each
	elevProfile := []int32{0, 50, 100} // dm
	segDist := []int32{500_000, 500_000}

	hr := propagateHysteresisProfile(PathState{}, elevProfile, segDist, cfg)

	// Both segments climb 5m (5000mm). First segment: buffer goes from 0 → 5000,
	// exceeds penBuf (2000), so UpWeight should be >0. Second segment: similarly.
	// The distance-weighted average should be >0.
	if hr.UpWeight <= 0 {
		t.Errorf("expected UpWeight > 0 for climbing profile, got %f", hr.UpWeight)
	}
	if hr.ElevCost <= 0 {
		t.Error("expected non-zero ElevCost for climbing profile")
	}
}
