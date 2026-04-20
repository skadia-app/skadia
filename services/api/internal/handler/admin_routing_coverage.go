package handler

import (
	"net/http"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// VfCoverageResponse is the JSON response for GET /admin/routing/vf-encoding-coverage.
//
// It reports how many via_ferrata edges (highway==22) in the current tile set
// carry a non-zero vfScale (K1–K6), vs. how many are unscaled (vfScale==0,
// meaning the tile was built before Task 7a landed or before a tile-rebuild ran).
//
// A pre-rebuild instance will report coverage_pct ≈ 0.001 (nearly all zeroes).
// A post-rebuild instance should report coverage_pct ≥ 0.95 once OSM data
// carries via_ferrata_scale tags (see docs/_internal/open-questions.md).
type VfCoverageResponse struct {
	TotalViaFerrataEdges int         `json:"total_via_ferrata_edges"`
	ScaledEdges          int         `json:"scaled_edges"`
	UnscaledEdges        int         `json:"unscaled_edges"`
	CoveragePct          float32     `json:"coverage_pct"` // scaled / total; 0.0 when total==0
	TilesScanned         int         `json:"tiles_scanned"`
	Truncated            bool        `json:"truncated"` // true when ?limit < total tile count
	TopUnscaledTiles     []TileCount `json:"top_unscaled_tiles,omitempty"`
}

// TileCount pairs a tile ID (human-readable "LAT_LON") with a per-tile edge count.
type TileCount struct {
	TileID    string `json:"tile_id"`    // e.g. "46_14" (lat_lon floor)
	UnscaledN int    `json:"unscaled_n"` // number of unscaled VF edges in this tile
}

const (
	hwViaFerrata = uint8(22) // routing/graph/tags.go: "via_ferrata": 22
	vfCoverageTopN = 20      // TopUnscaledTiles capped at 20 entries
)

// GetVfEncodingCoverage scans all routing tiles and classifies via_ferrata edges.
//
// GET /api/v1/admin/routing/vf-encoding-coverage
//
// Query parameters:
//   - limit=N   Scan at most N tiles (default 100, max 1000).
//               When more tiles exist, truncated=true is set in the response.
//
// The handler honours ctx.Done(): it stops scanning mid-way and returns a
// partial result with truncated=true if the request context is cancelled.
func (h *Handler) GetVfEncodingCoverage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.GraphManager == nil {
		httputil.WriteJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "routing not configured",
		})
		return
	}

	// --- limit query parameter ---
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 1000 {
				n = 1000
			}
			limit = n
		}
	}

	// --- Tile enumeration via TileManager ---
	tm := h.GraphManager.TileManager()
	if tm == nil {
		// Routing is configured but no tiles have been loaded yet.
		httputil.WriteJSON(w, http.StatusOK, VfCoverageResponse{})
		return
	}

	allIDs := tm.AllTileIDs()
	total := len(allIDs)

	truncated := total > limit
	scanIDs := allIDs
	if truncated {
		scanIDs = allIDs[:limit]
	}

	// --- Per-tile edge scan ---
	type tileResult struct {
		id        uint32
		vfTotal   int
		vfScaled  int
		vfUnscaled int
	}

	// tileDir derives the tile directory from RoutingGraphPath (same logic as
	// RebuildRoutingTiles / RebuildLandmarks).
	tileDir := ""
	if h.Config != nil {
		tileDir = filepath.Join(filepath.Dir(h.Config.RoutingGraphPath), "tiles")
	}

	// Scan each tile sequentially; respect ctx cancellation.
	results := make([]tileResult, 0, len(scanIDs))
	cancelledEarly := false

	for _, tileID := range scanIDs {
		select {
		case <-ctx.Done():
			cancelledEarly = true
			truncated = true
		default:
		}
		if cancelledEarly {
			break
		}

		g, err := loadTileForCoverage(tm, tileDir, tileID)
		if err != nil {
			// Non-fatal: skip missing or corrupt tiles.
			continue
		}

		res := tileResult{id: tileID}
		for edgeIdx := uint32(0); edgeIdx < g.EdgeCount; edgeIdx++ {
			tags1 := g.EdgeTags1[edgeIdx]
			hw := graph.DecodeHighway(tags1)
			if hw != hwViaFerrata {
				continue
			}
			res.vfTotal++
			tags2 := g.EdgeTags2[edgeIdx]
			scale := graph.DecodeViaFerrataScale(tags2)
			if scale > 0 {
				res.vfScaled++
			} else {
				res.vfUnscaled++
			}
		}
		results = append(results, res)
	}

	// --- Aggregate ---
	resp := VfCoverageResponse{
		TilesScanned: len(results),
		Truncated:    truncated,
	}
	tileCounts := make([]TileCount, 0)

	for _, r := range results {
		resp.TotalViaFerrataEdges += r.vfTotal
		resp.ScaledEdges += r.vfScaled
		resp.UnscaledEdges += r.vfUnscaled
		if r.vfUnscaled > 0 {
			lat, lon := graph.TileLatLon(r.id)
			tileCounts = append(tileCounts, TileCount{
				TileID:    strconv.Itoa(lat) + "_" + strconv.Itoa(lon),
				UnscaledN: r.vfUnscaled,
			})
		}
	}

	if resp.TotalViaFerrataEdges > 0 {
		resp.CoveragePct = float32(resp.ScaledEdges) / float32(resp.TotalViaFerrataEdges)
	}

	// Top-N unscaled tiles (descending by unscaled count).
	if len(tileCounts) > 0 {
		sort.Slice(tileCounts, func(i, j int) bool {
			return tileCounts[i].UnscaledN > tileCounts[j].UnscaledN
		})
		if len(tileCounts) > vfCoverageTopN {
			tileCounts = tileCounts[:vfCoverageTopN]
		}
		resp.TopUnscaledTiles = tileCounts
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// loadTileForCoverage loads a Graph from TileManager.
// tileDir is used as a fallback path hint; not strictly required since TileManager
// already holds the tile index internally — GetTile is the primary API.
func loadTileForCoverage(tm *routing.TileManager, _ string, tileID uint32) (*graph.Graph, error) {
	return tm.GetTile(tileID)
}
