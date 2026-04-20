package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/skadia-app/skadia/services/api/internal/settings"
)

// buildVfCoverageFixture creates a Handler with a GraphManager pointing at a
// temp tile dir containing one synthetic tile.
//
// The tile contains:
//   - nVfScaled   via_ferrata edges (hw=22, vfScale=3 → K3)
//   - nVfUnscaled via_ferrata edges (hw=22, vfScale=0 → unscaled)
//   - nOther      non-via_ferrata edges (hw=16 = path)
//
// All edges run from node 0 → node 1 (minimal 2-node graph).
func buildVfCoverageFixture(t *testing.T, nVfScaled, nVfUnscaled, nOther int) *Handler {
	t.Helper()

	tileDir := t.TempDir()

	nNodes := uint32(2)
	edgeCount := uint32(nVfScaled + nVfUnscaled + nOther)

	// CSR: node 0 owns all edges; node 1 has none.
	offsets := []uint32{0, edgeCount, edgeCount}
	targets := make([]uint32, edgeCount)
	edgeDist := make([]uint32, edgeCount)
	edgeTags1 := make([]uint64, edgeCount)
	edgeTags2 := make([]uint64, edgeCount)
	edgeOSMIdx := make([]uint32, edgeCount)
	edgeGeomOff := make([]uint32, edgeCount+1)
	edgeElevOff := make([]uint32, edgeCount+1)

	for i := 0; i < int(edgeCount); i++ {
		targets[i] = 1
		edgeDist[i] = 100_000 // 1 km in cm

		switch {
		case i < nVfScaled:
			// via_ferrata (hw=22), vfScale=3 (K3) at bits 33-35 of Tags2.
			edgeTags1[i] = uint64(22) // highway bits 0-4
			edgeTags2[i] = uint64(3) << 33
		case i < nVfScaled+nVfUnscaled:
			// via_ferrata (hw=22), vfScale=0 (unscaled).
			edgeTags1[i] = uint64(22)
			edgeTags2[i] = 0
		default:
			// Normal path (hw=16).
			edgeTags1[i] = uint64(16)
			edgeTags2[i] = 0
		}
	}

	nodeLat := []int32{int32(48.5 * 1e7), int32(48.501 * 1e7)}
	nodeLon := []int32{int32(14.0 * 1e7), int32(14.001 * 1e7)}
	nodeEle := []int32{5000, 5010}

	g := &graph.Graph{
		NodeCount:       nNodes,
		EdgeCount:       edgeCount,
		Version:         graph.FormatVersion,
		Offsets:         offsets,
		Targets:         targets,
		EdgeDist:        edgeDist,
		EdgeTags1:       edgeTags1,
		EdgeTags2:       edgeTags2,
		EdgeOSMIdx:      edgeOSMIdx,
		OSMIDPool:       []int64{100001, 100002},
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

	tileID := graph.TileIDFromCoord(48.0, 14.0)
	g.TileID = tileID
	g.BBox = graph.TileBBox(tileID)
	g.Grid = graph.BuildSpatialIndex(g, 0.01)

	// Serialize to the filename that TiledGraphReader / TileIndex expect.
	latFloor, lonFloor := graph.TileLatLon(tileID)
	tileFileName := "tile_" + strconv.Itoa(latFloor) + "_" + strconv.Itoa(lonFloor) + ".bin"
	tilePath := filepath.Join(tileDir, tileFileName)

	if err := graph.Serialize(g, tilePath); err != nil {
		t.Fatalf("buildVfCoverageFixture: serialize tile: %v", err)
	}

	mgr := &routing.GraphManager{}
	if _, err := mgr.InitTiledReader(tileDir, 64_000_000); err != nil {
		t.Fatalf("buildVfCoverageFixture: InitTiledReader: %v", err)
	}

	store := settings.NewStoreForTest(nil)
	return &Handler{
		GraphManager: mgr,
		Settings:     store,
	}
}

// doVfCoverageRequest fires GET /admin/routing/vf-encoding-coverage with an
// optional query string and deserialises the response.
func doVfCoverageRequest(t *testing.T, h *Handler, query string) VfCoverageResponse {
	t.Helper()
	url := "/admin/routing/vf-encoding-coverage"
	if query != "" {
		url += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	h.GetVfEncodingCoverage(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp VfCoverageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, rr.Body.String())
	}
	return resp
}

// TestGetVfEncodingCoverage_BasicCounts verifies edge-count fields from a
// mixed fixture (scaled + unscaled + non-VF edges).
func TestGetVfEncodingCoverage_BasicCounts(t *testing.T) {
	h := buildVfCoverageFixture(t, 5, 3, 10) // 5 scaled, 3 unscaled, 10 other
	resp := doVfCoverageRequest(t, h, "")

	if resp.TotalViaFerrataEdges != 8 {
		t.Errorf("TotalViaFerrataEdges: want 8, got %d", resp.TotalViaFerrataEdges)
	}
	if resp.ScaledEdges != 5 {
		t.Errorf("ScaledEdges: want 5, got %d", resp.ScaledEdges)
	}
	if resp.UnscaledEdges != 3 {
		t.Errorf("UnscaledEdges: want 3, got %d", resp.UnscaledEdges)
	}
	if resp.ScaledEdges+resp.UnscaledEdges != resp.TotalViaFerrataEdges {
		t.Errorf("scaled+unscaled (%d) != total (%d)", resp.ScaledEdges+resp.UnscaledEdges, resp.TotalViaFerrataEdges)
	}
}

// TestGetVfEncodingCoverage_AllScaled verifies coverage_pct > 0.95 when all
// via_ferrata edges carry a valid K-scale.
func TestGetVfEncodingCoverage_AllScaled(t *testing.T) {
	h := buildVfCoverageFixture(t, 10, 0, 5) // 10 scaled, 0 unscaled
	resp := doVfCoverageRequest(t, h, "")

	if resp.CoveragePct <= 0.95 {
		t.Errorf("CoveragePct: want >0.95, got %f", resp.CoveragePct)
	}
	if resp.UnscaledEdges != 0 {
		t.Errorf("UnscaledEdges: want 0, got %d", resp.UnscaledEdges)
	}
	if len(resp.TopUnscaledTiles) != 0 {
		t.Errorf("TopUnscaledTiles: want empty, got %v", resp.TopUnscaledTiles)
	}
}

// TestGetVfEncodingCoverage_NoViaFerrata verifies that a tile without any
// via_ferrata edges reports total=0, coverage_pct=0.0.
func TestGetVfEncodingCoverage_NoViaFerrata(t *testing.T) {
	h := buildVfCoverageFixture(t, 0, 0, 8) // 8 normal path edges only
	resp := doVfCoverageRequest(t, h, "")

	if resp.TotalViaFerrataEdges != 0 {
		t.Errorf("TotalViaFerrataEdges: want 0, got %d", resp.TotalViaFerrataEdges)
	}
	if resp.CoveragePct != 0.0 {
		t.Errorf("CoveragePct: want 0.0, got %f", resp.CoveragePct)
	}
}

// TestGetVfEncodingCoverage_LimitClamp verifies that limit=99999 is clamped to
// 1000 and the request still succeeds (fixture has only 1 tile, so no truncation).
func TestGetVfEncodingCoverage_LimitClamp(t *testing.T) {
	h := buildVfCoverageFixture(t, 2, 1, 2)

	// Oversized limit — clamped to 1000; fixture has 1 tile → no truncation.
	resp := doVfCoverageRequest(t, h, "limit=99999")
	if resp.TilesScanned == 0 {
		t.Error("expected at least 1 tile scanned with limit=99999")
	}
	if resp.Truncated {
		t.Error("Truncated should be false for single-tile fixture with large limit")
	}

	// Default limit (no query parameter) — should also scan the single tile.
	resp2 := doVfCoverageRequest(t, h, "")
	if resp2.TilesScanned == 0 {
		t.Error("expected at least 1 tile scanned with default limit")
	}
}

// TestGetVfEncodingCoverage_NilGraphManager verifies HTTP 503 when routing is
// not configured.
func TestGetVfEncodingCoverage_NilGraphManager(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/admin/routing/vf-encoding-coverage", nil)
	rr := httptest.NewRecorder()
	h.GetVfEncodingCoverage(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// TestGetVfEncodingCoverage_TilesScanned verifies TilesScanned is at least 1
// for a fixture with one tile.
func TestGetVfEncodingCoverage_TilesScanned(t *testing.T) {
	h := buildVfCoverageFixture(t, 3, 2, 4)
	resp := doVfCoverageRequest(t, h, "")
	if resp.TilesScanned < 1 {
		t.Errorf("TilesScanned: want ≥1, got %d", resp.TilesScanned)
	}
}
