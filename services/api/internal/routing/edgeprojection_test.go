package routing

import "testing"

func TestSnapToEdge_Basic(t *testing.T) {
	g := makePassableTestGraph()
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	ve, err := r.snapToEdge(47.05, 8.05, cfg)
	if err != nil {
		t.Fatalf("snapToEdge failed: %v", err)
	}
	if ve == nil {
		t.Fatal("expected non-nil VirtualEndpoint")
	}
	if ve.GlobalA == 0 && ve.GlobalB == 0 {
		t.Error("expected non-zero global node IDs")
	}
	if ve.CostA < 0 || ve.CostB < 0 {
		t.Errorf("costs should be >= 0: CostA=%.4f, CostB=%.4f", ve.CostA, ve.CostB)
	}
	if ve.StateA.BestCost != ve.CostA {
		t.Errorf("StateA.BestCost (%.4f) != CostA (%.4f)", ve.StateA.BestCost, ve.CostA)
	}
	if ve.StateB.BestCost != ve.CostB {
		t.Errorf("StateB.BestCost (%.4f) != CostB (%.4f)", ve.StateB.BestCost, ve.CostB)
	}
}

func TestSnapToEdge_ProfileFilter(t *testing.T) {
	g := makePassableTestGraph()
	// Set all edges to motorway (highway=1) which is blocked by trekking profile.
	for i := range g.EdgeTags1 {
		g.EdgeTags1[i] = 1 // motorway
	}
	reader := &SingleGraphReader{G: g, TileID: 0}
	r := &UnifiedRouter{reader: reader, sem: make(chan struct{}, 1)}
	cfg := DefaultCostConfig()

	_, err := r.snapToEdge(47.05, 8.05, cfg)
	if err == nil {
		t.Error("expected ErrNoRoute when all edges are blocked")
	}
}
