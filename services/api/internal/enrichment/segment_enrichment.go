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
	var err error
	if graphReader != nil {
		graphCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		edges, coverage, err = matchEdgesViaGraph(graphCtx, graphReader, coords)
		if err == nil && coverage >= coverageThreshold {
			return aggregateEdgesToSegmentEnrichment(edges, coverage), nil
		}
	}

	// Fallback: PostGIS nearest-way.
	edges, coverage, err = matchEdgesViaPostGIS(ctx, db, coords)
	if err != nil {
		return nil, fmt.Errorf("both graph and postgis match failed: %w", err)
	}
	resp := aggregateEdgesToSegmentEnrichment(edges, coverage)
	if resp.Reason == "" && len(edges) == 0 {
		resp.Reason = "no_graph"
	}
	return resp, nil
}

// matchEdgesViaGraph runs Viterbi against graphReader and extracts edge-level
// sample data (surface, sac, mtb, length). Follows the enrichSurfacesGraph pattern.
// Returns (edges, coverage_pct, err).
func matchEdgesViaGraph(
	ctx context.Context,
	graphReader routing.GraphReader,
	coords []orb.Point,
) ([]edgeSample, float64, error) {
	// TODO(impl): replicate the Viterbi-call pattern from
	//   enrichment/surfaces_graph.go:enrichSurfacesGraph.
	// For each matched edge, extract:
	//   tags1 := edge.Tags1
	//   sample := edgeSample{
	//     surfaceID: graph.DecodeSurface(tags1),
	//     sacID:     graph.DecodeSacScale(tags1),
	//     mtbID:     graph.DecodeMtbScale(tags1),
	//     lengthM:   edge.LengthM,
	//   }
	// coverage = matchedPoints / totalPoints (from Viterbi match-report).
	_ = graph.DecodeSurface // silence unused until impl
	return nil, 0, fmt.Errorf("not implemented — follow enrichSurfacesGraph pattern")
}

// matchEdgesViaPostGIS queries PostGIS for the nearest OSM way per coordinate
// and extracts surface/sac/mtb tags from the matched rows.
func matchEdgesViaPostGIS(
	ctx context.Context,
	db *database.DB,
	coords []orb.Point,
) ([]edgeSample, float64, error) {
	// TODO(impl): reuse the PostGIS-nearest-way query from
	//   enrichment/surfaces.go fallback path (if any). If no existing helper,
	//   write a minimal per-point ST_ClosestPoint query against the OSM ways
	//   table that ClassicRouter/BadRouter uses.
	return nil, 0, fmt.Errorf("not implemented")
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
		if group := surfaceIDToGroup(e.surfaceID); group != "" {
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
