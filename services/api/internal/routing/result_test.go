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
