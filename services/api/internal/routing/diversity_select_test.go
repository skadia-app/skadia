package routing

import (
	"reflect"
	"testing"
)

func ptrF64(v float64) *float64 { return &v }

func TestPickDiverseIndices_LessOrEqualK(t *testing.T) {
	metrics := []CandidateMetrics{
		{DistanceM: 1000, AscentM: 100, AvgFlat: 0.5, NodeID: 1},
		{DistanceM: 2000, AscentM: 200, AvgFlat: 0.6, NodeID: 2},
		{DistanceM: 3000, AscentM: 300, AvgFlat: 0.7, NodeID: 3},
	}
	got := PickDiverseIndices(metrics, 1500, nil, 5)
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestPickDiverseIndices_Deterministic(t *testing.T) {
	metrics := []CandidateMetrics{
		{DistanceM: 1000, AscentM: 100, AvgFlat: 0.5, NodeID: 10},
		{DistanceM: 3000, AscentM: 300, AvgFlat: 0.8, NodeID: 20},
		{DistanceM: 5000, AscentM: 500, AvgFlat: 0.2, NodeID: 30},
		{DistanceM: 7000, AscentM: 700, AvgFlat: 0.9, NodeID: 40},
		{DistanceM: 9000, AscentM: 900, AvgFlat: 0.4, NodeID: 50},
	}
	got1 := PickDiverseIndices(metrics, 5000, nil, 3)
	got2 := PickDiverseIndices(metrics, 5000, nil, 3)
	if !reflect.DeepEqual(got1, got2) {
		t.Errorf("non-deterministic: first call %v, second call %v", got1, got2)
	}
}

func TestPickDiverseIndices_Seed2Skipped(t *testing.T) {
	// Construct 3 candidates where anchor 1 is picked for distance,
	// and the "best ascent" candidate is nearly identical in Z-score space.
	// All three have nearly the same ascent → euclideanZDist < 0.5 → anchor2 rejected.
	metrics := []CandidateMetrics{
		{DistanceM: 5000, AscentM: 100.0, AvgFlat: 0.5, NodeID: 1}, // anchor1 (closest to target dist)
		{DistanceM: 6000, AscentM: 100.1, AvgFlat: 0.5, NodeID: 2}, // candidate for anchor2
		{DistanceM: 4000, AscentM: 100.2, AvgFlat: 0.5, NodeID: 3},
	}
	targetAscent := 100.1
	// With k=2, anchor2 should be tested; since all ascent values are nearly identical,
	// Z-score distance is effectively zero → anchor2 is skipped → max-min fills the slot.
	got := PickDiverseIndices(metrics, 5000, &targetAscent, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(got), got)
	}
	// Anchor1 must be included (index 0 = NodeID 1, closest to 5000m).
	found := false
	for _, idx := range got {
		if idx == 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("anchor1 (index 0) must be in result, got %v", got)
	}
}

func TestPickDiverseIndices_AllSame(t *testing.T) {
	// All metrics identical except NodeID → all Z-scores = 0 → all euclideanZDist = 0
	// → NodeID tiebreaker decides → smallest NodeIDs win.
	metrics := []CandidateMetrics{
		{DistanceM: 5000, AscentM: 300, AvgFlat: 0.5, NodeID: 30},
		{DistanceM: 5000, AscentM: 300, AvgFlat: 0.5, NodeID: 10},
		{DistanceM: 5000, AscentM: 300, AvgFlat: 0.5, NodeID: 20},
		{DistanceM: 5000, AscentM: 300, AvgFlat: 0.5, NodeID: 40},
	}
	got := PickDiverseIndices(metrics, 5000, nil, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(got), got)
	}
	// Anchor1: smallest |DistanceM - 5000| = 0 for all, tiebreak by NodeID asc → NodeID=10 → index 1
	// Fill: all remaining have minDist=0, NodeID tiebreak → NodeID=20 → index 2
	want := []int{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("expected %v (NodeID tiebreak), got %v", want, got)
	}
}

func TestPickDiverseIndices_MixedCoverage(t *testing.T) {
	// 4 candidates, 2 with GreenScore, 2 nil → median imputation activates Green dim.
	gs1 := float32(0.8)
	gs2 := float32(0.2)
	metrics := []CandidateMetrics{
		{DistanceM: 1000, AscentM: 100, AvgFlat: 0.5, GreenScore: &gs1, NodeID: 1},
		{DistanceM: 5000, AscentM: 500, AvgFlat: 0.3, GreenScore: nil, NodeID: 2},
		{DistanceM: 9000, AscentM: 900, AvgFlat: 0.7, GreenScore: &gs2, NodeID: 3},
		{DistanceM: 3000, AscentM: 300, AvgFlat: 0.6, GreenScore: nil, NodeID: 4},
	}
	got1 := PickDiverseIndices(metrics, 5000, nil, 3)
	got2 := PickDiverseIndices(metrics, 5000, nil, 3)
	if len(got1) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(got1), got1)
	}
	if !reflect.DeepEqual(got1, got2) {
		t.Errorf("non-deterministic with mixed coverage: %v vs %v", got1, got2)
	}
}
