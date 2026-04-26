package enrichment

import (
	"context"
	"fmt"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/paulmach/orb"
)

// SegmentEnrichmentResponse is the single source of truth for the HTTP response
// shape. The handler in services/api/internal/handler/segments.go imports this
// type and JSON-encodes it directly — do NOT re-declare it in the handler
// package (Batch-5-Fix: ensure no duplicate struct declarations).
type SegmentEnrichmentResponse struct {
	Surface     *SurfaceSuggestion `json:"surface"`
	SacScale    *ScaleSuggestion   `json:"sac_scale"`
	MtbScale    *ScaleSuggestion   `json:"mtb_scale"`
	CoveragePct float64            `json:"coverage_pct"`
	Reason      string             `json:"reason,omitempty"`
}

// SurfaceSuggestion holds the aggregated surface label and distribution.
type SurfaceSuggestion struct {
	Dominant     string             `json:"dominant"`
	Distribution map[string]float64 `json:"distribution"`
	Mixed        bool               `json:"mixed"`
}

// ScaleSuggestion holds the aggregated scale label, max scale, and distribution.
type ScaleSuggestion struct {
	Dominant     string             `json:"dominant"`
	Max          string             `json:"max"`
	Distribution map[string]float64 `json:"distribution"`
}

// edgeSample captures what the aggregator needs from a matched edge.
type edgeSample struct {
	surfaceID uint8
	sacID     uint8
	mtbID     uint8
	lengthM   float64
}

const coverageThreshold = 0.30

// SuggestSegmentEnrichment matches a segment LineString against OSM ways and
// aggregates Surface/SAC/MTB tags from the matched edges.
//
// Strategy: Viterbi on the tiled graph (fast path, ~10ms, 1s timeout) with
// PostGIS nearest-way fallback (slow path, ~100ms).
//
// This function is segType-agnostic — the wintersport-short-circuit is applied
// upstream in the HTTP handler.
func SuggestSegmentEnrichment(
	ctx context.Context,
	db *database.DB,
	coords []orb.Point,
	graphReader routing.GraphReader,
) (*SegmentEnrichmentResponse, error) {
	if len(coords) < 2 {
		return nil, fmt.Errorf("need at least 2 coordinates, got %d", len(coords))
	}

	// Fast path: graph. Skip entirely if no graph reader available
	// (e.g. tests, graph not initialized) — fall straight through to PostGIS.
	var edges []edgeSample
	var coverage float64
	var graphAvailable bool
	var err error
	if graphReader != nil {
		graphCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		edges, graphAvailable, coverage, err = matchEdgesViaGraph(graphCtx, graphReader, coords)
		if err == nil && coverage >= coverageThreshold {
			return aggregateEdgesToSegmentEnrichment(edges, coverage), nil
		}
	}

	// Fallback: PostGIS nearest-way.
	// graphAvailable (set by matchEdgesViaGraph above) tracks whether at least
	// one tile was successfully loaded during matching — used below to distinguish
	// "OSM not installed / no tiles loaded" from "route is genuinely off-road".
	// No second iteration over coords is needed: matchEdgesViaGraph already
	// made GraphAt calls per-point and reported whether any tile responded.

	edges, coverage, err = matchEdgesViaPostGIS(ctx, db, coords)
	if err != nil {
		return nil, fmt.Errorf("both graph and postgis match failed: %w", err)
	}
	resp := aggregateEdgesToSegmentEnrichment(edges, coverage)
	// Override low_coverage with no_graph when both PostGIS AND the graph
	// returned zero matched edges. This is the "OSM not installed" signal:
	// a pure off-road route would have PostGIS ways nearby even if Viterbi
	// couldn't match them. Only apply when the graph had no tiles at all
	// (graphAvailable == false); if tiles exist but coverage is low the
	// low_coverage reason is correct.
	if len(edges) == 0 && !graphAvailable {
		resp.Reason = "no_graph"
	}
	return resp, nil
}

