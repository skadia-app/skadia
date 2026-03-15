// services/api/internal/osm/rasterize.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnrichForestPct berechnet den Waldanteil jeder Way via direkte Polygon-Intersection.
// Ergebnis: forest_pct = Prozent der Way-Länge die durch osm_forests verläuft.
// Nutzt CREATE UNLOGGED TABLE (kann parallel workers nutzen) + UPDATE FROM (schneller PK-Join).
// Genauer als Raster-Sampling: exakte geometrische Schnittlänge statt diskretisierte 10m-Pixel.
func EnrichForestPct(ctx context.Context, pool *pgxpool.Pool, bboxClause string) error {
	slog.Info("enrichment: forest_pct starting (polygon join)")

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

	// Phase 1: Berechnung in Staging-Table (nutzt parallel workers via CTAS)
	// AND w.forest_pct IS NULL — nur resettete Ways (BBox-Reset hat nicht-betroffene bereits gesetzt)
	_, err = conn.Exec(ctx, `
		DROP TABLE IF EXISTS _forest_coverage;
		CREATE UNLOGGED TABLE _forest_coverage AS
		SELECT w.way_id,
			LEAST(100, ROUND(100.0 * SUM(ST_Length(ST_CollectionExtract(ST_Intersection(w.geom, f.geom), 2)::geography))
				/ NULLIF(ST_Length(w.geom::geography), 0)))::SMALLINT AS pct
		FROM osm_ways w
		JOIN osm_forests f ON ST_Intersects(w.geom, f.geom)
		WHERE w.highway IS NOT NULL AND w.forest_pct IS NULL AND ST_Length(w.geom::geography) > 0
		GROUP BY w.way_id, w.geom
	`)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: create staging: %w", err)
	}

	// Phase 2: Schreiben (schneller PK-Join, kein Spatial)
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET forest_pct = c.pct
		FROM _forest_coverage c WHERE osm_ways.way_id = c.way_id
	`)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
		return fmt.Errorf("enrichment forest_pct: update: %w", err)
	}

	// Phase 3: Default 0 für Ways ohne Wald-Überlappung
	_, err = conn.Exec(ctx, "UPDATE osm_ways w SET forest_pct = 0 WHERE highway IS NOT NULL AND forest_pct IS NULL"+bboxClause)
	if err != nil {
		conn.Exec(context.Background(), "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
		return fmt.Errorf("enrichment forest_pct: default: %w", err)
	}

	conn.Exec(ctx, "DROP TABLE IF EXISTS _forest_coverage") //nolint:errcheck
	slog.Info("enrichment: forest_pct complete")
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

	// AND w.protected_area IS NULL — nur resettete Ways
	_, err = conn.Exec(ctx, `
		DROP TABLE IF EXISTS _protected;
		CREATE UNLOGGED TABLE _protected AS
		SELECT w.way_id,
			MAX(CASE
				WHEN b.protect_class IN ('1','1a','1b') THEN 3
				WHEN b.protect_class IN ('2','3','4') THEN 2
				ELSE 1
			END)::SMALLINT AS level
		FROM osm_ways w
		JOIN osm_boundaries b ON ST_Intersects(w.geom, b.geom)
		WHERE w.highway IS NOT NULL
		  AND w.protected_area IS NULL
		  AND b.boundary_type IN ('protected_area','national_park')
		GROUP BY w.way_id
	`)
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
		ORDER BY w.way_id, ST_Length(ST_CollectionExtract(ST_Intersection(w.geom, lu.geom), 2)::geography) DESC NULLS LAST
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
