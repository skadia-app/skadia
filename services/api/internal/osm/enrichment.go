// services/api/internal/osm/enrichment.go
package osm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// EnrichForestPct calculates forest coverage percentage for each way in osm_ways.
// Uses batched 50km tiles (SRID 3857 = meters) for memory control.
// ST_Length ratio in 3857 is acceptable — Mercator distortion cancels out in numerator/denominator.
// Requires: osm_ways.forest_pct column (migration 000157), osm_forests table.
func EnrichForestPct(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting forest_pct")
	_, err := pool.Exec(ctx, `
		DO $$
		DECLARE
			tile RECORD;
			data_extent GEOMETRY;
			processed INT := 0;
		BEGIN
			-- SRID 3857: units are meters. 10000 = 10km buffer.
			SELECT ST_Expand(ST_Extent(geom), 10000) INTO data_extent
			FROM osm_ways WHERE highway IS NOT NULL;

			IF data_extent IS NULL THEN
				RAISE NOTICE 'No osm_ways with highway found';
				RETURN;
			END IF;

			-- 50000 = 50km grid cells (3857 meters)
			FOR tile IN SELECT * FROM ST_SquareGrid(50000, data_extent) LOOP
				UPDATE osm_ways w SET forest_pct = COALESCE(sub.pct, 0)
				FROM (
					SELECT w2.way_id,
						LEAST(100, ROUND(
							100.0 * SUM(ST_Length(ST_Intersection(w2.geom, f.geom)))
							/ NULLIF(ST_Length(w2.geom), 0)
						))::SMALLINT AS pct
					FROM osm_ways w2
					LEFT JOIN osm_forests f ON ST_Intersects(w2.geom, f.geom)
					WHERE ST_Intersects(w2.geom, tile.geom)
					  AND w2.highway IS NOT NULL
					GROUP BY w2.way_id, w2.geom
				) sub
				WHERE w.way_id = sub.way_id;
				processed := processed + 1;
			END LOOP;
			RAISE NOTICE 'forest_pct: processed % tiles', processed;
		END $$;
	`)
	if err != nil {
		return fmt.Errorf("enrichment forest_pct: %w", err)
	}
	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after forest_pct failed", "error", err)
	}
	slog.Info("enrichment: forest_pct complete")
	return nil
}

// EnrichNoiseScore calculates noise exposure score (0-100) for each way.
// Sources: road hierarchy proximity + aerodromes.
// Uses pool.Acquire() for connection pinning (TEMP VIEW needs same connection).
// SRID 3857: must ST_Transform to 4326 before ::geography cast.
// P3-TODO: Add railway lines and industrial areas when Lua importer is extended.
func EnrichNoiseScore(ctx context.Context, pool *pgxpool.Pool) error {
	slog.Info("enrichment: starting noise_score")

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: acquire conn: %w", err)
	}
	defer conn.Release()

	// Create temp view for noise sources (all geometry cast to untyped for UNION compat)
	_, err = conn.Exec(ctx, `
		CREATE TEMP VIEW noise_sources AS
			SELECT geom::geometry, 'motorway' AS source_type, 500 AS buffer_m, 1.0::float AS weight
			FROM osm_ways WHERE highway IN ('motorway', 'motorway_link', 'trunk', 'trunk_link')
			UNION ALL
			SELECT geom::geometry, 'primary', 200, 0.6
			FROM osm_ways WHERE highway IN ('primary', 'primary_link')
			UNION ALL
			SELECT geom::geometry, 'secondary', 100, 0.3
			FROM osm_ways WHERE highway IN ('secondary', 'secondary_link')
			UNION ALL
			SELECT geom::geometry, 'aerodrome', 3000, 0.9
			FROM osm_pois WHERE type = 'aerodrome'
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: create view: %w", err)
	}

	// Calculate noise score
	// SRID 3857: must ST_Transform to 4326 before ::geography cast for correct distances
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways w SET noise_score = sub.score
		FROM (
			SELECT w2.way_id,
				LEAST(100, GREATEST(0, ROUND(MAX(
					n.weight * 100.0 * (1.0 - ST_Distance(
						ST_Transform(w2.geom, 4326)::geography,
						ST_Transform(n.geom, 4326)::geography
					) / n.buffer_m)
				))))::SMALLINT AS score
			FROM osm_ways w2
			JOIN noise_sources n ON ST_DWithin(
				ST_Transform(w2.geom, 4326)::geography,
				ST_Transform(n.geom, 4326)::geography,
				n.buffer_m
			)
			WHERE w2.highway IS NOT NULL
			GROUP BY w2.way_id
		) sub
		WHERE w.way_id = sub.way_id
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: update: %w", err)
	}

	// Default 0 for ways with no nearby noise sources
	_, err = conn.Exec(ctx, `
		UPDATE osm_ways SET noise_score = 0
		WHERE noise_score IS NULL AND highway IS NOT NULL
	`)
	if err != nil {
		return fmt.Errorf("enrichment noise_score: default: %w", err)
	}

	conn.Exec(ctx, "DROP VIEW IF EXISTS noise_sources")

	_, err = pool.Exec(ctx, "ANALYZE osm_ways")
	if err != nil {
		slog.Warn("enrichment: ANALYZE after noise_score failed", "error", err)
	}
	slog.Info("enrichment: noise_score complete")
	return nil
}
