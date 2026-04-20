package routing

import (
	"context"
	"testing"
)

func TestRouteResultAvgFlat(t *testing.T) {
	r := &RouteResult{DistSum: 1000, QualitySum: 1200}
	got := r.AvgFlat()
	if got != 1.2 {
		t.Errorf("AvgFlat = %v, want 1.2", got)
	}
}

func TestRouteResultAvgFlatEmpty(t *testing.T) {
	r := &RouteResult{}
	if r.AvgFlat() != 0 {
		t.Errorf("empty route AvgFlat = %v, want 0", r.AvgFlat())
	}
}

// TestRetrackUnified_PopulatesEdgeSlices verifiziert, dass retrackUnified
// EdgeCoordStart/EdgeTags1/EdgeTags2 mit Länge == len(EdgeHandles) befüllt
// und die CoordStart-Werte monoton nicht-fallend sind.
func TestRetrackUnified_PopulatesEdgeSlices(t *testing.T) {
	// 4-node chain: 3 edges (0→1→2→3)
	g := makeChainGraph(4)
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 3)

	pass0, err := findRouteUnified(context.Background(), reader, nil, nil, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	trackIndex := buildTrackIndexUnified(pass0.state, dst, 0)

	pass1, err := findRouteUnified(context.Background(), reader, nil, nil, src, dst, cfg, 0, pass0.totalCost*cfg.Pass0Coefficient*cfg.Pass0Coefficient, trackIndex, nil, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	result := retrackUnified(reader, pass1.state, pass1.bestDstNode, nil, nil, cfg)
	if result == nil {
		t.Fatal("retrackUnified returned nil")
	}

	nEdges := len(result.EdgeHandles)
	if nEdges == 0 {
		t.Fatal("EdgeHandles is empty — no edges in route")
	}

	if got, want := len(result.EdgeCoordStart), nEdges; got != want {
		t.Fatalf("EdgeCoordStart len = %d, want %d (== len(EdgeHandles))", got, want)
	}
	if got, want := len(result.EdgeTags1), nEdges; got != want {
		t.Fatalf("EdgeTags1 len = %d, want %d (== len(EdgeHandles))", got, want)
	}
	if got, want := len(result.EdgeTags2), nEdges; got != want {
		t.Fatalf("EdgeTags2 len = %d, want %d (== len(EdgeHandles))", got, want)
	}

	// CoordStart must be monoton nicht-fallend (zero-length edges may share coord start)
	for i := 1; i < len(result.EdgeCoordStart); i++ {
		if result.EdgeCoordStart[i] < result.EdgeCoordStart[i-1] {
			t.Fatalf("EdgeCoordStart not monotonic at i=%d: %d < %d",
				i, result.EdgeCoordStart[i], result.EdgeCoordStart[i-1])
		}
	}

	// Last CoordStart must be a valid index into Coordinates (< len)
	last := result.EdgeCoordStart[len(result.EdgeCoordStart)-1]
	if last >= len(result.Coordinates) {
		t.Fatalf("last EdgeCoordStart %d >= len(Coordinates) %d", last, len(result.Coordinates))
	}
}

func TestMergeLegs_ShiftsEdgeCoordStartAndAppendsTags(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1, Lon: 1}, {Lat: 2, Lon: 2}, {Lat: 3, Lon: 3}},
		EdgeHandles:    []uint64{100, 101},
		EdgeCoordStart: []int{0, 1},
		EdgeTags1:      []uint64{0xAA, 0xBB},
		EdgeTags2:      []uint64{0x11, 0x22},
	}
	legB := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 3, Lon: 3}, {Lat: 4, Lon: 4}}, // dup first
		EdgeHandles:    []uint64{200},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xCC},
		EdgeTags2:      []uint64{0x33},
	}
	merged := mergeLegs([]*RouteResult{legA, legB})

	// Coordinates: legA(3) + legB[1:](1) == 4
	if got, want := len(merged.Coordinates), 4; got != want {
		t.Fatalf("Coordinates len = %d, want %d", got, want)
	}
	// EdgeHandles: 2 + 1 == 3
	if got, want := len(merged.EdgeHandles), 3; got != want {
		t.Fatalf("EdgeHandles len = %d, want %d", got, want)
	}
	// EdgeCoordStart: [0, 1, 2]  (legB's 0 shifted by coordsBefore-1 == 3-1 == 2)
	want := []int{0, 1, 2}
	for i, v := range want {
		if merged.EdgeCoordStart[i] != v {
			t.Fatalf("EdgeCoordStart[%d] = %d, want %d", i, merged.EdgeCoordStart[i], v)
		}
	}
	// Tags appended 1:1
	if merged.EdgeTags1[2] != 0xCC {
		t.Fatalf("EdgeTags1[2] = %x, want 0xCC", merged.EdgeTags1[2])
	}
	if merged.EdgeTags2[2] != 0x33 {
		t.Fatalf("EdgeTags2[2] = %x, want 0x33", merged.EdgeTags2[2])
	}
}

