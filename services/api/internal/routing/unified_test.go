package routing

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// makePassableTestGraph returns a 3-node, 2-edge graph where edges have
// highway=track (value 15, bits 0-4 of Tags1) so they pass cost config checks.
func makePassableTestGraph() *graph.Graph {
	// Highway=track is enum value 15 (0x0F), stored in Tags1 bits 0-4.
	// This is passable for all foot/bike profiles (only hw <= 4 or hw == 0 is blocked).
	const trackTags1 uint64 = 15 // highway=track
	g := &graph.Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 2},
		EdgeDist:        []uint32{10000, 20000}, // 100m, 200m
		EdgeTags1:       []uint64{trackTags1, trackTags1},
		EdgeTags2:       []uint64{0, 0},
		EdgeOSMIdx:      []uint32{0, 0},
		OSMIDPool:       []int64{1},
		NodeLat:         []int32{470500000, 470600000, 470700000},
		NodeLon:         []int32{80500000, 80600000, 80700000},
		NodeEle:         []int32{5000, 5100, 5200},
		NodeTags:        []uint8{0, 0, 0},
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, false, false},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	return g
}

func TestFindRouteUnified_BasicRoute(t *testing.T) {
	g := makePassableTestGraph() // 3 nodes, 2 edges, highway=track
	reader := &SingleGraphReader{G: g, TileID: 0}

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 1) // node 1 (target of edge 0)

	cfg := DefaultCostConfig()

	// Pass0
	pass0, err := findRouteUnified(context.Background(), reader, src, dst, cfg, cfg.Pass0Coefficient, 0, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	if pass0.totalCost <= 0 {
		t.Error("pass0 cost should be > 0")
	}

	trackIndex := buildTrackIndexUnified(pass0.state, src, dst)
	if trackIndex == nil {
		t.Fatal("trackIndex should not be nil")
	}

	// Pass1
	pass1, err := findRouteUnified(context.Background(), reader, src, dst, cfg, 0, pass0.totalCost, trackIndex)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	// Retrack
	result := retrackUnified(reader, pass1.state, src, dst)
	if result == nil {
		t.Fatal("retrack returned nil")
	}
	if len(result.Coordinates) < 2 {
		t.Errorf("expected >= 2 coordinates, got %d", len(result.Coordinates))
	}
	if result.DistanceM <= 0 {
		t.Error("distance should be > 0")
	}
}
