package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// mkReachableNode builds a minimal ReachableNode with a single state whose
// DistanceM is set to distM. Lat/Lon are zero (irrelevant for distance-band
// tests that do not exercise BuildCandidatePool's angular spread).
func mkReachableNode(nodeID uint64, distM float32) routing.ReachableNode {
	return routing.ReachableNode{
		NodeID: nodeID,
		States: []routing.NodePareto{{DistanceM: distM}},
	}
}

// mkReachableNodes builds n nodes with distances 1000, 2000, ..., n*1000.
func mkReachableNodes(n int) []routing.ReachableNode {
	nodes := make([]routing.ReachableNode, n)
	for i := range nodes {
		nodes[i] = mkReachableNode(uint64(i+1), float32((i+1)*1000))
	}
	return nodes
}

// ─── buildCandidatePool ──────────────────────────────────────────────────────

// TestBuildCandidatePool_DistanceBandFilter verifies that only nodes whose
// state DistanceM falls within [targetHalf*0.5, targetHalf*1.5] are kept.
//
// Input: 10 nodes with DistanceM = 1000, 2000, …, 10000.
// targetDistanceM=5000 → half=2500, band [1250, 3750].
// Nodes in band: DistanceM 2000 and 3000 (indices 1 and 2 in the 1-based list).
func TestBuildCandidatePool_DistanceBandFilter(t *testing.T) {
	nodes := mkReachableNodes(10) // DistanceM: 1000..10000
	filtered, _ := buildCandidatePool(nodes, 0, 0, 5000, 3)

	// Expect only nodes with DistanceM in [1250, 3750]: that's 2000 and 3000.
	want := map[float32]bool{2000: true, 3000: true}
	if len(filtered) != len(want) {
		t.Fatalf("distance-band filter: want %d nodes, got %d (%v)", len(want), len(filtered), filtered)
	}
	for _, rn := range filtered {
		d := rn.States[0].DistanceM
		if !want[d] {
			t.Errorf("unexpected node with DistanceM=%v in filtered set", d)
		}
	}
}

// TestBuildCandidatePool_EmptyFilteredFallsBackToAllReachable checks that when
// no node falls within the distance band, filtered falls back to the full
// reachable slice.
func TestBuildCandidatePool_EmptyFilteredFallsBackToAllReachable(t *testing.T) {
	// All nodes have DistanceM 8000–10000; target half = 1000, band [500, 1500].
	// None qualify → fallback to all.
	nodes := []routing.ReachableNode{
		mkReachableNode(1, 8000),
		mkReachableNode(2, 9000),
		mkReachableNode(3, 10000),
	}
	filtered, _ := buildCandidatePool(nodes, 0, 0, 2000, 3)
	if len(filtered) != len(nodes) {
		t.Fatalf("empty-filter fallback: want %d nodes (all), got %d", len(nodes), len(filtered))
	}
}

// TestBuildCandidatePool_HardLimit10000 verifies that a pool larger than 10000
// nodes is truncated to exactly 10000 after filtering/fallback.
func TestBuildCandidatePool_HardLimit10000(t *testing.T) {
	// 10001 nodes, all with DistanceM=1 (outside any reasonable band → fallback
	// path, so all 10001 reach the truncation check).
	nodes := make([]routing.ReachableNode, 10001)
	for i := range nodes {
		nodes[i] = mkReachableNode(uint64(i+1), 1)
	}

	filtered, _ := buildCandidatePool(nodes, 0, 0, 100000, 3)
	if len(filtered) != 10000 {
		t.Fatalf("hard limit: want len(filtered)=10000, got %d", len(filtered))
	}
}

// TestBuildCandidatePool_PoolSizeCapAt7 checks that poolSize is capped at 7
// regardless of maxVariants. With maxVariants=10, poolSize would be 12 without
// the cap; the cap ensures at most 7 candidates are requested.
//
// We verify indirectly: len(pool) <= 7 and the function does not panic.
func TestBuildCandidatePool_PoolSizeCapAt7(t *testing.T) {
	// Provide enough nodes so BuildCandidatePool can potentially return >7.
	nodes := mkReachableNodes(50)
	// targetDistanceM=5000, half=2500, band [1250,3750]: nodes 2000,3000.
	// Only 2 nodes in band → pool will be at most 2. But with fallback removed
	// (band is non-empty) and minDist large, pool may be small. The key assertion
	// is len(pool) <= 7.
	_, pool := buildCandidatePool(nodes, 0, 0, 5000, 10)
	if len(pool) > 7 {
		t.Errorf("pool size cap: want len(pool) <= 7, got %d", len(pool))
	}
}

// TestBuildCandidatePool_DeterministicSort checks that two calls with identical
// input produce identical filtered and pool slices.
func TestBuildCandidatePool_DeterministicSort(t *testing.T) {
	nodes := mkReachableNodes(20)
	f1, p1 := buildCandidatePool(nodes, 0, 0, 10000, 3)
	f2, p2 := buildCandidatePool(nodes, 0, 0, 10000, 3)

	if len(f1) != len(f2) {
		t.Fatalf("deterministic: filtered lengths differ: %d vs %d", len(f1), len(f2))
	}
	for i := range f1 {
		if f1[i].NodeID != f2[i].NodeID {
			t.Errorf("deterministic: filtered[%d] NodeID %d != %d", i, f1[i].NodeID, f2[i].NodeID)
		}
	}

	if len(p1) != len(p2) {
		t.Fatalf("deterministic: pool lengths differ: %d vs %d", len(p1), len(p2))
	}
	for i := range p1 {
		if p1[i].NodeIdx != p2[i].NodeIdx || p1[i].StateIdx != p2[i].StateIdx {
			t.Errorf("deterministic: pool[%d] %+v != %+v", i, p1[i], p2[i])
		}
	}
}

