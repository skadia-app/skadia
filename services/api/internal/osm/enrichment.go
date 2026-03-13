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
