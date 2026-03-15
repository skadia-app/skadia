// services/api/internal/osm/rasterize.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// checkRasterExists prüft ob Raster-Tiles für einen Layer bereits existieren.
func checkRasterExists(ctx context.Context, pool *pgxpool.Pool, source string) (bool, error) {
	var count int
	err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM enrichment.raster_tiles WHERE source = $1", source).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checkRasterExists(%s): %w", source, err)
	}
	return count > 0, nil
}

// AnalyzeEnrichmentRasters führt ANALYZE auf enrichment.raster_tiles aus.
func AnalyzeEnrichmentRasters(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "ANALYZE enrichment.raster_tiles")
	if err != nil {
		return fmt.Errorf("analyze enrichment.raster_tiles: %w", err)
	}
	slog.Info("enrichment: ANALYZE enrichment.raster_tiles complete")
	return nil
}

// RasterizeForest konvertiert osm_forests in binäre Raster-Tiles (10m Auflösung).
func RasterizeForest(ctx context.Context, pool *pgxpool.Pool) error {
	exists, err := checkRasterExists(ctx, pool, "forest")
	if err != nil {
		return err
	}
	if exists {
		slog.Info("enrichment: rasterize_forest skipped (tiles exist)")
		return nil
	}

	slog.Info("enrichment: rasterize_forest starting")
	_, err = pool.Exec(ctx, `
		INSERT INTO enrichment.raster_tiles (source, rast, resolution_m)
		SELECT 'forest',
			ST_AsRaster(
				ST_Union(ST_Intersection(f.geom, tile.geom)),
				0.00009, -0.00009, 0.0, 0.0,
				'8BUI', 1, 0
			),
			10.0
		FROM osm_forests f,
			 (SELECT geom FROM ST_SquareGrid(0.45,
				(SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326)
				 FROM osm_ways WHERE highway IS NOT NULL))) tile
		WHERE ST_Intersects(f.geom, tile.geom)
		GROUP BY tile.geom
		HAVING ST_Union(ST_Intersection(f.geom, tile.geom)) IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("rasterize_forest: %w", err)
	}
	slog.Info("enrichment: rasterize_forest complete")
	return nil
}

// RasterizeProtected konvertiert osm_boundaries (Schutzgebiete) in Raster-Tiles (25m).
// Loop-Ansatz: je ein Durchlauf pro Schutzklasse.
// Pixel-Werte: 1=allgemein, 2=mittel (class 2/3/4), 3=streng (class 1/1a/1b).
// ST_AsRaster braucht einen KONSTANTEN Pixelwert — pro-Polygon-Werte gehen nicht.
// SampleProtectedArea nutzt MAX() über alle Tiles, daher gewinnt der höchste Wert.
func RasterizeProtected(ctx context.Context, pool *pgxpool.Pool) error {
	exists, err := checkRasterExists(ctx, pool, "protected")
	if err != nil {
		return err
	}
	if exists {
		slog.Info("enrichment: rasterize_protected skipped (tiles exist)")
		return nil
	}

	slog.Info("enrichment: rasterize_protected starting")

	layers := []struct {
		value int
		where string
	}{
		{1, "boundary_type IN ('protected_area','national_park') AND (protect_class IS NULL OR protect_class NOT IN ('1','1a','1b','2','3','4'))"},
		{2, "protect_class IN ('2','3','4')"},
		{3, "protect_class IN ('1','1a','1b')"},
	}

	for _, l := range layers {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO enrichment.raster_tiles (source, rast, resolution_m)
			SELECT 'protected',
				ST_AsRaster(
					ST_Union(ST_Intersection(b.geom, tile.geom)),
					0.000225, -0.000225, 0.0, 0.0,
					'8BUI', %d, 0
				),
				25.0
			FROM osm_boundaries b,
				 (SELECT geom FROM ST_SquareGrid(0.45,
					(SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326)
					 FROM osm_ways WHERE highway IS NOT NULL))) tile
			WHERE (%s)
			  AND ST_Intersects(b.geom, tile.geom)
			GROUP BY tile.geom
			HAVING ST_Union(ST_Intersection(b.geom, tile.geom)) IS NOT NULL
		`, l.value, l.where))
		if err != nil {
			return fmt.Errorf("rasterize_protected (value=%d): %w", l.value, err)
		}
	}

	slog.Info("enrichment: rasterize_protected complete")
	return nil
}

