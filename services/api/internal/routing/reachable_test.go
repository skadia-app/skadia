package routing

import (
	"context"
	"testing"

	"github.com/paulmach/orb"
)

// makeReachableTestGraph creates a 4-node chain graph for reachable tests.
// Uses makeTestGraph from router_test.go (same package).
func makeReachableTestGraph(t *testing.T) GraphReader {
	t.Helper()
	nodes := [][3]float64{
		{47.05, 8.05, 500},   // node 0
		{47.051, 8.051, 510}, // node 1
		{47.052, 8.052, 520}, // node 2
		{47.053, 8.053, 530}, // node 3
	}
	edges := [][4]int{
		{0, 1, 10000, 16}, // 100m, path
		{1, 0, 10000, 16},
		{1, 2, 10000, 16},
		{2, 1, 10000, 16},
		{2, 3, 10000, 16},
		{3, 2, 10000, 16},
	}
	g := makeTestGraph(nodes, edges)
	return &SingleGraphReader{G: g, TileID: 0}
}

func TestFindReachableBasic(t *testing.T) {
	reader := makeReachableTestGraph(t)
	cfg := DefaultCostConfig()

	src, _, ok := reader.NearestNode(47.05, 8.05)
	if !ok {
		t.Fatal("no nearest node")
	}

	nodes, truncated := findReachableLegacy(context.Background(), reader, src, cfg, 100000, 10000)
	if len(nodes) == 0 {
		t.Fatal("expected reachable nodes")
	}
	if truncated {
		t.Error("should not be truncated with high budget")
	}

	// Source node should be in the result with cost 0
	srcNode, ok := nodes[src]
	if !ok {
		t.Fatal("source node should be reachable")
	}
	if srcNode.Cost != 0 {
		t.Errorf("source cost should be 0, got %f", srcNode.Cost)
	}
}

func TestFindReachableTruncation(t *testing.T) {
	reader := makeReachableTestGraph(t)
	cfg := DefaultCostConfig()

	src, _, _ := reader.NearestNode(47.05, 8.05)

	// maxExpansions=1 should truncate immediately
	nodes, truncated := findReachableLegacy(context.Background(), reader, src, cfg, 100000, 1)
	if !truncated {
		t.Error("should be truncated with maxExpansions=1")
	}
	_ = nodes
}

func TestAlphaShapeConcavity(t *testing.T) {
	points := []orb.Point{
		{14.51, 46.06}, {14.52, 46.06}, {14.53, 46.06},
		{14.53, 46.05}, {14.53, 46.04},
		{14.51, 46.04}, {14.52, 46.04},
		{14.52, 46.05},
	}
	ring := AlphaShape(points, 0)
	if ring == nil {
		t.Fatal("expected non-nil alpha shape")
	}
	if len(ring) < 4 {
		t.Errorf("expected at least 4 ring points, got %d", len(ring))
	}
}

func TestAlphaShapeTooFewPoints(t *testing.T) {
	ring := AlphaShape([]orb.Point{{0, 0}, {1, 0}}, 0)
	if ring != nil {
		t.Error("expected nil for 2 points")
	}
}

func TestAlphaShapeLargeAlpha(t *testing.T) {
	points := []orb.Point{
		{0, 0}, {1, 0}, {0.5, 1}, {0.5, 0.3},
	}
	ring := AlphaShape(points, 1e10)
	if ring == nil {
		t.Fatal("expected non-nil ring")
	}
	if len(ring) < 4 {
		t.Errorf("expected at least 4 ring points, got %d", len(ring))
	}
}

func TestConvexHullBasic(t *testing.T) {
	points := []orb.Point{
		{0, 0}, {1, 0}, {1, 1}, {0, 1}, {0.5, 0.5},
	}
	ring := ConvexHull(points)
	if ring == nil {
		t.Fatal("expected non-nil hull")
	}
	// 4 corner points + close = 5
	if len(ring) != 5 {
		t.Errorf("expected 5 hull points, got %d", len(ring))
	}
}
