// services/api/internal/osm/rasterize.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnrichForestPct berechnet den Waldanteil jeder Way via BRouter-Hybrid Point-Sampling.
// 3-Phasen-Ansatz: ST_CoveredBy für vollständig im Wald liegende Ways (~70%, instant),
// DWithin-Filter für Ways weit weg von jedem Wald (instant), Point-Sampling alle 25m
// für Grenzfälle (~30%). Nutzt ST_Intersects (boolean) statt ST_Intersection (Geometrie).
func EnrichForestPct(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
	slog.Info("enrichment: forest_pct starting (BRouter-hybrid point-sampling)")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment forest_pct: SET work_mem failed (non-fatal)", "error", err)
	}

	// Phase 1: Ways komplett im Wald → forest_pct = 100
	// ST_CoveredBy statt ST_Within: behandelt MultiPolygone mit Löchern korrekt.
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways w SET forest_pct = 100
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NULL
		  AND EXISTS (
			SELECT 1 FROM osm_forests f
			WHERE ST_CoveredBy(w.geom, f.geom)
		  )
	`)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: within phase: %w", err)
	}
	slog.Info("enrichment: forest_pct ST_Within phase complete")

	// Prüfe ob subdivided Tabelle existiert (Fallback auf Original sonst)
	var forestSubExists bool
	conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_forests_subdivided')").Scan(&forestSubExists) //nolint:errcheck
	forestTable := "osm_forests"
	if forestSubExists {
		forestTable = "osm_forests_subdivided"
	} else {
		slog.Warn("enrichment: osm_forests_subdivided not found, falling back to osm_forests")
	}

	// Phase 2: Ways weit weg von jedem Wald → forest_pct = 0
	// 0.0015° ≈ ~120m bei 47°N (konservativ)
	_, err = conn.Exec(ctx, fmt.Sprintf(`
		UPDATE osm_ways w SET forest_pct = 0
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NULL
		  AND NOT EXISTS (
			SELECT 1 FROM %s fs
			WHERE ST_DWithin(w.geom, fs.geom, 0.0015)
		  )
	`, forestTable))
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: no-forest phase: %w", err)
	}
	slog.Info("enrichment: forest_pct no-forest phase complete")

	// Phase 3: Grenzfälle — Point-Sampling alle 25m entlang der Way
	// ST_Intersects pro Punkt (boolean, billig) statt ST_Intersection (Geometrie, teuer)
	_, err = conn.Exec(ctx, `
		DROP TABLE IF EXISTS _forest_coverage;
		CREATE UNLOGGED TABLE _forest_coverage AS
		SELECT w.way_id,
			ROUND(100.0 *
				COUNT(*) FILTER (WHERE EXISTS (
					SELECT 1 FROM `+forestTable+` fs
					WHERE ST_Intersects(fs.geom, pt.geom)
				))::float / NULLIF(COUNT(*), 0)
			)::SMALLINT AS pct
		FROM osm_ways w
		CROSS JOIN LATERAL (
			SELECT (dp).geom
			FROM ST_DumpPoints(
				ST_LineInterpolatePoints(w.geom,
					LEAST(1.0, 25.0 / GREATEST(25.0, ST_Length(w.geom) * 111320 * COS(RADIANS(ST_Y(ST_Centroid(w.geom))))))
				)
			) dp
		) pt
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NULL
		GROUP BY w.way_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: point-sampling phase: %w", err)
	}

	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET forest_pct = c.pct
		FROM _forest_coverage c WHERE osm_ways.way_id = c.way_id
	`)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
		return fmt.Errorf("enrichment forest_pct: update: %w", err)
	}

	// Phase 4: Verbleibende NULLs → 0 (sehr kurze Ways ohne Sample-Punkte)
	_, err = conn.Exec(ctx, "UPDATE osm_ways w SET forest_pct = 0 WHERE highway IS NOT NULL AND forest_pct IS NULL"+bboxClause)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
		return fmt.Errorf("enrichment forest_pct: default: %w", err)
	}

	conn.Exec(ctx, "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
	slog.Info("enrichment: forest_pct complete (BRouter-hybrid)")
	return nil
}

// EnrichProtectedArea bestimmt die Schutzklasse jeder Way via ST_Intersects.
// Keine ST_Intersection nötig — nur boolean Überlappungsprüfung.
// Schutzklassen: 0=kein, 1=allgemein, 2=mittel (class 2/3/4), 3=streng (class 1/1a/1b).
func EnrichProtectedArea(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
	slog.Info("enrichment: protected_area starting (polygon join)")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment protected_area: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment protected_area: SET work_mem failed (non-fatal)", "error", err)
	}

	// Prüfe ob subdivided Tabelle existiert (Fallback auf Original sonst)
	var boundarySubExists bool
	conn.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_boundaries_subdivided')").Scan(&boundarySubExists) //nolint:errcheck
	boundaryTable := "osm_boundaries"
	if boundarySubExists {
		boundaryTable = "osm_boundaries_subdivided"
	} else {
		slog.Warn("enrichment: osm_boundaries_subdivided not found, falling back to osm_boundaries")
	}

	// AND w.protected_area IS NULL — nur resettete Ways
	// If subdivided table exists: join against subdivided (tight BBs → fast GIST),
	// then back to original for boundary_type filtering.
	// If fallback: simple join directly against osm_boundaries.
	var stagingSQL string
	if boundarySubExists {
		stagingSQL = fmt.Sprintf(`
		DROP TABLE IF EXISTS _protected;
		CREATE UNLOGGED TABLE _protected AS
		SELECT w.way_id,
			MAX(CASE
				WHEN b.protect_class IN ('1','1a','1b') THEN 3
				WHEN b.protect_class IN ('2','3','4') THEN 2
				ELSE 1
			END)::SMALLINT AS level
		FROM osm_ways w
		JOIN %s bs ON ST_Intersects(w.geom, bs.geom)
		JOIN osm_boundaries b ON b.osm_id = bs.osm_id AND b.boundary_type IN ('protected_area','national_park')
		WHERE w.highway IS NOT NULL AND w.protected_area IS NULL
		GROUP BY w.way_id`, boundaryTable)
	} else {
		stagingSQL = `
		DROP TABLE IF EXISTS _protected;
		CREATE UNLOGGED TABLE _protected AS
		SELECT w.way_id,
			MAX(CASE
				WHEN b.protect_class IN ('1','1a','1b') THEN 3
				WHEN b.protect_class IN ('2','3','4') THEN 2
				ELSE 1
			END)::SMALLINT AS level
		FROM osm_ways w
		JOIN osm_boundaries b ON ST_Intersects(w.geom, b.geom) AND b.boundary_type IN ('protected_area','national_park')
		WHERE w.highway IS NOT NULL AND w.protected_area IS NULL
		GROUP BY w.way_id`
	}
	_, err = conn.Exec(ctx, stagingSQL)
	if err != nil {
		return fmt.Errorf("enrichment protected_area: create staging: %w", err)
	}

	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET protected_area = p.level
		FROM _protected p WHERE osm_ways.way_id = p.way_id
	`)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _protected") //nolint:errcheck
		return fmt.Errorf("enrichment protected_area: update: %w", err)
	}

	_, err = conn.Exec(ctx, "UPDATE osm_ways w SET protected_area = 0 WHERE highway IS NOT NULL AND protected_area IS NULL"+bboxClause)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _protected") //nolint:errcheck
		return fmt.Errorf("enrichment protected_area: default: %w", err)
	}

	conn.Exec(ctx, "DROP TABLE IF EXISTS _protected") //nolint:errcheck
	slog.Info("enrichment: protected_area complete")
	return nil
}

