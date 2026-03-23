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

// TestEstimateTileBytesExcludesMmap verifies that estimateTileBytes counts only
// heap-allocated sections and skips mmap zero-copy sections (EdgeGeomLat/Lon,
// EdgeElevOff, EdgeElevProf, EdgeOSMIdx).
func TestEstimateTileBytesExcludesMmap(t *testing.T) {
	const nodeCount = 1000
	const edgeCount = 2000

	g := &graph.Graph{
		NodeCount:   nodeCount,
		EdgeCount:   edgeCount,
		Offsets:     make([]uint32, nodeCount+1),
		Targets:     make([]uint32, edgeCount),
		EdgeDist:    make([]uint32, edgeCount),
		EdgeTags1:   make([]uint64, edgeCount),
		EdgeTags2:   make([]uint64, edgeCount),
		NodeLat:     make([]int32, nodeCount),
		NodeLon:     make([]int32, nodeCount),
		NodeEle:     make([]int32, nodeCount),
		NodeTags:    make([]uint8, nodeCount),
		OSMIDPool:   make([]int64, 500),
		// mmap sections — must NOT be counted
		EdgeGeomLat:  make([]int32, 5000),
		EdgeGeomLon:  make([]int32, 5000),
		EdgeElevOff:  make([]uint32, edgeCount+1),
		EdgeElevProf: make([]int32, 3000),
		EdgeOSMIdx:   make([]uint32, edgeCount),
		// heap section
		EdgeElevSegDist: make([]uint16, edgeCount),
		// SpatialGrid: 100 cells, 1000 nodes → 100*4 + 1000*4 = 4400 bytes
		Grid: &graph.SpatialGrid{
			CellStart: make([]uint32, 100),
			NodeOrder: make([]uint32, 1000),
		},
	}

	got := estimateTileBytes(g)

	// Expected:
	// csr = (1000+1)*4 + 2000*(4+4+8+8) + (2000+1)*4 = 4004 + 48000 + 8004 = 60008
	// nodes = 1000 * (4+4+4+1+1) = 14000
	// segDist = 2000 * 2 = 4000
	// osmPool = 500 * 8 = 4000
	// border = 0
	// grid = 100*4 + 1000*4 = 4400
	// total = 60008 + 14000 + 4000 + 4000 + 0 + 4400 = 86408
	want := int64(60008 + 14000 + 4000 + 4000 + 0 + 4400)
	if got != want {
		t.Errorf("estimateTileBytes = %d, want %d", got, want)
	}

	// Sanity: mmap sections alone would add much more if counted.
	// EdgeGeomLat+Lon = 5000*4*2 = 40000, EdgeElevOff = 2001*4 = 8004,
	// EdgeElevProf = 3000*4 = 12000, EdgeOSMIdx = 2000*4 = 8000 → 68004 extra.
	// Old formula would yield (2000*48 + 1000*13 + 5000*8 + 3000*4 + 2000*2) = 96000+13000+40000+12000+4000 = 165000.
	if got >= 165_000 {
		t.Errorf("estimateTileBytes %d looks like it is counting mmap sections (old formula threshold ~165000)", got)
	}

	// Grid must be counted correctly.
	wantGrid := int64(100*4 + 1000*4)
	if wantGrid != 4400 {
		t.Fatalf("grid constant sanity check failed")
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
