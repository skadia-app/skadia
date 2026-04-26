package enrichment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/jackc/pgx/v5"
)

type scalePercentages struct {
	Sac       map[string]float64
	Mtb       map[string]float64
	Vf        map[string]float64
	VfUnknown float64
}

func enrichTrackAnalysisGraph(
	ctx context.Context,
	db *database.DB,
	graphGetter func(lat, lng float64) *graph.Graph,
	activityID string,
	stream []EnrichmentStreamPoint,
	settings *model.EnrichmentSettings,
) error {
	pts := resampleStreamPoints(stream, surfaceResampleM)
	if len(pts) < 2 {
		return nil
	}

	candidates := make([][]graph.EdgeCandidate, len(pts))
	for i, pt := range pts {
		g := graphGetter(pt.lat, pt.lng)
		if g == nil {
			continue
		}
		candidates[i] = graph.FindNearEdges(g, pt.lat, pt.lng)
	}

	matched := viterbiMatch(pts, candidates)

	var edgeIdxs []uint32
	var tags1 []uint64
	var tags2 []uint64
	var matchedCoordIdxs []int
	for i, ec := range matched {
		if len(candidates[i]) == 0 {
			continue
		}
		matchedCoordIdxs = append(matchedCoordIdxs, i)
		edgeIdxs = append(edgeIdxs, ec.EdgeIdx)
		tags1 = append(tags1, ec.Tags1)
		tags2 = append(tags2, ec.Tags2)
	}

	lastCoordIdx := len(pts) - 1
	edgeCoordStart, coalescedTags1, coalescedTags2 := buildMatchedEdgeRanges(edgeIdxs, tags1, tags2, matchedCoordIdxs, lastCoordIdx)

	coords := make([]edgetags.LatLng, len(pts))
	for i, pt := range pts {
		coords[i] = edgetags.LatLng{Lat: pt.lat, Lng: pt.lng}
	}

	allRanges := edgetags.ComputeAllRanges(edgeCoordStart, coalescedTags1, coalescedTags2, lastCoordIdx)
	scalePct := computeScalePercentagesByDistance(coords, edgeCoordStart, coalescedTags1, coalescedTags2, lastCoordIdx)
	forestPct, protectedPct := edgetags.AggregateForestProtected(coords, edgeCoordStart, coalescedTags2, lastCoordIdx)

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("track_analysis_graph: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := deleteExistingTrackAnalysisTx(ctx, tx, activityID); err != nil {
		return err
	}

	if settings.EnableSurfaceDetection {
		points := classifyGraphMatches(matched, candidates)
		// withRanges=true: graph path has full per-point index alignment, so ranges are reliable.
		surfaceBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.SurfaceClass }, true)
		highwayBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.HighwayClass }, true)

		if err := writeSurfaceHighwayEnrichments(ctx, tx, activityID, surfaceBreakdowns, highwayBreakdowns); err != nil {
			return err
		}
		if err := writeForestProtectedTx(ctx, tx, activityID, forestPct*100.0, protectedPct*100.0, allRanges.Forest, allRanges.Protected); err != nil {
			return err
		}
	}

	if settings.EnableDifficultyScales {
		if err := writeDifficultyScalesTx(ctx, tx, activityID, scalePct, allRanges); err != nil {
			return err
		}
	}

	// Persist enrichment path metadata so that the track-summary handler can read
	// graph_snap_used directly without a heuristic (see activity_enrichment_meta).
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_enrichment_meta (activity_id, path, resample_spacing_m, computed_at)
		VALUES ($1, 'graph', $2, NOW())
		ON CONFLICT (activity_id) DO UPDATE
			SET path = EXCLUDED.path,
			    resample_spacing_m = EXCLUDED.resample_spacing_m,
			    computed_at = EXCLUDED.computed_at
	`, activityID, surfaceResampleM); err != nil {
		return fmt.Errorf("track_analysis_graph: upsert enrichment meta: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("track_analysis_graph: commit tx: %w", err)
	}
	return nil
}

func buildMatchedEdgeRanges(edgeIdxs []uint32, tags1, tags2 []uint64, matchedCoordIdxs []int, lastCoordIdx int) (edgeCoordStart []int, t1Out, t2Out []uint64) {
	n := len(edgeIdxs)
	if n == 0 || len(tags1) != n || len(tags2) != n || len(matchedCoordIdxs) != n || lastCoordIdx < 0 {
		return nil, nil, nil
	}

	appendRange := func(start int, t1, t2 uint64) {
		if len(edgeCoordStart) > 0 && edgeCoordStart[len(edgeCoordStart)-1] == start {
			t1Out[len(t1Out)-1] = t1
			t2Out[len(t2Out)-1] = t2
			return
		}
		edgeCoordStart = append(edgeCoordStart, start)
		t1Out = append(t1Out, t1)
		t2Out = append(t2Out, t2)
	}

	prevCoord := -1
	var prevEdge uint32
	for i, coordIdx := range matchedCoordIdxs {
		if coordIdx < 0 || coordIdx > lastCoordIdx {
			return nil, nil, nil
		}

		if i == 0 {
			if coordIdx > 0 {
				appendRange(0, 0, 0)
			}
			appendRange(coordIdx, tags1[i], tags2[i])
		} else if coordIdx > prevCoord+1 {
			appendRange(prevCoord+1, 0, 0)
			appendRange(coordIdx, tags1[i], tags2[i])
		} else if coordIdx == prevCoord+1 && edgeIdxs[i] != prevEdge {
			appendRange(coordIdx, tags1[i], tags2[i])
		}

		prevCoord = coordIdx
		prevEdge = edgeIdxs[i]
	}

	if prevCoord >= 0 && prevCoord < lastCoordIdx {
		appendRange(prevCoord+1, 0, 0)
	}

	return edgeCoordStart, t1Out, t2Out
}

func expandCoalescedStarts(edgeCoordStart []int, matchedCoordIdxs []int) []int {
	if len(edgeCoordStart) == 0 {
		return nil
	}
	out := make([]int, 0, len(edgeCoordStart))
	for _, start := range edgeCoordStart {
		if start < 0 || start >= len(matchedCoordIdxs) {
			return nil
		}
		out = append(out, matchedCoordIdxs[start])
	}
	return out
}

func computeScalePercentagesByDistance(coords []edgetags.LatLng, edgeCoordStart []int, tags1, tags2 []uint64, lastCoordIdx int) scalePercentages {
	out := scalePercentages{
		Sac: make(map[string]float64, len(edgetags.SacLevels)),
		Mtb: make(map[string]float64, len(edgetags.MtbLevels)),
		Vf:  make(map[string]float64, len(edgetags.VfLevels)),
	}
	n := len(tags1)
	if n == 0 || len(tags2) != n || len(edgeCoordStart) != n || len(coords) < 2 || lastCoordIdx <= 0 {
		return out
	}
	lastCoordIdx = min(lastCoordIdx, len(coords)-1)

	var totalDist float64
	sacDist := make(map[string]float64, len(edgetags.SacLevels))
	mtbDist := make(map[string]float64, len(edgetags.MtbLevels))
	vfDist := make(map[string]float64, len(edgetags.VfLevels))
	var vfUnknownDist float64

	for i, t1 := range tags1 {
		startIdx := edgeCoordStart[i]
		endIdx := lastCoordIdx
		if i+1 < n {
			endIdx = edgeCoordStart[i+1]
		}
		if endIdx > lastCoordIdx {
			endIdx = lastCoordIdx
		}
		if startIdx < 0 || startIdx >= endIdx || endIdx >= len(coords) {
			continue
		}

		var edgeDist float64
		for j := startIdx + 1; j <= endIdx; j++ {
			edgeDist += geo.DistanceHaversine(
				coords[j-1].Lat, coords[j-1].Lng,
				coords[j].Lat, coords[j].Lng,
			)
		}
		if edgeDist == 0 {
			continue
		}
		totalDist += edgeDist

		if sac := graph.SacScaleShort(graph.DecodeSacScale(t1)); sac != "" {
			sacDist[sac] += edgeDist
		}
		if mtb := graph.MtbScaleShort(graph.DecodeMtbScale(t1)); mtb != "" {
			mtbDist[mtb] += edgeDist
		}
		t2 := tags2[i]
		if vf := graph.ViaFerrataScaleShort(graph.DecodeViaFerrataScale(t2)); vf != "" {
			vfDist[vf] += edgeDist
		}
		if graph.DecodeViaFerrataScale(t2) == 0 && graph.DecodeHighway(t1) == 22 {
			vfUnknownDist += edgeDist
		}
	}

	if totalDist == 0 {
		return out
	}
	for k, v := range sacDist {
		out.Sac[k] = v / totalDist * 100.0
	}
	for k, v := range mtbDist {
		out.Mtb[k] = v / totalDist * 100.0
	}
	for k, v := range vfDist {
		out.Vf[k] = v / totalDist * 100.0
	}
	out.VfUnknown = vfUnknownDist / totalDist * 100.0
	return out
}

func classifyGraphMatches(matched []graph.EdgeCandidate, candidates [][]graph.EdgeCandidate) []matchedPoint {
	points := make([]matchedPoint, len(matched))
	for i, ec := range matched {
		p := matchedPoint{Idx: i}
		if len(candidates[i]) == 0 {
			p.SurfaceClass = "offroad"
			p.HighwayClass = "offroad"
		} else if ec.Surface == "" || ec.Surface == "unknown" {
			p.SurfaceClass = "unknown"
			p.HighwayClass = ec.Highway
			if p.HighwayClass == "" {
				p.HighwayClass = "unknown"
			}
		} else {
			p.SurfaceClass = ec.Surface
			p.HighwayClass = ec.Highway
			if p.HighwayClass == "" {
				p.HighwayClass = "unknown"
			}
		}
		points[i] = p
	}
	return points
}

func writeSurfaceHighwayEnrichments(
	ctx context.Context,
	tx pgx.Tx,
	activityID string,
	surfaces []SurfaceBreakdown,
	highways []SurfaceBreakdown,
) error {
	batch := &pgx.Batch{}
	for _, s := range surfaces {
		batch.Queue(`
			INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
			VALUES ($1, 'surface', $2, $3, $4, $5)
		`, activityID, s.Type, s.Percentage, s.LengthM, rangesToJSON(s.Ranges))
	}
	for _, h := range highways {
		batch.Queue(`
			INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
			VALUES ($1, 'highway', $2, $3, $4, $5)
		`, activityID, h.Type, h.Percentage, h.LengthM, rangesToJSON(h.Ranges))
	}
	if batch.Len() == 0 {
		return nil
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("track_analysis: write surface/highway enrichments: %w", err)
	}
	return nil
}

func writeForestProtectedTx(
	ctx context.Context,
	tx pgx.Tx,
	activityID string,
	forestPct float64,
	protectedPct float64,
	forestRanges []edgetags.CoordRange,
	protectedRanges []edgetags.CoordRange,
) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
		VALUES ($1, 'forest_pct', '', $2, NULL, $3),
		       ($1, 'protected_pct', '', $4, NULL, $5)
	`, activityID, forestPct, rangesToJSON(forestRanges), protectedPct, rangesToJSON(protectedRanges)); err != nil {
		return fmt.Errorf("track_analysis: write forest/protected enrichments: %w", err)
	}
	return nil
}