// EnrichLanduseFromPolygons bestimmt die dominante Landnutzung jeder Way.
// Wählt den Landuse-Typ mit der längsten Überlappung (DISTINCT ON + ORDER BY Schnittlänge).
// Enum: 0=other, 2=farmland, 3=residential, 4=industrial, 5=commercial, 6=quarry.
// Forest (1) und highway-basiert Residential (3) werden separat in EnrichLanduseClass gesetzt.
func EnrichLanduseFromPolygons(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
	slog.Info("enrichment: landuse_from_polygons starting")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment landuse_polygons: acquire conn: %w", err)
	}
	defer conn.Release()

	workMemMB := availableRAMMB() * 10 / 100
	if workMemMB > 512 { workMemMB = 512 }
	if workMemMB < 64 { workMemMB = 64 }
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET work_mem = '%dMB'", workMemMB)); err != nil {
		slog.Warn("enrichment landuse_polygons: SET work_mem failed (non-fatal)", "error", err)
	}

	// AND w.landuse_class IS NULL — nur resettete Ways
	_, err = conn.Exec(ctx, `
		DROP TABLE IF EXISTS _landuse;
		CREATE UNLOGGED TABLE _landuse AS
		SELECT DISTINCT ON (w.way_id) w.way_id,
			(CASE lu.landuse
				WHEN 'farmland' THEN 2 WHEN 'meadow' THEN 2 WHEN 'orchard' THEN 2 WHEN 'vineyard' THEN 2
				WHEN 'residential' THEN 3
				WHEN 'industrial' THEN 4
				WHEN 'commercial' THEN 5 WHEN 'retail' THEN 5
				WHEN 'quarry' THEN 6
				ELSE 0
			END)::SMALLINT AS lc
		FROM osm_ways w
		JOIN osm_landuse lu ON ST_Intersects(w.geom, lu.geom)
		WHERE w.highway IS NOT NULL
		  AND w.landuse_class IS NULL
		  AND lu.landuse IN ('farmland','meadow','orchard','vineyard','residential','industrial','commercial','retail','quarry')
		ORDER BY w.way_id, ST_Length(ST_CollectionExtract(ST_Intersection(w.geom, lu.geom), 2)) DESC NULLS LAST
	`)
	if err != nil {
		return fmt.Errorf("enrichment landuse_polygons: create staging: %w", err)
	}

	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET landuse_class = l.lc
		FROM _landuse l WHERE osm_ways.way_id = l.way_id
	`)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _landuse") //nolint:errcheck
		return fmt.Errorf("enrichment landuse_polygons: update: %w", err)
	}

	_, err = conn.Exec(ctx, "UPDATE osm_ways w SET landuse_class = 0 WHERE highway IS NOT NULL AND landuse_class IS NULL"+bboxClause)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _landuse") //nolint:errcheck
		return fmt.Errorf("enrichment landuse_polygons: default: %w", err)
	}

	conn.Exec(ctx, "DROP TABLE IF EXISTS _landuse") //nolint:errcheck
	slog.Info("enrichment: landuse_from_polygons complete")
	return nil
}