// RasterizeLanduse konvertiert osm_landuse in Raster-Tiles (25m).
// Pixel-Werte: 2=farmland, 3=residential, 4=industrial, 5=commercial/retail, 6=quarry.
// Forest (1) kommt NICHT aus diesem Raster — wird aus forest_pct abgeleitet.
func RasterizeLanduse(ctx context.Context, pool *pgxpool.Pool) error {
	exists, err := checkRasterExists(ctx, pool, "landuse")
	if err != nil {
		return err
	}
	if exists {
		slog.Info("enrichment: rasterize_landuse skipped (tiles exist)")
		return nil
	}

	slog.Info("enrichment: rasterize_landuse starting")

	layers := []struct {
		value int
		types string
	}{
		{2, "'farmland','meadow','orchard','vineyard'"},
		{3, "'residential'"},
		{4, "'industrial'"},
		{5, "'commercial','retail'"},
		{6, "'quarry'"},
	}

	for _, l := range layers {
		_, err := pool.Exec(ctx, fmt.Sprintf(`
			INSERT INTO enrichment.raster_tiles (source, rast, resolution_m)
			SELECT 'landuse',
				ST_AsRaster(
					ST_Union(ST_Intersection(lu.geom, tile.geom)),
					0.000225, -0.000225, 0.0, 0.0,
					'8BUI', %d, 0
				),
				25.0
			FROM osm_landuse lu,
				 (SELECT geom FROM ST_SquareGrid(0.45,
					(SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326)
					 FROM osm_ways WHERE highway IS NOT NULL))) tile
			WHERE lu.landuse IN (%s)
			  AND ST_Intersects(lu.geom, tile.geom)
			GROUP BY tile.geom
			HAVING ST_Union(ST_Intersection(lu.geom, tile.geom)) IS NOT NULL
		`, l.value, l.types))
		if err != nil {
			return fmt.Errorf("rasterize_landuse (value=%d): %w", l.value, err)
		}
	}

	slog.Info("enrichment: rasterize_landuse complete")
	return nil
}

// SampleForestPct samplet forest-Raster entlang jeder Way-Geometrie.
// Nutzt PL/pgSQL DO-Block für Tile-Iteration (pgx kann PostgreSQL geometry nicht direkt scannen).
func SampleForestPct(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: sample_forest_pct starting")

	_, err := pool.Exec(ctx, `
		DO $$
		DECLARE
			tile RECORD;
			data_extent GEOMETRY;
			processed INT := 0;
		BEGIN
			SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326) INTO data_extent
			FROM osm_ways WHERE highway IS NOT NULL;
			IF data_extent IS NULL THEN
				RAISE NOTICE 'No osm_ways found';
				RETURN;
			END IF;

			FOR tile IN SELECT * FROM ST_SquareGrid(0.45, data_extent) LOOP
				UPDATE osm_ways w SET forest_pct = sub.pct
				FROM (
					SELECT w2.way_id,
						ROUND(100.0 * COUNT(*) FILTER (
							WHERE ST_Value(r.rast, pt.geom) = 1
						) / NULLIF(COUNT(*), 0))::SMALLINT AS pct
					FROM osm_ways w2
					CROSS JOIN LATERAL ST_DumpPoints(
						ST_LineInterpolatePoints(w2.geom, LEAST(20.0 / NULLIF(ST_Length(w2.geom::geography), 0), 0.5))
					) AS pt
					JOIN enrichment.raster_tiles r
						ON r.source = 'forest'
						AND ST_Intersects(r.rast, pt.geom)
					WHERE w2.highway IS NOT NULL
					  AND ST_Length(w2.geom::geography) > 0
					  AND ST_Intersects(ST_PointOnSurface(w2.geom), tile.geom)
					GROUP BY w2.way_id
				) sub
				WHERE w.way_id = sub.way_id;
				processed := processed + 1;
			END LOOP;
			RAISE NOTICE 'sample_forest_pct: processed % tiles', processed;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("sample_forest_pct: %w", err)
	}

	// NULL-Default: Ways ohne Raster-Coverage → 0
	_, err = pool.Exec(ctx, "UPDATE osm_ways SET forest_pct = 0 WHERE highway IS NOT NULL AND forest_pct IS NULL")
	if err != nil {
		return fmt.Errorf("sample_forest_pct: null default: %w", err)
	}

	slog.Info("enrichment: sample_forest_pct complete")
	return nil
}

// SampleProtectedArea samplet protected-Raster (MAX über Sample-Punkte).
// PL/pgSQL DO-Block für Tile-Iteration (analog SampleForestPct).
func SampleProtectedArea(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: sample_protected starting")

	_, err := pool.Exec(ctx, `
		DO $$
		DECLARE
			tile RECORD;
			data_extent GEOMETRY;
			processed INT := 0;
		BEGIN
			SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326) INTO data_extent
			FROM osm_ways WHERE highway IS NOT NULL;
			IF data_extent IS NULL THEN RETURN; END IF;

			FOR tile IN SELECT * FROM ST_SquareGrid(0.45, data_extent) LOOP
				UPDATE osm_ways w SET protected_area = sub.level
				FROM (
					SELECT w2.way_id,
						GREATEST(0, LEAST(3, MAX(ST_Value(r.rast, pt.geom))))::SMALLINT AS level
					FROM osm_ways w2
					CROSS JOIN LATERAL ST_DumpPoints(
						ST_LineInterpolatePoints(w2.geom, LEAST(20.0 / NULLIF(ST_Length(w2.geom::geography), 0), 0.5))
					) AS pt
					JOIN enrichment.raster_tiles r
						ON r.source = 'protected'
						AND ST_Intersects(r.rast, pt.geom)
					WHERE w2.highway IS NOT NULL
					  AND ST_Length(w2.geom::geography) > 0
					  AND ST_Intersects(ST_PointOnSurface(w2.geom), tile.geom)
					GROUP BY w2.way_id
				) sub
				WHERE w.way_id = sub.way_id;
				processed := processed + 1;
			END LOOP;
			RAISE NOTICE 'sample_protected: processed % tiles', processed;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("sample_protected: %w", err)
	}

	_, err = pool.Exec(ctx, "UPDATE osm_ways SET protected_area = 0 WHERE highway IS NOT NULL AND protected_area IS NULL")
	if err != nil {
		return fmt.Errorf("sample_protected: null default: %w", err)
	}

	slog.Info("enrichment: sample_protected complete")
	return nil
}