func writeDifficultyScalesTx(
	ctx context.Context,
	tx pgx.Tx,
	activityID string,
	pct scalePercentages,
	ranges edgetags.AllRanges,
) error {
	batch := &pgx.Batch{}
	for _, level := range edgetags.SacLevels {
		if pct.Sac[level] > 0 {
			batch.Queue(`
				INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
				VALUES ($1, 'sac_scale', $2, $3, NULL, $4)
			`, activityID, level, pct.Sac[level], rangesToJSON(ranges.Sac[level]))
		}
	}
	for _, level := range edgetags.MtbLevels {
		if pct.Mtb[level] > 0 {
			batch.Queue(`
				INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
				VALUES ($1, 'mtb_scale', $2, $3, NULL, $4)
			`, activityID, level, pct.Mtb[level], rangesToJSON(ranges.Mtb[level]))
		}
	}
	for _, level := range edgetags.VfLevels {
		if pct.Vf[level] > 0 {
			batch.Queue(`
				INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
				VALUES ($1, 'vf_scale', $2, $3, NULL, $4)
			`, activityID, level, pct.Vf[level], rangesToJSON(ranges.Vf[level]))
		}
	}
	if pct.VfUnknown > 0 {
		batch.Queue(`
			INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
			VALUES ($1, 'vf_unknown', '', $2, NULL, $3)
		`, activityID, pct.VfUnknown, rangesToJSON(ranges.VfUnknown))
	}
	if batch.Len() == 0 {
		return nil
	}
	if err := tx.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("track_analysis: write difficulty enrichments: %w", err)
	}
	return nil
}

