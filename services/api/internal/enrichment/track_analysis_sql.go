package enrichment

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
	"github.com/jackc/pgx/v5"
)

type sqlTrackTagStats struct {
	totalPoints  int
	forestPct    float64
	protectedPct float64
	scales       scalePercentages
}

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

	if settings.EnableSurfaceDetection {
		surfaceBreakdowns, highwayBreakdowns, err := computeSurfaceBreakdownsSQLTx(ctx, tx, activityID)
		if err != nil {
			return err
		}
		if err := writeSurfaceHighwayEnrichments(ctx, tx, activityID, surfaceBreakdowns, highwayBreakdowns); err != nil {
			return err
		}
		if err := persistSurfaceResultsTx(ctx, tx, activityID, surfaceBreakdowns, highwayBreakdowns); err != nil {
			return err
		}
		if err := writeForestProtectedSQLTx(ctx, tx, activityID); err != nil {
			return err
		}
	}

	if settings.EnableDifficultyScales {
		if err := writeDifficultyScalesSQLTx(ctx, tx, activityID); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("track_analysis_sql: commit tx: %w", err)
	}
	return nil
}

func writeForestProtectedSQLTx(ctx context.Context, tx pgx.Tx, activityID string) error {
	stats, err := computeTrackTagStatsSQLTx(ctx, tx, activityID)
	if err != nil {
		return err
	}
	return writeForestProtectedTx(ctx, tx, activityID, stats.forestPct, stats.protectedPct, nil, nil)
}

func writeDifficultyScalesSQLTx(ctx context.Context, tx pgx.Tx, activityID string) error {
	stats, err := computeTrackTagStatsSQLTx(ctx, tx, activityID)
	if err != nil {
		return err
	}
	return writeDifficultyScalesTx(ctx, tx, activityID, stats.scales, edgetags.AllRanges{})
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
