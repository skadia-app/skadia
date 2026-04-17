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
		NodeCount: nodeCount,
		EdgeCount: edgeCount,
		Offsets:   make([]uint32, nodeCount+1),
		Targets:   make([]uint32, edgeCount),
		EdgeDist:  make([]uint32, edgeCount),
		EdgeTags1: make([]uint64, edgeCount),
		EdgeTags2: make([]uint64, edgeCount),
		NodeLat:   make([]int32, nodeCount),
		NodeLon:   make([]int32, nodeCount),
		NodeEle:   make([]int32, nodeCount),
		NodeTags:  make([]uint8, nodeCount),
		OSMIDPool: make([]int64, 500),
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

	// Expected (no RevAdj — lazy-built only on ForEachInEdge):
	// csr = (1000+1)*4 + 2000*(4+4+8+8) + (2000+1)*4 = 4004 + 48000 + 8004 = 60008
	// nodes = 1000 * (4+4+4+1+1) = 14000
	// segDist = 2000 * 2 = 4000
	// osmPool = 500 * 8 = 4000
	// border = 0
	// grid = 100*4 + 1000*4 = 4400
	// revAdj = 0 (not yet built)
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

func TestTiledGraphReader_EdgeTags2_CrossTile(t *testing.T) {
	dir := t.TempDir()

	// Build and serialize two tiles, each with distinct EdgeTags2 values.
	tileID1 := graph.TileIDFromCoord(47.0, 8.0)
	tileID2 := graph.TileIDFromCoord(48.0, 9.0)

	makeTagGraph := func(tileID uint32, bbox [4]float64, lat0, lat1, lon0, lon1 int32, tags2 []uint64) *graph.Graph {
		g := &graph.Graph{
			TileID:          tileID,
			Version:         graph.FormatVersion,
			BBox:            bbox,
			NodeCount:       2,
			EdgeCount:       1,
			Offsets:         []uint32{0, 1, 1},
			Targets:         []uint32{1},
			EdgeDist:        []uint32{10000},
			EdgeTags1:       []uint64{0},
			EdgeTags2:       tags2,
			EdgeOSMIdx:      []uint32{0},
			OSMIDPool:       []int64{1},
			NodeLat:         []int32{lat0, lat1},
			NodeLon:         []int32{lon0, lon1},
			NodeEle:         []int32{5000, 5100},
			NodeTags:        []uint8{0, 0},
			EdgeGeomOff:     []uint32{0, 0},
			EdgeGeomLat:     []int32{},
			EdgeGeomLon:     []int32{},
			EdgeElevOff:     []uint32{0, 0},
			EdgeElevProf:    []int32{},
			EdgeElevSegDist: []uint16{},
			IsBorder:        []bool{false, false},
		}
		g.Grid = graph.BuildSpatialIndex(g, 0.01)
		return g
	}

	g1 := makeTagGraph(tileID1, [4]float64{47.0, 8.0, 48.0, 9.0},
		470500000, 470600000, 80500000, 80600000,
		[]uint64{0xAAAA000000000001})
	g2 := makeTagGraph(tileID2, [4]float64{48.0, 9.0, 49.0, 10.0},
		480500000, 480600000, 90500000, 90600000,
		[]uint64{0xBBBB000000000002})

	if err := graph.Serialize(g1, filepath.Join(dir, tileName(tileID1))); err != nil {
		t.Fatalf("serialize tile1: %v", err)
	}
	if err := graph.Serialize(g2, filepath.Join(dir, tileName(tileID2))); err != nil {
		t.Fatalf("serialize tile2: %v", err)
	}

	reader := NewTiledGraphReader(dir, 100_000_000)

	// Tile 1, edge 0 → must return tile1's Tags2 value.
	handle1 := uint64(tileID1)<<32 | 0
	got1 := reader.EdgeTags2(handle1)
	if got1 != 0xAAAA000000000001 {
		t.Errorf("tile1 edge0 Tags2: expected 0xAAAA000000000001, got 0x%X", got1)
	}

	// Tile 2, edge 0 → must return tile2's Tags2 value (different tile).
	handle2 := uint64(tileID2)<<32 | 0
	got2 := reader.EdgeTags2(handle2)
	if got2 != 0xBBBB000000000002 {
		t.Errorf("tile2 edge0 Tags2: expected 0xBBBB000000000002, got 0x%X", got2)
	}

	// Out-of-range edge index → expect 0.
	handleOOB := uint64(tileID1)<<32 | 999
	gotOOB := reader.EdgeTags2(handleOOB)
	if gotOOB != 0 {
		t.Errorf("out-of-range edge: expected 0, got 0x%X", gotOOB)
	}
}

