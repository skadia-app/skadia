// services/api/internal/osm/rasterize.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnrichForestPct berechnet den Waldanteil jeder Way via BRouter-Hybrid Point-Sampling (chunked).
// 4-Phasen-Ansatz: ST_CoveredBy für vollständig im Wald liegende Ways (~70%, instant),
// DWithin-Filter für Ways weit weg von jedem Wald (instant), Point-Sampling alle 25m
// für Grenzfälle (~30%), Default 0 für Restfälle. Nutzt ST_Intersects (boolean).
// All phases process way_ids in 50k chunks for resume support and bounded memory.
func EnrichForestPct(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: forest_pct starting (chunked)")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 {
		workMemMB = 512
	}
	if workMemMB < 64 {
		workMemMB = 64
	}
	conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)) //nolint:errcheck

	minID, maxID, err := wayIDRange(ctx, conn, "forest_pct", bboxClause)
	if err != nil || maxID == 0 {
		slog.Info("enrichment: forest_pct — no NULL rows, skipping")
		return nil
	}
	slog.Info("enrichment: forest_pct way_id range", "min", minID, "max", maxID)

	// Phase 1: Ways completely inside forest → forest_pct = 100
	if _, err := chunkedExec(ctx, conn, `
		UPDATE osm_ways w SET forest_pct = 100
		WHERE w.way_id BETWEEN $1 AND $2
		  AND w.highway IS NOT NULL AND w.forest_pct IS NULL
		  AND EXISTS (
			SELECT 1 FROM osm_forests f
			WHERE ST_CoveredBy(w.geom, f.geom)
		  )
	`, minID, maxID, defaultChunkSize, "forest_pct:within", progressFn); err != nil {
		return fmt.Errorf("enrichment forest_pct: within phase: %w", err)
	}
	slog.Info("enrichment: forest_pct within phase complete")

	// Check subdivided table
	var forestSubExists bool
	conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_forests_subdivided')").Scan(&forestSubExists) //nolint:errcheck
	forestTable := "osm_forests"
	if forestSubExists {
		forestTable = "osm_forests_subdivided"
	} else {
		slog.Warn("enrichment: osm_forests_subdivided not found, falling back to osm_forests")
	}

	// Refresh range (Phase 1 set many rows)
	minID, maxID, _ = wayIDRange(ctx, conn, "forest_pct", bboxClause)
	if maxID == 0 {
		slog.Info("enrichment: forest_pct — all done after within phase")
		return nil
	}

	// Phase 2: Ways far from any forest → forest_pct = 0
	if _, err := chunkedExec(ctx, conn, fmt.Sprintf(`
		UPDATE osm_ways w SET forest_pct = 0
		WHERE w.way_id BETWEEN $1 AND $2
		  AND w.highway IS NOT NULL AND w.forest_pct IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM %s fs
			WHERE ST_DWithin(w.geom, fs.geom, 0.0015)
		  )
	`, forestTable), minID, maxID, defaultChunkSize, "forest_pct:no-forest", progressFn); err != nil {
		return fmt.Errorf("enrichment forest_pct: no-forest phase: %w", err)
	}
	slog.Info("enrichment: forest_pct no-forest phase complete")

	// Refresh range (Phase 2 set many more rows)
	minID, maxID, _ = wayIDRange(ctx, conn, "forest_pct", bboxClause)
	if maxID == 0 {
		slog.Info("enrichment: forest_pct — all done after no-forest phase")
		return nil
	}

	// Phase 3: Edge cases — point-sampling every 25m (chunked, no CTAS)
	if _, err := chunkedExec(ctx, conn, `
		UPDATE osm_ways w SET forest_pct = sub.pct
		FROM (
			SELECT w2.way_id,
				ROUND(100.0 *
					COUNT(*) FILTER (WHERE EXISTS (
						SELECT 1 FROM `+forestTable+` fs
						WHERE ST_Intersects(fs.geom, pt.geom)
					))::float / NULLIF(COUNT(*), 0)
				)::SMALLINT AS pct
			FROM osm_ways w2
			CROSS JOIN LATERAL (
				SELECT (dp).geom
				FROM ST_DumpPoints(
					ST_LineInterpolatePoints(w2.geom,
						LEAST(1.0, 25.0 / GREATEST(25.0, ST_Length(w2.geom::geography)))
					)
				) dp
			) pt
			WHERE w2.way_id BETWEEN $1 AND $2
			  AND w2.highway IS NOT NULL AND w2.forest_pct IS NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`, minID, maxID, defaultChunkSize, "forest_pct:sampling", progressFn); err != nil {
		return fmt.Errorf("enrichment forest_pct: point-sampling phase: %w", err)
	}
	slog.Info("enrichment: forest_pct sampling phase complete")

	// Phase 4: Remaining NULLs → 0 (very short ways without sample points)
	minID, maxID, _ = wayIDRange(ctx, conn, "forest_pct", bboxClause)
	if maxID > 0 {
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET forest_pct = 0
			WHERE w.way_id BETWEEN $1 AND $2
			  AND w.highway IS NOT NULL AND w.forest_pct IS NULL
		`, minID, maxID, defaultChunkSize, "forest_pct:default", progressFn); err != nil {
			return fmt.Errorf("enrichment forest_pct: default: %w", err)
		}
	}

	slog.Info("enrichment: forest_pct complete (chunked)")
	return nil
}

// EnrichProtectedArea bestimmt die Schutzklasse jeder Way via ST_Intersects (chunked).
// Keine ST_Intersection nötig — nur boolean Überlappungsprüfung.
// Schutzklassen: 0=kein, 1=allgemein, 2=mittel (class 2/3/4), 3=streng (class 1/1a/1b).
// All steps process way_ids in 50k chunks — no temp tables needed.
func EnrichProtectedArea(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: protected_area starting (chunked)")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment protected_area: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 {
		workMemMB = 512
	}
	if workMemMB < 64 {
		workMemMB = 64
	}
	conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)) //nolint:errcheck

	minID, maxID, err := wayIDRange(ctx, conn, "protected_area", bboxClause)
	if err != nil || maxID == 0 {
		slog.Info("enrichment: protected_area — no NULL rows, skipping")
		return nil
	}
	slog.Info("enrichment: protected_area way_id range", "min", minID, "max", maxID)

	// Check subdivided table
	var boundarySubExists bool
	conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_boundaries_subdivided')").Scan(&boundarySubExists) //nolint:errcheck

	// Chunked UPDATE FROM subquery — no temp table needed
	var updateSQL string
	if boundarySubExists {
		updateSQL = `
		UPDATE osm_ways w SET protected_area = sub.level
		FROM (
			SELECT w2.way_id,
				MAX(CASE
					WHEN b.protect_class IN ('1','1a','1b') THEN 3
					WHEN b.protect_class IN ('2','3','4') THEN 2
					ELSE 1
				END)::SMALLINT AS level
			FROM osm_ways w2
			JOIN osm_boundaries_subdivided bs ON ST_Intersects(w2.geom, bs.geom)
			JOIN osm_boundaries b ON b.osm_id = bs.osm_id AND b.boundary_type IN ('protected_area','national_park')
			WHERE w2.way_id BETWEEN $1 AND $2
			  AND w2.highway IS NOT NULL AND w2.protected_area IS NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id`
	} else {
		slog.Warn("enrichment: osm_boundaries_subdivided not found, falling back to osm_boundaries")
		updateSQL = `
		UPDATE osm_ways w SET protected_area = sub.level
		FROM (
			SELECT w2.way_id,
				MAX(CASE
					WHEN b.protect_class IN ('1','1a','1b') THEN 3
					WHEN b.protect_class IN ('2','3','4') THEN 2
					ELSE 1
				END)::SMALLINT AS level
			FROM osm_ways w2
			JOIN osm_boundaries b ON ST_Intersects(w2.geom, b.geom)
			  AND b.boundary_type IN ('protected_area','national_park')
			WHERE w2.way_id BETWEEN $1 AND $2
			  AND w2.highway IS NOT NULL AND w2.protected_area IS NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id`
	}

	if _, err := chunkedExec(ctx, conn, updateSQL, minID, maxID, defaultChunkSize, "protected_area:classify", progressFn); err != nil {
		return fmt.Errorf("enrichment protected_area: classify: %w", err)
	}

	// Default 0 for ways not in any protected area (chunked)
	minID, maxID, _ = wayIDRange(ctx, conn, "protected_area", bboxClause)
	if maxID > 0 {
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET protected_area = 0
			WHERE w.way_id BETWEEN $1 AND $2
			  AND w.highway IS NOT NULL AND w.protected_area IS NULL
		`, minID, maxID, defaultChunkSize, "protected_area:default", progressFn); err != nil {
			return fmt.Errorf("enrichment protected_area: default: %w", err)
		}
	}

	slog.Info("enrichment: protected_area complete")
	return nil
}

