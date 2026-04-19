package handler

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
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
//  2. Builds a 25-node grid graph (5×5) spread over ~5km×5km inside the
//     48–49°N / 12–13°E tile and serializes it as a tile file.
//  3. Initialises a TiledGraphReader + GraphManager pointing at that directory.
//  4. Wraps everything in a *Handler with a no-DB SettingsStore.
//
// Grid layout — origin at top-left, growing south and east:
//
//	Row/Col  0        1        2        3        4
//	  0   (48.990, 12.082)  START  …  …  (48.990, 12.154)
//	  1   (48.981, 12.082)   …     …  …  …
//	  2   (48.972, 12.082)   …     …  …  …
//	  3   (48.963, 12.082)   …     …  …  …
//	  4   (48.954, 12.082)   …     …  …  (48.954, 12.154)
//
// node(row, col):
//
//	lat = 48.990 − row×0.009°   (≈ 1 001 m/step south)
//	lon = 12.082 + col×0.018°   (≈ 1 311 m/step east at 49°N)
//
// START = node(0,1) = (48.990, 12.100) matches the test request [12.10, 48.99].
// All node latitudes lie in [48.954, 48.990] — well inside the 48°N tile so
// TiledGraphReader.tile() never crosses into the 49°N tile.
//
// Edges: grid adjacency (horizontal + vertical) between neighbours, plus
// every-other diagonal for disjoint round-trip paths. Distances are derived
// from the real-world step sizes. The bottom-right quadrant (row≥2, col≥2)
// gets a descending elevation to create ascent when returning. ForestPct=70
// is encoded in EdgeTags2 for edges entirely in that quadrant so that the
// coverage-aware descriptor test can assert on high_forest.
//
// The returned handler has CoverageReader=nil (coverageAvailable=false).
// For coverage-enabled tests use newTestRoutingHandlerWithCoverage.
func newTestRoutingHandler(t *testing.T) *testRoutingSetup {
	t.Helper()
	return newTestRoutingHandlerOpts(t, false, false)
}

// newTestRoutingHandlerWithCoverage is like newTestRoutingHandler but sets
// h.CoverageReader to a non-nil stub so that coverageAvailable=true and
// coverage-based descriptors are computed from the EdgeTags2 forest bits
// pre-encoded in the bottom-right quadrant edges.
//
// The stub DiskCoverageReader points at an empty temp directory; it will
// return errors/zeros for any actual tile read, but SuggestRoutes reads
// coverage exclusively from EdgeTags2 and only needs CoverageReader≠nil
// to set the coverageAvailable flag.
func newTestRoutingHandlerWithCoverage(t *testing.T) *testRoutingSetup {
	t.Helper()
	return newTestRoutingHandlerOpts(t, true, false)
}

// newTestRoutingHandlerWithOutdoor is like newTestRoutingHandler but injects
// outdoor tags (mtb/sac/via_ferrata/trail_visibility) into specific edges of
// the grid fixture so that outdoor descriptors and segment annotations are
// exercised in integration tests.
func newTestRoutingHandlerWithOutdoor(t *testing.T) *testRoutingSetup {
	t.Helper()
	return newTestRoutingHandlerOpts(t, false, true)
}

