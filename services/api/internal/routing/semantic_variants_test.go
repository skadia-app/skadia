package routing

import (
	"math"
	"testing"
)

func TestScoreShortest_PicksNearestToTarget(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{DistanceM: 3000}, // 3 km
			{DistanceM: 5000}, // 5 km — closest to target
			{DistanceM: 8000}, // 8 km
		},
	}
	ctx := VariantSelectionContext{TargetDistanceM: 10000} // → half-target 5 km
	score, idx := scoreShortest(node, ctx)
	if idx != 1 {
		t.Errorf("expected state index 1 (5km), got %d (score %v)", idx, score)
	}
}

func TestScoreShortest_ZeroTarget(t *testing.T) {
	node := ReachableNode{States: []NodePareto{{DistanceM: 100}}}
	ctx := VariantSelectionContext{TargetDistanceM: 0}
	_, idx := scoreShortest(node, ctx)
	if idx != -1 {
		t.Errorf("zero target should return idx -1, got %d", idx)
	}
}

func TestScoreShortest_NegativeTarget(t *testing.T) {
	node := ReachableNode{States: []NodePareto{{DistanceM: 100}}}
	ctx := VariantSelectionContext{TargetDistanceM: -1000}
	_, idx := scoreShortest(node, ctx)
	if idx != -1 {
		t.Errorf("negative target should return idx -1, got %d", idx)
	}
}

func TestScoreCalmest_LowTrafficWins(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{AvgFlat: 1.0, TrafficClass: 1.0, NoiseScore: 1.0}, // noisy
			{AvgFlat: 1.0, TrafficClass: 0.0, NoiseScore: 0.0}, // quiet
		},
	}
	ctx := VariantSelectionContext{}
	_, idx := scoreCalmest(node, ctx)
	if idx != 1 {
		t.Errorf("quiet state should win, got idx %d", idx)
	}
}

func TestScoreCalmest_EmptyStates(t *testing.T) {
	node := ReachableNode{States: nil}
	ctx := VariantSelectionContext{}
	score, idx := scoreCalmest(node, ctx)
	if idx != -1 {
		t.Errorf("empty states should return idx -1, got %d", idx)
	}
	if !math.IsInf(score, -1) {
		t.Errorf("empty states should return -Inf score, got %v", score)
	}
}

func TestScoreGreenest_ForestWins(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{ForestPct: 0.2, RiverProximity: 0.1, ProtectedArea: 0.0}, // urban
			{ForestPct: 0.9, RiverProximity: 0.5, ProtectedArea: 0.8}, // green
		},
	}
	_, idx := scoreGreenest(node, VariantSelectionContext{})
	if idx != 1 {
		t.Errorf("green state should win, got idx %d", idx)
	}
}

func TestScoreGreenest_Weights(t *testing.T) {
	// forest=0.5, river=0.25, protected=0.25
	// state0: 0.8*0.5 + 0.0*0.25 + 0.0*0.25 = 0.4
	// state1: 0.0*0.5 + 1.0*0.25 + 1.0*0.25 = 0.5 — wins
	node := ReachableNode{
		States: []NodePareto{
			{ForestPct: 0.8, RiverProximity: 0.0, ProtectedArea: 0.0},
			{ForestPct: 0.0, RiverProximity: 1.0, ProtectedArea: 1.0},
		},
	}
	_, idx := scoreGreenest(node, VariantSelectionContext{})
	if idx != 1 {
		t.Errorf("river+protected combo should beat solo forest at these values, got idx %d", idx)
	}
}

func TestScoreHilly_NoTarget_MaxAscent(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{Ascent: 100}, // flat
			{Ascent: 500}, // hilly
		},
	}
	_, idx := scoreHilly(node, VariantSelectionContext{})
	if idx != 1 {
		t.Errorf("max-ascent should win when no target, got idx %d", idx)
	}
}

func TestScoreHilly_WithTarget_Proximity(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{Ascent: 100},  // far from target
			{Ascent: 300},  // closest
			{Ascent: 1000}, // overshoot
		},
	}
	target := 300.0
	ctx := VariantSelectionContext{TargetAscentM: &target}
	_, idx := scoreHilly(node, ctx)
	if idx != 1 {
		t.Errorf("closest-to-target should win, got idx %d", idx)
	}
}

func TestScoreHilly_ZeroTarget_NoStates(t *testing.T) {
	node := ReachableNode{States: nil}
	ctx := VariantSelectionContext{}
	score, idx := scoreHilly(node, ctx)
	if idx != -1 {
		t.Errorf("empty states should return idx -1, got %d", idx)
	}
	if !math.IsInf(score, -1) {
		t.Errorf("empty states should return -Inf score, got %v", score)
	}
}

func TestScoreBalanced_RespectsLandscapePrefs(t *testing.T) {
	node := ReachableNode{
		States: []NodePareto{
			{DistanceM: 5000, AvgFlat: 2.0, ForestPct: 0.1, NoiseScore: 0.8},
			{DistanceM: 5000, AvgFlat: 1.0, ForestPct: 0.9, NoiseScore: 0.1},
		},
	}
	prefs := &LandscapePrefsInternal{Forest: 10, LowNoise: 10}
	ctx := VariantSelectionContext{
		TargetDistanceM: 10000,
		LandscapePrefs:  prefs,
	}
	_, idx := scoreBalanced(node, ctx)
	if idx != 1 {
		t.Errorf("green+quiet state should win with strong prefs, got idx %d", idx)
	}
}

func TestScoreBalanced_ZeroTarget(t *testing.T) {
	node := ReachableNode{States: []NodePareto{{DistanceM: 5000}}}
	ctx := VariantSelectionContext{TargetDistanceM: 0}
	score, idx := scoreBalanced(node, ctx)
	if idx != -1 {
		t.Errorf("zero target should return idx -1, got %d", idx)
	}
	if !math.IsInf(score, -1) {
		t.Errorf("zero target should return -Inf score, got %v", score)
	}
}

func TestScoreBalanced_NoPrefs(t *testing.T) {
	// Without prefs, coverageScore=0 — balanced reduces to distProx*0.4 + quality*0.3
	// state0: distProx=1.0 (exactly on target), quality=10-1=9 → 0.4 + 2.7 = 3.1
	// state1: distProx=0.5 (off by 50%), quality=10-5=5  → 0.2 + 1.5 = 1.7
	node := ReachableNode{
		States: []NodePareto{
			{DistanceM: 5000, AvgFlat: 1.0},
			{DistanceM: 2500, AvgFlat: 5.0},
		},
	}
	ctx := VariantSelectionContext{TargetDistanceM: 10000}
	_, idx := scoreBalanced(node, ctx)
	if idx != 0 {
		t.Errorf("on-target+low-flat state should win without prefs, got idx %d", idx)
	}
}
