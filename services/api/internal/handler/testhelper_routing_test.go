package handler

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

// testRoutingSetup holds everything Phase 4 integration tests need to exercise
// SuggestRoutes end-to-end without a real database or live OSM tiles.
type testRoutingSetup struct {
	Handler  *Handler
	Settings *settings.SettingsStore
	Router   *routing.UnifiedRouter
	TileDir  string
	Cleanup  func()
}

// newTestRoutingHandler creates a minimal but functional *Handler for routing
// integration tests:
//
//  1. Creates a temp directory (cleaned up via t.Cleanup).
//  2. Builds a 5-node synthetic graph and serializes it as a tile file.
//  3. Initialises a TiledGraphReader + GraphManager pointing at that directory.
//  4. Wraps everything in a *Handler with a no-DB SettingsStore.
//
// The returned handler is intentionally lightweight: DB is nil, Events are
// nil, ShadowFS is nil. SuggestRoutes only touches h.GraphManager,
// h.CoverageReader, h.Settings, and h.DB (only when PreferUnknown=true or a
// non-standard profile is requested). Tests that require those paths must wire
// them separately.
func newTestRoutingHandler(t *testing.T) *testRoutingSetup {
	t.Helper()

	tileDir := t.TempDir()

	// --- Build synthetic graph ---
	// 5 nodes arranged as a simple star around 48.99°N, 12.10°E so that
	// FindReachableWithCoords has something to traverse.
	//
	// Coordinates are stored as int32 × 1e7.
	// Distances are in centimetres (uint32).
	// Elevation is in decimetres (int32).
	//
	// Layout (roughly):
	//   node 0: centre        (48.990, 12.100)  ele 5000 dm = 500 m
	//   node 1: north         (48.999, 12.100)  ele 5100 dm
	//   node 2: east          (48.990, 12.110)  ele 5050 dm
	//   node 3: south         (48.981, 12.100)  ele 4950 dm
	//   node 4: west          (48.990, 12.090)  ele 4900 dm
	//
	// Edges: bidirectional ring 0↔1, 0↔2, 0↔3, 0↔4 plus 1↔2, 2↔3, 3↔4, 4↔1
	// so the graph is fully connected and round-trips are possible.
	const nodeCount = 5
	lats := [nodeCount]int32{
		int32(48.990 * 1e7),
		int32(48.999 * 1e7),
		int32(48.990 * 1e7),
		int32(48.981 * 1e7),
		int32(48.990 * 1e7),
	}
	lons := [nodeCount]int32{
		int32(12.100 * 1e7),
		int32(12.100 * 1e7),
		int32(12.110 * 1e7),
		int32(12.100 * 1e7),
		int32(12.090 * 1e7),
	}
	eles := [nodeCount]int32{5000, 5100, 5050, 4950, 4900}

	// Directed edges: from → to, dist_cm, highway_enum=16 (path)
	type edge struct{ from, to, distCm int }
	directedEdges := []edge{
		{0, 1, 100_000}, {1, 0, 100_000}, // 0↔1 ~1 km
		{0, 2, 80_000}, {2, 0, 80_000}, // 0↔2 ~800 m
		{0, 3, 100_000}, {3, 0, 100_000}, // 0↔3 ~1 km
		{0, 4, 80_000}, {4, 0, 80_000}, // 0↔4 ~800 m
		{1, 2, 120_000}, {2, 1, 120_000}, // 1↔2
		{2, 3, 120_000}, {3, 2, 120_000}, // 2↔3
		{3, 4, 120_000}, {4, 3, 120_000}, // 3↔4
		{4, 1, 160_000}, {1, 4, 160_000}, // 4↔1
	}

	// Count out-degree per node.
	degree := make([]uint32, nodeCount)
	for _, e := range directedEdges {
		degree[e.from]++
	}

	edgeCount := uint32(len(directedEdges))
	offsets := make([]uint32, nodeCount+1)
	for i := 0; i < nodeCount; i++ {
		offsets[i+1] = offsets[i] + degree[i]
	}

	targets := make([]uint32, edgeCount)
	edgeDist := make([]uint32, edgeCount)
	edgeTags1 := make([]uint64, edgeCount)
	edgeTags2 := make([]uint64, edgeCount)
	edgeOSMIdx := make([]uint32, edgeCount)
	edgeGeomOff := make([]uint32, edgeCount+1) // all-zero = no intermediate geom
	edgeElevOff := make([]uint32, edgeCount+1)

	cursor := make([]uint32, nodeCount)
	copy(cursor, offsets[:nodeCount])
	for _, e := range directedEdges {
		pos := cursor[e.from]
		cursor[e.from]++
		targets[pos] = uint32(e.to)
		edgeDist[pos] = uint32(e.distCm)
		edgeTags1[pos] = 16 // highway=path enum
	}

	g := &graph.Graph{
		NodeCount:       nodeCount,
		EdgeCount:       edgeCount,
		Version:         graph.FormatVersion,
		Offsets:         offsets,
		Targets:         targets,
		EdgeDist:        edgeDist,
		EdgeTags1:       edgeTags1,
		EdgeTags2:       edgeTags2,
		EdgeOSMIdx:      edgeOSMIdx,
		OSMIDPool:       []int64{99999},
		NodeLat:         lats[:],
		NodeLon:         lons[:],
		NodeEle:         eles[:],
		NodeTags:        make([]uint8, nodeCount),
		IsBorder:        make([]bool, nodeCount),
		EdgeGeomOff:     edgeGeomOff,
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     edgeElevOff,
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
	}

	// Place the graph inside tile 48N/12E.
	tileID := graph.TileIDFromCoord(48.0, 12.0)
	g.TileID = tileID
	bbox := graph.TileBBox(tileID)
	g.BBox = bbox

	g.Grid = graph.BuildSpatialIndex(g, 0.01)

	// Serialize to the tile file name that TiledGraphReader expects:
	// tile_{lat}_{lon}.bin  (same formula as the unexported tileName function).
	latFloor := int(tileID/360) - 90
	lonFloor := int(tileID%360) - 180
	tileFileName := "tile_" + strconv.Itoa(latFloor) + "_" + strconv.Itoa(lonFloor) + ".bin"
	tilePath := filepath.Join(tileDir, tileFileName)

	if err := graph.Serialize(g, tilePath); err != nil {
		t.Fatalf("newTestRoutingHandler: serialize tile: %v", err)
	}

	// --- Routing stack ---
	mgr := &routing.GraphManager{}
	loaded, err := mgr.InitTiledReader(tileDir, 100_000_000)
	if err != nil {
		t.Fatalf("newTestRoutingHandler: InitTiledReader: %v", err)
	}
	if !loaded {
		t.Fatalf("newTestRoutingHandler: InitTiledReader: expected tiles to be found in %s", tileDir)
	}

	router := mgr.UnifiedRouter()
	if router == nil {
		t.Fatal("newTestRoutingHandler: UnifiedRouter is nil after InitTiledReader")
	}

	// --- Settings (no-DB, in-memory) ---
	// Provide sensible defaults so code paths that call h.Settings.Get don't panic.
	store := settings.NewStoreForTest(map[string]string{
		"routing.route_variants_v2": "false",
		"limits.max_upload_bytes":   fmt.Sprintf("%d", 256*1024*1024),
	})

	// --- Handler ---
	h := &Handler{
		// DB: nil — SuggestRoutes only touches DB for PreferUnknown and custom
		// profiles.  Tests that exercise those paths must set h.DB themselves.
		GraphManager:   mgr,
		CoverageReader: nil, // coverage unavailable → coverageAvailable=false
		Settings:       store,
	}

	cleanup := func() {
		// t.TempDir() already handles deletion; nothing extra needed.
	}
	t.Cleanup(cleanup)

	return &testRoutingSetup{
		Handler:  h,
		Settings: store,
		Router:   router,
		TileDir:  tileDir,
		Cleanup:  cleanup,
	}
}

// TestNewTestRoutingHandler_Smoke verifies that the test harness boots without
// panicking and that all required components are non-nil.
func TestNewTestRoutingHandler_Smoke(t *testing.T) {
	setup := newTestRoutingHandler(t)

	if setup.Handler == nil {
		t.Fatal("handler is nil")
	}
	if setup.Router == nil {
		t.Fatal("router is nil")
	}
	if setup.Settings == nil {
		t.Fatal("settings store is nil")
	}
	if setup.TileDir == "" {
		t.Fatal("tile dir is empty")
	}
	if setup.Handler.GraphManager == nil {
		t.Fatal("handler.GraphManager is nil")
	}
	if setup.Handler.GraphManager.UnifiedRouter() == nil {
		t.Fatal("handler.GraphManager.UnifiedRouter() is nil")
	}

	// Verify that the Settings store returns the expected defaults.
	if v := setup.Settings.Get("routing.route_variants_v2").Value; v != "false" {
		t.Errorf("expected routing.route_variants_v2=false, got %q", v)
	}
}
