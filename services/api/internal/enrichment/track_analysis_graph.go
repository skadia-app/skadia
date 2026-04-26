package enrichment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
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

	edgeCoordStart, coalescedTags1, coalescedTags2 := edgetags.CoalesceConsecutive(edgeIdxs, tags1, tags2)
	edgeCoordStart = expandCoalescedStarts(edgeCoordStart, matchedCoordIdxs)
	lastCoordIdx := len(pts) - 1
	allRanges := edgetags.ComputeAllRanges(edgeCoordStart, coalescedTags1, coalescedTags2, lastCoordIdx)
	scalePct := computeScalePercentages(tags1, tags2, len(pts))

	coords := make([]edgetags.LatLng, len(pts))
	for i, pt := range pts {
		coords[i] = edgetags.LatLng{Lat: pt.lat, Lng: pt.lng}
	}
	forestPct, protectedPct := edgetags.AggregateForestProtected(coords, edgeCoordStart, coalescedTags2, lastCoordIdx)

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("track_analysis_graph: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if settings.EnableSurfaceDetection {
		points := classifyGraphMatches(matched, candidates)
		surfaceBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.SurfaceClass })
		highwayBreakdowns := aggregateByField(points, func(p matchedPoint) string { return p.HighwayClass })

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

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("track_analysis_graph: commit tx: %w", err)
	}
	return nil
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
			VALUES ($1, 'surface', $2, $3, $4, NULL)
		`, activityID, s.Type, s.Percentage, s.LengthM)
	}
	for _, h := range highways {
		batch.Queue(`
			INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m, ranges)
			VALUES ($1, 'highway', $2, $3, $4, NULL)
		`, activityID, h.Type, h.Percentage, h.LengthM)
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
