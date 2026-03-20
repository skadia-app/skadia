package enrichment

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

var (
	osmWaysExists     bool
	osmWaysExistsOnce sync.Once
)

func checkOSMWaysExists(ctx context.Context, db *database.DB) bool {
	osmWaysExistsOnce.Do(func() {
		var exists bool
		err := db.Pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'osm_ways')`).Scan(&exists)
		if err == nil {
			osmWaysExists = exists
		}
	})
	return osmWaysExists
}

// EnrichSurfaces matches the activity track against osm_ways and writes
// surface-type statistics to activity_surfaces.
//
// Only active in Full mode (osm_ways table must exist and be populated).
// Uses activity_tracks.track_full as the geometry source (identical to surfaces.py).
// Buffer: 10 m (same as Python module). Minimum segment length: 10 m.
//
// On re-enrichment the existing rows for the activity are deleted first.
func EnrichSurfaces(ctx context.Context, db *database.DB, activityID string) error {
	// Guard: skip silently if osm_ways does not exist (Lite mode).
	// Result is cached via sync.Once to avoid per-call information_schema queries.
	if !checkOSMWaysExists(ctx, db) {
		slog.Debug("surfaces: osm_ways not available, skipping")
		return nil
	}

	// Delete existing surface data to allow re-enrichment.
	if _, err := db.Pool.Exec(ctx,
		"DELETE FROM activity_surfaces WHERE activity_id = $1",
		activityID,
	); err != nil {
		return fmt.Errorf("surfaces: delete existing rows: %w", err)
	}

	// Match the track against osm_ways using a 10 m buffer (consistent with
	// surfaces.py BUFFER_M = 10). Aggregate by (surface_type, path_type) and
	// compute the percentage of total track length.
	// Returns 0 rows when no track_full exists for the activity (no separate
	// existence check needed).
	rows, err := db.Pool.Query(ctx, `
		WITH route AS (
			-- total_length_m uses the full track for accuracy;
			-- geom is simplified (~11 m tolerance) to speed up spatial ops.
			SELECT ST_Simplify(track_full, 0.0001) AS geom,
			       ST_Length(track_full::geography)       AS total_length_m
			FROM activity_tracks
			WHERE activity_id = $1
		),
		matched AS (
			SELECT t.surface, t.highway,
			       ST_Length(
			           ST_Intersection(
			               ST_Buffer(route.geom::geography, 10)::geometry,
			               t.geom
			           )::geography
			       ) AS match_length_m,
			       route.total_length_m
			FROM osm_ways t, route
			WHERE ST_DWithin(route.geom::geography, t.geom::geography, 10)
			  AND t.surface IS NOT NULL
		)
		SELECT surface,
		       COALESCE(highway, 'unknown') AS highway,
		       ROUND(SUM(match_length_m))::int          AS total_m,
		       ROUND((SUM(match_length_m) / NULLIF(MAX(total_length_m), 0) * 100)::numeric, 1) AS pct
		FROM matched
		GROUP BY surface, highway
		HAVING SUM(match_length_m) > 10
		ORDER BY total_m DESC
	`, activityID)
	if err != nil {
		return fmt.Errorf("surfaces: query: %w", err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var surfaceType string
		var highway string
		var totalM int
		var pct float64

		if err := rows.Scan(&surfaceType, &highway, &totalM, &pct); err != nil {
			return fmt.Errorf("surfaces: scan: %w", err)
		}

		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_surfaces
			    (activity_id, surface_type, path_type, length_m, percentage)
			VALUES ($1, $2, $3, $4, $5)
		`, activityID, surfaceType, highway, float64(totalM), pct); err != nil {
			slog.Warn("surfaces: insert failed",
				"activity_id", activityID,
				"surface_type", surfaceType,
				"error", err,
			)
		} else {
			count++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("surfaces: rows iteration: %w", err)
	}

	slog.Info("surfaces: enrichment complete",
		"activity_id", activityID,
		"surface_types", count,
	)
	return nil
}
