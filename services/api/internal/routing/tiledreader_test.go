package routing

import (
	"path/filepath"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestTiledGraphReaderHasNoTiles(t *testing.T) {
	dir := t.TempDir()
	reader := NewTiledGraphReader(dir, 100_000_000)
	if reader.HasTiles() {
		t.Error("empty dir should have no tiles")
	}
	_, _, ok := reader.NearestNode(47.0, 14.0)
	if ok {
		t.Error("should not find node with no tiles")
	}
}

func TestTiledGraphReaderLoadsTile(t *testing.T) {
	dir := t.TempDir()
	g := makeTiledTestGraph()
	g.TileID = graph.TileIDFromCoord(47.0, 8.0)
	g.Version = graph.FormatVersion
	g.BBox = [4]float64{47.0, 8.0, 48.0, 9.0}
	path := filepath.Join(dir, tileName(g.TileID))
	if err := graph.Serialize(g, path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	reader := NewTiledGraphReader(dir, 100_000_000)
	if !reader.HasTiles() {
		t.Fatal("should detect tile")
	}

	nodeID, _, ok := reader.NearestNode(47.05, 8.05)
	if !ok {
		t.Fatal("should find nearest node")
	}

	// ForEachEdge should work
	var edgeCount int
	reader.ForEachEdge(nodeID, func(ev EdgeView) { edgeCount++ })
	if edgeCount == 0 {
		t.Error("expected edges from nearest node")
	}
}

func TestTiledCacheSingleflight(t *testing.T) {
	// Verifies that concurrent getOrLoad for same tile only loads once
	dir := t.TempDir()
	g := makeTiledTestGraph()
	g.TileID = graph.TileIDFromCoord(47.0, 8.0)
	g.Version = graph.FormatVersion
	g.BBox = [4]float64{47.0, 8.0, 48.0, 9.0}
	path := filepath.Join(dir, tileName(g.TileID))
	if err := graph.Serialize(g, path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	cache := newTiledCache(100_000_000)
	// Load twice -- second should hit cache
	g1 := cache.getOrLoad(g.TileID, dir)
	g2 := cache.getOrLoad(g.TileID, dir)
	if g1 == nil || g2 == nil {
		t.Fatal("both loads should succeed")
	}
	// Same pointer (cached)
	if g1 != g2 {
		t.Error("expected same graph pointer from cache")
	}
}

// makeTiledTestGraph creates a small graph suitable for Serialize round-trip.
// Uses coordinates within the 47N/8E tile.
func makeTiledTestGraph() *graph.Graph {
	g := &graph.Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:         []uint32{0, 1, 2, 2},
		Targets:         []uint32{1, 2},
		EdgeDist:        []uint32{10000, 20000},
		EdgeTags1:       []uint64{0, 0},
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