func TestTiledGraphReader_EdgeDistanceCm_CrossTile(t *testing.T) {
	dir := t.TempDir()

	// Build and serialize two tiles, each with a distinct EdgeDist value.
	tileID1 := graph.TileIDFromCoord(47.0, 8.0)
	tileID2 := graph.TileIDFromCoord(48.0, 9.0)

	makeDistGraph := func(tileID uint32, bbox [4]float64, lat0, lat1, lon0, lon1 int32, dist uint32) *graph.Graph {
		g := &graph.Graph{
			TileID:          tileID,
			Version:         graph.FormatVersion,
			BBox:            bbox,
			NodeCount:       2,
			EdgeCount:       1,
			Offsets:         []uint32{0, 1, 1},
			Targets:         []uint32{1},
			EdgeDist:        []uint32{dist},
			EdgeTags1:       []uint64{0},
			EdgeTags2:       []uint64{0},
			EdgeOSMIdx:      []uint32{0},
			OSMIDPool:       []int64{1},
			NodeLat:         []int32{lat0, lat1},
			NodeLon:         []int32{lon0, lon1},
			NodeEle:         []int32{5000, 5100},
			NodeTags:        []uint8{0, 0},
			EdgeGeomOff:     []uint32{0, 0},
			EdgeGeomLat:     []int32{},
			EdgeGeomLon:     []int32{},
			EdgeElevOff:     []uint32{0, 0},
			EdgeElevProf:    []int32{},
			EdgeElevSegDist: []uint16{},
			IsBorder:        []bool{false, false},
		}
		g.Grid = graph.BuildSpatialIndex(g, 0.01)
		return g
	}

	g1 := makeDistGraph(tileID1, [4]float64{47.0, 8.0, 48.0, 9.0},
		470500000, 470600000, 80500000, 80600000, 12345)
	g2 := makeDistGraph(tileID2, [4]float64{48.0, 9.0, 49.0, 10.0},
		480500000, 480600000, 90500000, 90600000, 67890)

	if err := graph.Serialize(g1, filepath.Join(dir, tileName(tileID1))); err != nil {
		t.Fatalf("serialize tile1: %v", err)
	}
	if err := graph.Serialize(g2, filepath.Join(dir, tileName(tileID2))); err != nil {
		t.Fatalf("serialize tile2: %v", err)
	}

	reader := NewTiledGraphReader(dir, 100_000_000)

	// Tile 1, edge 0 → must return tile1's EdgeDist value.
	handle1 := uint64(tileID1)<<32 | 0
	got1 := reader.EdgeDistanceCm(handle1)
	if got1 != 12345 {
		t.Errorf("tile1 edge0 EdgeDistanceCm: expected 12345, got %d", got1)
	}

	// Tile 2, edge 0 → must return tile2's EdgeDist value (different tile).
	handle2 := uint64(tileID2)<<32 | 0
	got2 := reader.EdgeDistanceCm(handle2)
	if got2 != 67890 {
		t.Errorf("tile2 edge0 EdgeDistanceCm: expected 67890, got %d", got2)
	}

	// Out-of-range edge index → expect 0.
	handleOOB := uint64(tileID1)<<32 | 999
	gotOOB := reader.EdgeDistanceCm(handleOOB)
	if gotOOB != 0 {
		t.Errorf("out-of-range edge: expected 0, got %d", gotOOB)
	}
}

func TestTiledGraphReader_EdgeDistanceCm_MissingTile(t *testing.T) {
	dir := t.TempDir()
	// No tiles written → any tile load returns nil.
	reader := NewTiledGraphReader(dir, 100_000_000)

	// Any handle to a missing tile must return 0 without panic.
	missingTileID := graph.TileIDFromCoord(55.0, 25.0)
	handle := uint64(missingTileID)<<32 | 0
	got := reader.EdgeDistanceCm(handle)
	if got != 0 {
		t.Errorf("missing tile: expected 0, got %d", got)
	}

	// EdgeTags2 on missing tile must also return 0.
	gotTags2 := reader.EdgeTags2(handle)
	if gotTags2 != 0 {
		t.Errorf("missing tile Tags2: expected 0, got 0x%X", gotTags2)
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
