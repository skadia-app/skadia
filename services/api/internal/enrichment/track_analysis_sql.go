package enrichment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type sqlTrackTagStats struct {
	totalPoints  int
	forestPct    float64
	protectedPct float64
	scales       scalePercentages
}

// enrichTrackAnalysisSQL is the PostGIS-lateral-join fallback path.
//
// INVARIANT: this function is the sole writer to activity_enrichments for the
// given activity within this call. Both the graph path
// (enrichTrackAnalysisGraph) and this SQL fallback MUST DELETE+INSERT inside
// their own Tx; no outer-Tx writes are allowed. Callers (re_enrich,
// recompute, backfill) must not pre-DELETE — see regression case for the data-loss
// window that creates. Track_analysis.go falls through from graph to SQL on
// error, which is only safe because both paths are self-contained.
func enrichTrackAnalysisSQL(
	ctx context.Context,
	db *database.DB,
	activityID string,
	settings *model.EnrichmentSettings,
) error {
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("track_analysis_sql: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	if err := deleteExistingTrackAnalysisTx(ctx, tx, activityID); err != nil {
		return err
	}

	var tagStats sqlTrackTagStats
	tagStatsLoaded := false
	loadTagStats := func() (sqlTrackTagStats, error) {
		if tagStatsLoaded {
			return tagStats, nil
		}
		stats, err := computeTrackTagStatsSQLTx(ctx, tx, activityID)
		if err != nil {
			return sqlTrackTagStats{}, err
		}
		tagStats = stats
		tagStatsLoaded = true
		return tagStats, nil
	}

	if settings.EnableSurfaceDetection {
		surfaceBreakdowns, highwayBreakdowns, err := computeSurfaceBreakdownsSQLTx(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if err := writeSurfaceHighwayEnrichments(ctx, tx, activityID, surfaceBreakdowns, highwayBreakdowns); err != nil {
			return err
		}
		stats, err := loadTagStats()
		if err != nil {
			return err
		}
		if err := writeForestProtectedTx(ctx, tx, activityID, stats.forestPct, stats.protectedPct, nil, nil); err != nil {
			return err
		}
	}

	if settings.EnableDifficultyScales {
		stats, err := loadTagStats()
		if err != nil {
			return err
		}
		// SQL fallback never has per-edge ranges — only the graph path can
		// produce coordinate-index-aligned ranges from the snapped track. Pass
		// zero-value AllRanges{} explicitly to make the absence intentional;
		// writeDifficultyScalesTx then writes scale percentages with NULL
		// ranges, which is the contract documented in
		// activity_enrichment_meta.path = 'sql' (see migration 000329).
		if err := writeDifficultyScalesTx(ctx, tx, activityID, stats.scales, edgetags.AllRanges{}); err != nil {
			return err
		}
	}

	// Persist enrichment path metadata so that the track-summary handler can read
	// graph_snap_used directly without a heuristic (see activity_enrichment_meta).
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_enrichment_meta (activity_id, path, resample_spacing_m, computed_at)
		VALUES ($1, 'sql', $2, NOW())
		ON CONFLICT (activity_id) DO UPDATE
			SET path = EXCLUDED.path,
			    resample_spacing_m = EXCLUDED.resample_spacing_m,
			    computed_at = EXCLUDED.computed_at
	`, activityID, surfaceResampleM); err != nil {
		return fmt.Errorf("track_analysis_sql: upsert enrichment meta: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("track_analysis_sql: commit tx: %w", err)
	}
	return nil
}

func computeTrackTagStatsSQLTx(ctx context.Context, tx pgx.Tx, activityID string) (sqlTrackTagStats, error) {
	rows, err := tx.Query(ctx, `
		WITH route_pts AS (
		    SELECT (dp).path[1] AS idx, (dp).geom AS pt
		    FROM (
		        SELECT ST_DumpPoints(
		            ST_Segmentize(ST_Simplify(track_full, 0.00005)::geography, $2)::geometry
		        ) AS dp
		        FROM activity_tracks WHERE activity_id = $1
		    ) sub
		),
		matched AS (
		    SELECT rp.idx, w.highway, w.surface, w.sac_scale, w.mtb_scale,
		           w.via_ferrata_scale, w.forest_pct, w.protected_area,
		           ST_Distance(rp.pt::geography, w.geom::geography) AS dist_m
		    FROM route_pts rp
		    LEFT JOIN LATERAL (
		        SELECT w.highway, w.surface, w.sac_scale, w.mtb_scale,
		               w.via_ferrata_scale, w.forest_pct, w.protected_area, w.geom
		        FROM osm_ways w
		        WHERE w.geom && ST_Expand(rp.pt, $3)
		        ORDER BY w.geom <-> rp.pt
		        LIMIT 1
		    ) w ON true
		)
		SELECT highway, surface, sac_scale, mtb_scale, via_ferrata_scale,
		       forest_pct, protected_area, dist_m
		FROM matched
		ORDER BY idx
	`, activityID, surfaceResampleM, surfaceBBoxExpand)
	if err != nil {
		return sqlTrackTagStats{}, fmt.Errorf("track_analysis_sql: query tag stats: %w", err)
	}
	defer rows.Close()

	var tags1 []uint64
	var tags2 []uint64
	var total int
	var forestSum float64
	var protectedCount int

	for rows.Next() {
		total++
		var highway, surface, sacScale, mtbScale, vfScale *string
		var forestPct, protectedArea *int16
		var distM *float64
		if err := rows.Scan(&highway, &surface, &sacScale, &mtbScale, &vfScale, &forestPct, &protectedArea, &distM); err != nil {
			return sqlTrackTagStats{}, fmt.Errorf("track_analysis_sql: scan tag stats: %w", err)
		}
		if distM == nil || *distM > surfaceMatchToleranceM {
			continue
		}

		rawTags := graph.RawTags{
			Highway:         stringValue(highway),
			Surface:         stringValue(surface),
			SacScale:        stringValue(sacScale),
			MtbScale:        stringValue(mtbScale),
			ViaFerrataScale: stringValue(vfScale),
		}
		rawEnrichment := graph.RawEnrichment{
			ForestPct:     clampPercentInt16(forestPct),
			ProtectedArea: intValue(protectedArea),
		}
		t1 := graph.EncodeOSMTags(&rawTags)
		t2 := graph.EncodeEnrichmentTags(&rawEnrichment)
		tags1 = append(tags1, t1)
		tags2 = append(tags2, t2)

		forestSum += float64(rawEnrichment.ForestPct)
		if rawEnrichment.ProtectedArea > 0 {
			protectedCount++
		}
	}
	if err := rows.Err(); err != nil {
		return sqlTrackTagStats{}, fmt.Errorf("track_analysis_sql: rows tag stats: %w", err)
	}

	stats := sqlTrackTagStats{
		totalPoints: total,
		scales:      computeScalePercentages(tags1, tags2, total),
	}
	if total > 0 {
		stats.forestPct = forestSum / float64(total)
		stats.protectedPct = float64(protectedCount) / float64(total) * 100.0
	}
	return stats, nil
}

func stringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func intValue(v *int16) int {
	if v == nil {
		return 0
	}
	return int(*v)
}

func clampPercentInt16(v *int16) int {
	if v == nil || *v < 0 {
		return 0
	}
	if *v > 100 {
		return 100
	}
	return int(*v)
}