func computeScalePercentages(tags1, tags2 []uint64, totalPoints int) scalePercentages {
	out := scalePercentages{
		Sac: make(map[string]float64, len(edgetags.SacLevels)),
		Mtb: make(map[string]float64, len(edgetags.MtbLevels)),
		Vf:  make(map[string]float64, len(edgetags.VfLevels)),
	}
	if totalPoints <= 0 {
		return out
	}

	for i, t1 := range tags1 {
		if sac := graph.SacScaleShort(graph.DecodeSacScale(t1)); sac != "" {
			out.Sac[sac]++
		}
		if mtb := graph.MtbScaleShort(graph.DecodeMtbScale(t1)); mtb != "" {
			out.Mtb[mtb]++
		}
		if i < len(tags2) {
			t2 := tags2[i]
			if vf := graph.ViaFerrataScaleShort(graph.DecodeViaFerrataScale(t2)); vf != "" {
				out.Vf[vf]++
			}
			if graph.DecodeViaFerrataScale(t2) == 0 && graph.DecodeHighway(t1) == 22 {
				out.VfUnknown++
			}
		}
	}

	denom := float64(totalPoints)
	for k, v := range out.Sac {
		out.Sac[k] = v / denom * 100.0
	}
	for k, v := range out.Mtb {
		out.Mtb[k] = v / denom * 100.0
	}
	for k, v := range out.Vf {
		out.Vf[k] = v / denom * 100.0
	}
	out.VfUnknown = out.VfUnknown / denom * 100.0
	return out
}

func rangesToJSON(ranges []edgetags.CoordRange) any {
	if len(ranges) == 0 {
		return nil
	}
	b, err := json.Marshal(ranges)
	if err != nil {
		return nil
	}
	return string(b)
}
