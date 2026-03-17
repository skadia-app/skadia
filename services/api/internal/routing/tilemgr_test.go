package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func writeDummyTile(t *testing.T, dir string, tileID uint32) {
	t.Helper()
	g := &graph.Graph{
		NodeCount: 3,
		EdgeCount: 2,
		TileID:    tileID,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{1000, 2000},
		EdgeTags1: []uint64{0, 0},
		EdgeTags2: []uint64{0, 0},
		EdgeOSMIdx: []uint32{0, 0},
		OSMIDPool: []int64{12345},
		NodeLat:   []int32{470000000, 473700000, 475000000},
		NodeLon:   []int32{80000000, 85400000, 85000000},
		NodeEle:   []int32{0, 0, 0},
		NodeTags:  []uint8{0, 0, 0},
		EdgeGeomOff: []uint32{0, 0, 0},
		EdgeElevOff: []uint32{0, 0, 0},
	}
	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	path := graph.TileFilePath(dir, tileID)
	if err := graph.Serialize(g, path); err != nil {
		t.Fatal(err)
	}
}

func TestTileManager_LoadTile(t *testing.T) {
	dir := t.TempDir()
	tileID := graph.TileIDFromCoord(47.37, 8.54)
	writeDummyTile(t, dir, tileID)

	mgr, err := NewTileManager(dir, 2_500_000_000)
	if err != nil {
		t.Fatal(err)
	}

	g, err := mgr.GetTile(tileID)
	if err != nil {
		t.Fatal(err)
	}
	if g.NodeCount != 3 {
		t.Errorf("NodeCount = %d, want 3", g.NodeCount)
	}

	g2, err := mgr.GetTile(tileID)
	if err != nil {
		t.Fatal(err)
	}
	if g != g2 {
		t.Error("expected cached pointer identity")
	}
}

func TestTileManager_NoTile(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewTileManager(dir, 2_500_000_000)
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.GetTile(99999)
	if err == nil {
		t.Error("expected error for missing tile")
	}
}