// EnrichLanduseFromPolygons bestimmt die dominante Landnutzung jeder Way via Point-Sampling Mehrheitsvotum (chunked).
// BRouter-Hybrid-Muster: Punkte alle 50m, ST_Intersects (boolean) statt ST_Intersection (Geometrie).
// Enum: 0=other, 2=farmland/meadow/orchard/vineyard, 3=residential, 4=industrial,
// 5=commercial/retail, 6=quarry, 7=military, 8=landfill.
// Forest (1) und highway-basiertes Residential werden separat in EnrichLanduseClass gesetzt.
// All steps process way_ids in 50k chunks — no temp tables needed.
func EnrichLanduseFromPolygons(ctx context.Context, pool *pgxpool.Pool, bboxClause string, progressFn func(int64)) error {
	slog.Info("enrichment: landuse_from_polygons starting (chunked)")

	// Fallback wenn subdivided Tabelle fehlt
	var landuseSubExists bool
	pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_landuse_subdivided')").Scan(&landuseSubExists) //nolint:errcheck
	landuseTable := "osm_landuse"
	if landuseSubExists {
		landuseTable = "osm_landuse_subdivided"
	} else {
		slog.Warn("enrichment: osm_landuse_subdivided not found, falling back to osm_landuse")
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment landuse_polygons: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 {
		workMemMB = 512
	}
	if workMemMB < 64 {
		workMemMB = 64
	}
	conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)) //nolint:errcheck

	minID, maxID, err := wayIDRange(ctx, conn, "landuse_class", bboxClause)
	if err != nil || maxID == 0 {
		slog.Info("enrichment: landuse_from_polygons — no NULL rows, skipping")
		return nil
	}
	slog.Info("enrichment: landuse_from_polygons way_id range", "min", minID, "max", maxID)

	// Point-Sampling: Punkte alle 50m, Mehrheitsvotum des Landuse-Typs (chunked, no CTAS)
	if _, err := chunkedExec(ctx, conn, `
		UPDATE osm_ways w SET landuse_class = sub.lc
		FROM (
			SELECT DISTINCT ON (way_id) way_id, lc
			FROM (
				SELECT w2.way_id,
					(CASE lus.landuse
						WHEN 'farmland' THEN 2 WHEN 'meadow' THEN 2 WHEN 'orchard' THEN 2 WHEN 'vineyard' THEN 2
						WHEN 'residential' THEN 3
						WHEN 'industrial' THEN 4
						WHEN 'commercial' THEN 5 WHEN 'retail' THEN 5
						WHEN 'quarry' THEN 6
						WHEN 'military' THEN 7
						WHEN 'landfill' THEN 8
						ELSE 0
					END)::SMALLINT AS lc,
					COUNT(*) AS cnt
				FROM osm_ways w2
				CROSS JOIN LATERAL (
					SELECT (dp).geom
					FROM ST_DumpPoints(
						ST_LineInterpolatePoints(w2.geom,
							LEAST(1.0, 50.0 / GREATEST(50.0, ST_Length(w2.geom::geography)))
						)
					) dp
				) pt
				JOIN `+landuseTable+` lus ON ST_Intersects(lus.geom, pt.geom)
				WHERE w2.way_id BETWEEN $1 AND $2
				  AND w2.highway IS NOT NULL AND w2.landuse_class IS NULL
				  AND lus.landuse IN ('farmland','meadow','orchard','vineyard','residential','industrial','commercial','retail','quarry','military','landfill')
				GROUP BY w2.way_id, lc
			) votes
			ORDER BY way_id, cnt DESC
		) sub
		WHERE w.way_id = sub.way_id
	`, minID, maxID, defaultChunkSize, "landuse_polygons:sampling", progressFn); err != nil {
		return fmt.Errorf("enrichment landuse_polygons: point-sampling: %w", err)
	}

	// Default 0 for ways not covered by any landuse polygon (chunked)
	minID, maxID, _ = wayIDRange(ctx, conn, "landuse_class", bboxClause)
	if maxID > 0 {
		if _, err := chunkedExec(ctx, conn, `
			UPDATE osm_ways w SET landuse_class = 0
			WHERE w.way_id BETWEEN $1 AND $2
			  AND w.highway IS NOT NULL AND w.landuse_class IS NULL
		`, minID, maxID, defaultChunkSize, "landuse_polygons:default", progressFn); err != nil {
			return fmt.Errorf("enrichment landuse_polygons: default: %w", err)
		}
	}

	slog.Info("enrichment: landuse_from_polygons complete")
	return nil
}