// matchEdgesViaGraph runs Viterbi against graphReader and extracts edge-level
// sample data (surface, sac, mtb, length). Mirrors track-analysis graph
// control flow: per-point FindNearEdges → Viterbi → edge-tag extraction.
// Returns (edges, graphAvailable, coverage_pct, err).
//
// graphAvailable is true if at least one tile was successfully loaded during
// matching, regardless of how many edges were ultimately matched. The caller
// uses this to distinguish "OSM graph not installed" (graphAvailable=false)
// from "route genuinely off-road or coverage too low" (graphAvailable=true).
// This avoids a second iteration over coords just to query GraphAt again.
func matchEdgesViaGraph(
	ctx context.Context,
	graphReader routing.GraphReader,
	coords []orb.Point,
) ([]edgeSample, bool, float64, error) {
	if len(coords) < 2 {
		return nil, false, 0, fmt.Errorf("need at least 2 coordinates")
	}

	// Convert orb.Point{lng, lat} → {lat, lng} structs used by Viterbi/FindNearEdges.
	pts := make([]struct{ lat, lng float64 }, len(coords))
	for i, c := range coords {
		pts[i] = struct{ lat, lng float64 }{lat: c[1], lng: c[0]}
	}

	// Gather edge candidates per point.  tileGraphs[i] is the tile graph that
	// produced candidates[i] — needed after Viterbi to re-read Tags1/EdgeDist.
	candidates := make([][]graph.EdgeCandidate, len(pts))
	tileGraphs := make([]*graph.Graph, len(pts))
	var graphAvailable bool
	for i, pt := range pts {
		// Context check: abort early if the caller timed out.
		if err := ctx.Err(); err != nil {
			return nil, graphAvailable, 0, err
		}
		g := graphReader.GraphAt(pt.lat, pt.lng)
		if g == nil {
			continue // offroad / no tile loaded for this location
		}
		graphAvailable = true // at least one tile responded
		cands := graph.FindNearEdges(g, pt.lat, pt.lng)
		if len(cands) > 0 {
			candidates[i] = cands
			tileGraphs[i] = g
		}
	}

	matched := viterbiMatch(pts, candidates)

	// Extract per-edge samples from the Viterbi path.
	var samples []edgeSample
	var matchedCount int
	for i, ec := range matched {
		if len(candidates[i]) == 0 {
			// Offroad point — no tile data.
			continue
		}
		matchedCount++
		g := tileGraphs[i]
		if g == nil || int(ec.EdgeIdx) >= len(g.EdgeTags1) {
			continue
		}
		tags1 := g.EdgeTags1[ec.EdgeIdx]
		distCm := g.EdgeDist[ec.EdgeIdx]
		samples = append(samples, edgeSample{
			surfaceID: graph.DecodeSurface(tags1),
			sacID:     graph.DecodeSacScale(tags1),
			mtbID:     graph.DecodeMtbScale(tags1),
			lengthM:   float64(distCm) / 100.0,
		})
	}

	coverage := float64(matchedCount) / float64(len(pts))
	return samples, graphAvailable, coverage, nil
}

// matchEdgesViaPostGIS queries osm_ways for the nearest way per coordinate and
// extracts surface/sac/mtb from the text columns using EncodeOSMTags + Decode*.
//
// This is a slow fallback (~100 ms for a typical segment): used only when no
// graph tile covers the region.  Each coordinate triggers a lateral nearest-
// neighbour query bounded by a 20 m bbox expand to keep the scan bounded.
//
// Known limitation: the osm_ways geography index was dropped in Migration
// 000194 (6.6 GB unused), so the query uses the standard geometry index.  For
// dense regions the geometry-KNN is still fast (< 2 ms per point).
//
// If no osm_ways rows exist at all (OSM data not yet imported), every query
// returns no rows → empty sample slice → coverage 0 → caller returns an error.
func matchEdgesViaPostGIS(
	ctx context.Context,
	db *database.DB,
	coords []orb.Point,
) ([]edgeSample, float64, error) {
	if len(coords) < 2 {
		return nil, 0, fmt.Errorf("need at least 2 coordinates")
	}

	// bboxExpand ~22 m at mid-latitudes — matches surfaceBBoxExpand.
	// Distance filtering is done implicitly via bbox expand + KNN ordering;
	// no explicit maxDistM guard is needed here.
	const bboxExpand = 0.0002

	var samples []edgeSample
	var matchedCount int

	for _, coord := range coords {
		// coord = orb.Point{lng, lat}
		lng, lat := coord[0], coord[1]

		var surface, sacScale, mtbScale string
		var lengthM float64
		err := db.Pool.QueryRow(ctx, `
			SELECT
				COALESCE(surface, ''),
				COALESCE(sac_scale, ''),
				COALESCE(mtb_scale, ''),
				COALESCE(length_m, 0)
			FROM osm_ways
			WHERE highway IS NOT NULL
			  AND highway != 'construction'
			  AND geom && ST_Expand(ST_SetSRID(ST_MakePoint($1, $2), 4326), $3)
			ORDER BY geom <-> ST_SetSRID(ST_MakePoint($1, $2), 4326)
			LIMIT 1
		`, lng, lat, bboxExpand).Scan(&surface, &sacScale, &mtbScale, &lengthM)
		if err != nil {
			// pgx returns pgx.ErrNoRows when no way is within the bbox — treat as offroad.
			continue
		}

		matchedCount++

		// Convert string tags → numeric IDs via the same encoding path as the tile builder.
		tags1 := graph.EncodeOSMTags(&graph.RawTags{
			Surface:  surface,
			SacScale: sacScale,
			MtbScale: mtbScale,
		})
		samples = append(samples, edgeSample{
			surfaceID: graph.DecodeSurface(tags1),
			sacID:     graph.DecodeSacScale(tags1),
			mtbID:     graph.DecodeMtbScale(tags1),
			lengthM:   lengthM,
		})
	}

	coverage := float64(matchedCount) / float64(len(coords))
	return samples, coverage, nil
}