// newTestRoutingHandlerOpts is the shared implementation.
func newTestRoutingHandlerOpts(t *testing.T, withCoverage, withOutdoorTags bool) *testRoutingSetup {
	t.Helper()

	tileDir := t.TempDir()

	// ---------------------------------------------------------------------------
	// 5×5 grid graph.
	//
	// Coordinate scheme:
	//   node(row, col): lat = 48.990 − row×0.009°, lon = 12.082 + col×0.018°
	//
	// At ~49°N:
	//   0.009° lat ≈ 1 001 m  (using 111_195 m/°)
	//   0.018° lon ≈ 0.018 × 111_195 × cos(49°) ≈ 1 311 m
	//
	// All latitudes 48.954–48.990 are strictly south of 49°N, so
	// TileIDFromCoord always returns tile 48_12 for every node.
	// ---------------------------------------------------------------------------
	const (
		gridRows = 5
		gridCols = 5
		nNodes   = gridRows * gridCols // 25
	)

	const (
		topLat  = 48.990 // lat of row 0
		leftLon = 12.082 // lon of col 0
		latStep = 0.009  // degrees south per row  ≈ 1 001 m
		lonStep = 0.018  // degrees east  per col  ≈ 1 311 m
	)

	// Real-world edge distances in centimetres.
	const (
		distVertCm  = 100_100 // latStep × 111_195 m/°
		distHorizCm = 131_100 // lonStep × 111_195 × cos(49°) m/°
		distDiagCm  = 164_600 // √(100_100²+131_100²)
	)

	// nodeIdx converts (row, col) to a flat node index.
	nodeIdx := func(row, col int) int { return row*gridCols + col }

	// Build node coordinates and base elevations.
	nodeLat := make([]int32, nNodes)
	nodeLon := make([]int32, nNodes)
	nodeEle := make([]int32, nNodes) // decimetres

	for r := 0; r < gridRows; r++ {
		for c := 0; c < gridCols; c++ {
			idx := nodeIdx(r, c)
			lat := topLat - float64(r)*latStep
			lon := leftLon + float64(c)*lonStep
			nodeLat[idx] = int32(lat * 1e7)
			nodeLon[idx] = int32(lon * 1e7)

			// Elevation: flat top rows; bottom-right quadrant (r≥2, c≥2) is elevated
			// so that south-east routes have ascending approaches.
			ele := int32(5000)
			if r >= 2 && c >= 2 {
				// +25 dm per step deeper into quadrant; max +75 dm
				ele += int32((r-1)+(c-1)) * 25
			}
			nodeEle[idx] = ele
		}
	}

	// ---------------------------------------------------------------------------
	// Directed edge list.
	// ---------------------------------------------------------------------------
	type dirEdge struct {
		from, to  int
		distCm    int
		forestPct uint8 // 0–100, encoded into EdgeTags2 bits 0-6
	}

	var edges []dirEdge

	addBidir := func(r1, c1, r2, c2, distCm int) {
		from := nodeIdx(r1, c1)
		to := nodeIdx(r2, c2)

		// ForestPct=70 for edges where both endpoints are in the bottom-right
		// quadrant (r≥2, c≥2).  This lets coverage descriptor tests trigger
		// high_forest on routes that traverse those edges.
		var fp uint8
		if r1 >= 2 && c1 >= 2 && r2 >= 2 && c2 >= 2 {
			fp = 70
		}

		edges = append(edges,
			dirEdge{from: from, to: to, distCm: distCm, forestPct: fp},
			dirEdge{from: to, to: from, distCm: distCm, forestPct: fp},
		)
	}

	// Horizontal edges (same row, adjacent cols).
	for r := 0; r < gridRows; r++ {
		for c := 0; c < gridCols-1; c++ {
			addBidir(r, c, r, c+1, distHorizCm)
		}
	}

	// Vertical edges (same col, adjacent rows).
	for c := 0; c < gridCols; c++ {
		for r := 0; r < gridRows-1; r++ {
			addBidir(r, c, r+1, c, distVertCm)
		}
	}

	// Diagonal edges (every other cell) — create disjoint round-trip paths
	// so that MergeLegs can find non-trivial loops.
	for r := 0; r < gridRows-1; r++ {
		for c := 0; c < gridCols-1; c++ {
			if (r+c)%2 == 0 {
				addBidir(r, c, r+1, c+1, distDiagCm)
			}
		}
	}

	// ---------------------------------------------------------------------------
	// Build CSR arrays from edge list.
	// ---------------------------------------------------------------------------
	edgeCount := uint32(len(edges))
	degree := make([]uint32, nNodes)
	for _, e := range edges {
		degree[e.from]++
	}

	offsets := make([]uint32, nNodes+1)
	for i := 0; i < nNodes; i++ {
		offsets[i+1] = offsets[i] + degree[i]
	}

	targets := make([]uint32, edgeCount)
	edgeDist := make([]uint32, edgeCount)
	edgeTags1 := make([]uint64, edgeCount)
	edgeTags2 := make([]uint64, edgeCount)
	edgeOSMIdx := make([]uint32, edgeCount)
	edgeGeomOff := make([]uint32, edgeCount+1) // all-zero = no intermediate geom
	edgeElevOff := make([]uint32, edgeCount+1)

	cursor := make([]uint32, nNodes)
	copy(cursor, offsets[:nNodes])
	for _, e := range edges {
		pos := cursor[e.from]
		cursor[e.from]++
		targets[pos] = uint32(e.to)
		edgeDist[pos] = uint32(e.distCm)
		edgeTags1[pos] = 16 // highway=path enum
		// Encode ForestPct into EdgeTags2 bits 0-6.
		edgeTags2[pos] = uint64(e.forestPct) & 0x7F

		if withOutdoorTags {
			// Inject outdoor tags per spec §8.2 rev5 — encoded values.
			//
			// Tag placement is calibrated against the three routes the V2 pipeline
			// produces from this fixture (baseline = no outdoor tags):
			//   Route 0 (shortest ≈6km):  col-1 vertical: (0,1)→(1,1)→(2,1)→(3,1)
			//   Route 1 (medium   ≈9km):  NE diagonal:    (0,1)→(0,2)→(1,3)→(2,4)
			//   Route 2 (longest  ≈11km): long loop:      (0,1)→(1,1)→(2,2)→(3,3)→…
			//
			// MTB and TV are cost-neutral for the hiking profile (no additive penalty),
			// so placing them does not alter route geometry. SAC uses sacPenalty[3] ≈ 3.1
			// (1.6^3 − 1, additive to CF); via-point selection is DistanceM-based (not
			// cost), so penalty does not change which via-points are chosen. The SAC
			// edge is placed where the penalised direct path is still cheaper than any
			// detour, so route geometry is preserved.
			// VF with hw=22 would be blocked (maxVF=0 default); we keep hw=path and only
			// set vfScale in Tags2 — also soft-fail.
			//
			// MTB  → (0,2)↔(1,3): Route 1 exclusive diagonal (r+c=2, exists).
			//                       TaggedPct ≈ 2×1646m/9224m ≈ 35% > 20% gate.
			//                       MaxMtbScale Routes 0,2 = 0 → mtb_technical fires.
			// TV   → (1,3)↔(2,4): Route 1 exclusive diagonal (r+c=4, exists).
			//                       IntermediatePct ≈ 35%; intTrigger fires (Δ > 10%).
			// SAC  → (0,1)↔(1,1): Start-vertical edge (start=(0,1)). Route 0 (col-1
			//                       down-and-back) traverses this edge on both its
			//                       outbound and return legs. Route 1 goes LEFT to
			//                       col-0 first, Route 2 goes RIGHT to col-2/col-3 —
			//                       neither uses this edge, so SAC-penalty does not
			//                       deflect their paths. Penalised direct cost stays
			//                       cheaper than any detour via (0,0); via-point
			//                       selection is DistanceM-based (unaffected by cost).
			//                       Geometrically traversed by Route 0, but integration
			//                       test cannot hard-assert until V2 pipeline emits
			//                       full-route segments (currently missing final-edge
			//                       per return leg — see variants_test SAC comment).
			// VF   → (3,3)↔(4,4): Route 2 diagonal (r+c=6, exists); hw stays path=16.
			//                       Appears in seg if Route 2 traverses (3,3)→(4,4).
			//                       Soft-fail.
			switch {
			case (e.from == nodeIdx(0, 2) && e.to == nodeIdx(1, 3)) ||
				(e.from == nodeIdx(1, 3) && e.to == nodeIdx(0, 2)):
				// mtb_scale encoded=3 (OSM raw "2" → Display S2), mtb:scale:uphill same
				edgeTags1[pos] = 16 | (uint64(3) << 17)
				edgeTags2[pos] = (uint64(e.forestPct) & 0x7F) | (uint64(3) << 30)
			case (e.from == nodeIdx(1, 3) && e.to == nodeIdx(2, 4)) ||
				(e.from == nodeIdx(2, 4) && e.to == nodeIdx(1, 3)):
				// trail_visibility encoded=3 (intermediate) — cost-neutral for hiking
				edgeTags2[pos] = (uint64(e.forestPct) & 0x7F) | (uint64(3) << 36)
			case (e.from == nodeIdx(0, 1) && e.to == nodeIdx(1, 1)) ||
				(e.from == nodeIdx(1, 1) && e.to == nodeIdx(0, 1)):
				// sac_scale encoded=3 (demanding_mountain_hiking = Display T3) on start-vertical.
				// Routes 0+2 traverse (0,1)↔(1,1) on both legs. Route 1 uses (0,1)→(0,2)
				// instead (NE diagonal path) — zero impact on Route 1 geometry.
				// Penalised direct cost (2613m units) < detour via (0,0) (5906m units).
				edgeTags1[pos] = 16 | (uint64(3) << 14)
			case (e.from == nodeIdx(3, 3) && e.to == nodeIdx(4, 4)) ||
				(e.from == nodeIdx(4, 4) && e.to == nodeIdx(3, 3)):
				// via_ferrata_scale encoded=2 (K2) — hw stays path (16) to avoid
				// hiking-profile block (maxVF=0). Soft-fail in test.
				edgeTags2[pos] = (uint64(e.forestPct) & 0x7F) | (uint64(2) << 33)
			}
		}
	}

	// One OSM-ID entry per node (fake, unique to avoid ID collisions).
	osmIDPool := make([]int64, nNodes)
	for i := range osmIDPool {
		osmIDPool[i] = int64(99900 + i)
	}

	g := &graph.Graph{
		NodeCount:       uint32(nNodes),
		EdgeCount:       edgeCount,
		Version:         graph.FormatVersion,
		Offsets:         offsets,
		Targets:         targets,
		EdgeDist:        edgeDist,
		EdgeTags1:       edgeTags1,
		EdgeTags2:       edgeTags2,
		EdgeOSMIdx:      edgeOSMIdx,
		OSMIDPool:       osmIDPool,
		NodeLat:         nodeLat,
		NodeLon:         nodeLon,
		NodeEle:         nodeEle,
		NodeTags:        make([]uint8, nNodes),
		IsBorder:        make([]bool, nNodes),
		EdgeGeomOff:     edgeGeomOff,
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     edgeElevOff,
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
	}

	// All nodes lie in the 48–49°N / 12–13°E tile.
	tileID := graph.TileIDFromCoord(48.0, 12.0)
	g.TileID = tileID
	g.BBox = graph.TileBBox(tileID)
	g.Grid = graph.BuildSpatialIndex(g, 0.01)

	// Serialize to the tile file name that TiledGraphReader expects.
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
		t.Fatalf("newTestRoutingHandler: InitTiledReader: expected tiles in %s", tileDir)
	}

	router := mgr.UnifiedRouter()
	if router == nil {
		t.Fatal("newTestRoutingHandler: UnifiedRouter is nil after InitTiledReader")
	}

	// --- Settings ---
	store := settings.NewStoreForTest(map[string]string{
		"limits.max_upload_bytes": fmt.Sprintf("%d", 256*1024*1024),
	})

	// --- Coverage reader (optional stub) ---
	var covReader *coverage.DiskCoverageReader
	if withCoverage {
		covDir := t.TempDir()
		covReader, err = coverage.NewDiskCoverageReader(covDir, 16)
		if err != nil {
			t.Fatalf("newTestRoutingHandler: NewDiskCoverageReader: %v", err)
		}
	}

	h := &Handler{
		GraphManager:   mgr,
		CoverageReader: covReader,
		Settings:       store,
	}

	cleanup := func() {}
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
	if setup.Handler.CoverageReader != nil {
		t.Error("expected CoverageReader=nil for base variant")
	}
}