// SampleLanduse samplet landuse-Raster (MODE über Sample-Punkte).
// PL/pgSQL DO-Block für Tile-Iteration (analog SampleForestPct).
func SampleLanduse(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: sample_landuse starting")

	_, err := pool.Exec(ctx, `
		DO $$
		DECLARE
			tile RECORD;
			data_extent GEOMETRY;
			processed INT := 0;
		BEGIN
			SELECT ST_SetSRID(ST_Expand(ST_Extent(geom), 0.01)::geometry, 4326) INTO data_extent
			FROM osm_ways WHERE highway IS NOT NULL;
			IF data_extent IS NULL THEN RETURN; END IF;

			FOR tile IN SELECT * FROM ST_SquareGrid(0.45, data_extent) LOOP
				UPDATE osm_ways w SET landuse_class = sub.lc
				FROM (
					SELECT w2.way_id,
						(MODE() WITHIN GROUP (ORDER BY ST_Value(r.rast, pt.geom)::INT))::SMALLINT AS lc
					FROM osm_ways w2
					CROSS JOIN LATERAL ST_DumpPoints(
						ST_LineInterpolatePoints(w2.geom, LEAST(20.0 / NULLIF(ST_Length(w2.geom::geography), 0), 0.5))
					) AS pt
					JOIN enrichment.raster_tiles r
						ON r.source = 'landuse'
						AND ST_Intersects(r.rast, pt.geom)
					WHERE w2.highway IS NOT NULL
					  AND ST_Length(w2.geom::geography) > 0
					  AND ST_Intersects(ST_PointOnSurface(w2.geom), tile.geom)
					  AND ST_Value(r.rast, pt.geom) > 0
					GROUP BY w2.way_id
				) sub
				WHERE w.way_id = sub.way_id;
				processed := processed + 1;
			END LOOP;
			RAISE NOTICE 'sample_landuse: processed % tiles', processed;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("sample_landuse: %w", err)
	}

	_, err = pool.Exec(ctx, "UPDATE osm_ways SET landuse_class = 0 WHERE highway IS NOT NULL AND landuse_class IS NULL")
	if err != nil {
		return fmt.Errorf("sample_landuse: null default: %w", err)
	}

	slog.Info("enrichment: sample_landuse complete")
	return nil
}