func TestMergeLegs_EmptyFollowLegSkipsCoordStartShift(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1}, {Lat: 2}},
		EdgeHandles:    []uint64{100},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xAA},
		EdgeTags2:      []uint64{0x11},
	}
	legB := &RouteResult{} // empty coords, empty slices

	merged := mergeLegs([]*RouteResult{legA, legB})
	// len(EdgeCoordStart) stays 1; no stray shifted zero was appended
	if got, want := len(merged.EdgeCoordStart), 1; got != want {
		t.Fatalf("EdgeCoordStart len = %d, want %d", got, want)
	}
	if merged.EdgeCoordStart[0] != 0 {
		t.Fatalf("EdgeCoordStart[0] = %d, want 0", merged.EdgeCoordStart[0])
	}
}

// TestMergeLegs_EmptyCoordsWithTags_InvariantSafe prüft den pathologischen Fall:
// ein Folge-Leg hat keine Coordinates aber nicht-leere EdgeTags1/EdgeTags2.
// Invariant: len(EdgeTags1) == len(EdgeTags2) == len(EdgeCoordStart) nach dem Merge.
// Vor dem Fix führte das unconditional append von Tags zu einem out-of-range Panic
// in computeScaleRanges / contiguousRangesFromMatch (Iteration über edgeTags1 mit
// edgeCoordStart[i] ohne length-guard).
func TestMergeLegs_EmptyCoordsWithTags_InvariantSafe(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1}, {Lat: 2}},
		EdgeHandles:    []uint64{100},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xAA},
		EdgeTags2:      []uint64{0x11},
	}
	// Pathological leg: no coords but has tags — should NOT append tags
	// (empty leg = no edges to merge; tags without a CoordStart anchor are bogus).
	legB := &RouteResult{
		Coordinates:    []LatLonEle{},
		EdgeHandles:    []uint64{200},
		EdgeCoordStart: []int{},
		EdgeTags1:      []uint64{0xCC},
		EdgeTags2:      []uint64{0x33},
	}
	merged := mergeLegs([]*RouteResult{legA, legB})

	// Invariant: all three per-edge slices must have the same length.
	nCoordStart := len(merged.EdgeCoordStart)
	if got := len(merged.EdgeTags1); got != nCoordStart {
		t.Fatalf("invariant broken: len(EdgeTags1)=%d != len(EdgeCoordStart)=%d", got, nCoordStart)
	}
	if got := len(merged.EdgeTags2); got != nCoordStart {
		t.Fatalf("invariant broken: len(EdgeTags2)=%d != len(EdgeCoordStart)=%d", got, nCoordStart)
	}
	// legA contributes exactly 1 edge; legB (empty coords) contributes nothing.
	if nCoordStart != 1 {
		t.Fatalf("EdgeCoordStart len = %d, want 1 (legA only)", nCoordStart)
	}
	// legA's tags must be intact.
	if merged.EdgeTags1[0] != 0xAA {
		t.Fatalf("EdgeTags1[0] = %x, want 0xAA", merged.EdgeTags1[0])
	}
	if merged.EdgeTags2[0] != 0x11 {
		t.Fatalf("EdgeTags2[0] = %x, want 0x11", merged.EdgeTags2[0])
	}
	// EdgeHandles is NOT length-synchronized with EdgeCoordStart (used by
	// aggregateLegOutdoor/aggregateLeg2Coverage which iterate handles independently
	// via reader.EdgeTags*(h)). So we accept EdgeHandles may have len=2 here.
}