// aggregateEdgesToSegmentEnrichment is the deterministic, unit-testable core.
// Given matched edges and a coverage ratio, produces the response struct.
func aggregateEdgesToSegmentEnrichment(edges []edgeSample, coverage float64) *SegmentEnrichmentResponse {
	resp := &SegmentEnrichmentResponse{CoveragePct: coverage}

	if coverage < coverageThreshold {
		resp.Reason = "low_coverage"
		return resp
	}

	// Aggregate surface IDs → group-label totals by length.
	surfaceTotals := map[string]float64{}
	sacTotals := map[string]float64{}
	mtbTotals := map[string]float64{}
	var surfaceLen, sacLen, mtbLen float64
	for _, e := range edges {
		if group := surfaceIDToGroupForSegment(e.surfaceID); group != "" {
			surfaceTotals[group] += e.lengthM
			surfaceLen += e.lengthM
		}
		if e.sacID >= 1 && e.sacID <= 6 {
			key := fmt.Sprintf("T%d", e.sacID)
			sacTotals[key] += e.lengthM
			sacLen += e.lengthM
		}
		if e.mtbID >= 1 && e.mtbID <= 6 {
			key := fmt.Sprintf("S%d", e.mtbID-1)
			mtbTotals[key] += e.lengthM
			mtbLen += e.lengthM
		}
	}

	if len(surfaceTotals) > 0 {
		resp.Surface = buildSurfaceSuggestion(surfaceTotals, surfaceLen)
	}
	if len(sacTotals) > 0 {
		resp.SacScale = buildScaleSuggestion(sacTotals, sacLen, "T")
	}
	if len(mtbTotals) > 0 {
		resp.MtbScale = buildScaleSuggestion(mtbTotals, mtbLen, "S")
	}

	if resp.Surface == nil && resp.SacScale == nil && resp.MtbScale == nil {
		resp.Reason = "no_data"
	}
	return resp
}

// buildSurfaceSuggestion / buildScaleSuggestion — deterministic tie-handling
//
// Go map iteration is intentionally randomized. If we picked `dominant` by a
// single-pass "largest seen so far" check, two identical input maps could yield
// different `dominant` strings on different runs. Spec §4.7 says: at a true tie
// (no single majority), `dominant` MUST be empty rather than arbitrary. We
// collect all candidates sharing the top value, and return `dominant = ""` iff
// more than one candidate shares it.
func buildSurfaceSuggestion(totals map[string]float64, total float64) *SurfaceSuggestion {
	dist := map[string]float64{}
	var best []string
	var bestVal float64
	overTwenty := 0
	for k, v := range totals {
		pct := v / total
		dist[k] = pct
		if pct >= 0.20 {
			overTwenty++
		}
		if v > bestVal {
			bestVal = v
			best = []string{k}
		} else if v == bestVal {
			best = append(best, k)
		}
	}
	dominant := ""
	if len(best) == 1 {
		dominant = best[0]
	}
	// At a true tie (len(best) >= 2) we leave dominant empty per spec §4.7.
	return &SurfaceSuggestion{
		Dominant:     dominant,
		Distribution: dist,
		Mixed:        overTwenty >= 2,
	}
}

// buildScaleSuggestion takes totals keyed by scale-label (e.g. "T1".."T6" or
// "S0".."S5") and returns a ScaleSuggestion with distribution percentages +
// dominant (highest share, empty at tie) + max (highest scale-number present).
// `max` is well-defined at ties — it's the numerically highest label, not the
// highest share.
func buildScaleSuggestion(totals map[string]float64, total float64, prefix string) *ScaleSuggestion {
	dist := map[string]float64{}
	maxLabel := ""
	maxNum := -1
	// First pass: distribution + max (max is well-defined even at share-ties).
	for k, v := range totals {
		dist[k] = v / total
		num := 0
		fmt.Sscanf(k, prefix+"%d", &num)
		if num > maxNum {
			maxNum = num
			maxLabel = k
		}
	}
	// Second pass: determine best (dominant) candidates deterministically.
	var best []string
	var bestVal float64
	for k, v := range totals {
		if v > bestVal {
			bestVal = v
			best = []string{k}
		} else if v == bestVal {
			best = append(best, k)
		}
	}
	dominant := ""
	if len(best) == 1 {
		dominant = best[0]
	}
	return &ScaleSuggestion{Dominant: dominant, Max: maxLabel, Distribution: dist}
}