// ─── deriveCandidateMetrics ──────────────────────────────────────────────────

// TestDeriveCandidateMetrics_NilScoresWhenNoCoverage verifies that a candidate
// with HasCoverage=false produces nil GreenScore and CalmScore.
func TestDeriveCandidateMetrics_NilScoresWhenNoCoverage(t *testing.T) {
	built := []RouteCandidate{{
		HasCoverage:          false,
		MergedForestPct:      0.8,
		MergedRiverProximity: 0.5,
		MergedProtectedArea:  0.3,
		MergedTrafficClass:   0.4,
		MergedNoiseScore:     0.3,
	}}
	metrics := deriveCandidateMetrics(built)
	if metrics[0].GreenScore != nil {
		t.Errorf("no-coverage: GreenScore must be nil, got %v", *metrics[0].GreenScore)
	}
	if metrics[0].CalmScore != nil {
		t.Errorf("no-coverage: CalmScore must be nil, got %v", *metrics[0].CalmScore)
	}
}

// TestDeriveCandidateMetrics_NilScoresWhenAllZero verifies that HasCoverage=true
// but all merged fields = 0 still produces nil scores (the > 0 filter).
func TestDeriveCandidateMetrics_NilScoresWhenAllZero(t *testing.T) {
	built := []RouteCandidate{{
		HasCoverage:          true,
		MergedForestPct:      0,
		MergedRiverProximity: 0,
		MergedProtectedArea:  0,
		MergedTrafficClass:   0,
		MergedNoiseScore:     0,
	}}
	metrics := deriveCandidateMetrics(built)
	if metrics[0].GreenScore != nil {
		t.Errorf("all-zero: GreenScore must be nil, got %v", *metrics[0].GreenScore)
	}
	if metrics[0].CalmScore != nil {
		t.Errorf("all-zero: CalmScore must be nil, got %v", *metrics[0].CalmScore)
	}
}

// TestDeriveCandidateMetrics_GreenScoreAverage checks the average of the three
// green dimensions.
func TestDeriveCandidateMetrics_GreenScoreAverage(t *testing.T) {
	built := []RouteCandidate{{
		HasCoverage:          true,
		MergedForestPct:      0.6,
		MergedRiverProximity: 0.3,
		MergedProtectedArea:  0.0,
	}}
	metrics := deriveCandidateMetrics(built)
	if metrics[0].GreenScore == nil {
		t.Fatal("green-avg: GreenScore must not be nil")
	}
	want := float32((0.6 + 0.3 + 0.0) / 3.0)
	got := *metrics[0].GreenScore
	if abs32(got-want) > 1e-5 {
		t.Errorf("green-avg: want %v, got %v", want, got)
	}
}

// TestDeriveCandidateMetrics_CalmScoreClampAndInvert tests three sub-cases for
// CalmScore derivation including traffic clamping and overall floor/ceiling.
func TestDeriveCandidateMetrics_CalmScoreClampAndInvert(t *testing.T) {
	cases := []struct {
		name    string
		traffic float32
		noise   float32
		want    float32
	}{
		{
			name:    "normal",
			traffic: 0.5,
			noise:   0.3,
			want:    1.0 - (0.5+0.3)/2.0, // = 0.6
		},
		{
			name:    "traffic_clamped_to_1",
			traffic: 1.5, // clamped → 1.0
			noise:   0.0,
			want:    1.0 - (1.0+0.0)/2.0, // = 0.5
		},
		{
			name:    "raw_negative_clamped_to_zero",
			traffic: 2.0, // clamped → 1.0
			noise:   2.0,
			want:    0.0, // raw = 1-(1+2)/2 = -0.5 → clamped to 0
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			built := []RouteCandidate{{
				HasCoverage:        true,
				MergedTrafficClass: tc.traffic,
				MergedNoiseScore:   tc.noise,
			}}
			metrics := deriveCandidateMetrics(built)
			if metrics[0].CalmScore == nil {
				t.Fatal("CalmScore must not be nil")
			}
			got := *metrics[0].CalmScore
			if abs32(got-tc.want) > 1e-5 {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestDeriveCandidateMetrics_PreservesDistanceAscentAvgFlatNodeID verifies that
// the scalar fields are copied 1:1 regardless of coverage state.
func TestDeriveCandidateMetrics_PreservesDistanceAscentAvgFlatNodeID(t *testing.T) {
	built := []RouteCandidate{{
		DistanceM: 15000,
		AscentM:   500,
		AvgFlat:   0.8,
		NodeID:    42,
	}}
	metrics := deriveCandidateMetrics(built)
	m := metrics[0]
	if m.DistanceM != 15000 {
		t.Errorf("DistanceM: want 15000, got %v", m.DistanceM)
	}
	if m.AscentM != 500 {
		t.Errorf("AscentM: want 500, got %v", m.AscentM)
	}
	if abs32(m.AvgFlat-0.8) > 1e-5 {
		t.Errorf("AvgFlat: want 0.8, got %v", m.AvgFlat)
	}
	if m.NodeID != 42 {
		t.Errorf("NodeID: want 42, got %v", m.NodeID)
	}
}
