package database

import (
	"context"
	"fmt"
)

// terraResult is the shared return type for singleflight.
type terraResult struct {
	rings   []string
	areaKm2 float64
}

// GetTerraIncognitaCoverage computes the terra incognita explored area as
// 5 concentric ring polygons (GeoJSON) for gradient fog rendering.
//
// The rings are computed by buffering the union of all heatmap_lines:
//   ring[0] = ST_Buffer(union, -R*0.3)   → innermost (clear boundary)
//   ring[1] = ST_Buffer(union, -R*0.15)
//   ring[2] = union (at radius R)        → the configured radius
//   ring[3] = ST_Buffer(union, +R*0.3)
//   ring[4] = ST_Buffer(union, +R*0.6)   → outermost fog boundary
//
// The union uses a divide-and-conquer strategy: 8 partial unions via
// ST_UnaryUnion(ST_Collect()) (O(n log n) per bucket), then one final
// merge of 8 results. This is dramatically faster than a naive ST_Union
// aggregate (O(n²)). The 4 extra ST_Buffer calls operate on the already-
// merged single polygon and are fast (~50ms each).
//
// The frontend stacks 5 fill layers with additive alpha compositing to create
// a smooth fog gradient. The configured radius sits at ~50% opacity.
func (db *DB) GetTerraIncognitaCoverage(ctx context.Context, userID string, radiusM int) ([]string, float64, error) {
	args := []any{userID, radiusM} // $1=userID, $2=radiusM

	emptyGeom := `{"type":"Polygon","coordinates":[]}`

	query := fmt.Sprintf(`
		WITH snapped AS (
			SELECT ST_SimplifyPreserveTopology(geom, 0.0001) AS geom
			FROM heatmap_lines
			WHERE user_id = $1
		),
		buffered AS (
			SELECT ST_Buffer(geom::geography, $2)::geometry AS geom,
			       ntile(8) OVER (ORDER BY ST_Length(geom) DESC) AS bucket
			FROM snapped
			WHERE geom IS NOT NULL
				AND ST_NumPoints(geom) >= 2
		),
		-- Divide-and-conquer: 8 Partial-Unions (je ~125 Geometrien),
		-- dann 1 Final-Union über 8 Ergebnisse.
		-- ST_UnaryUnion(ST_Collect()) nutzt Cascaded Union mit Spatial Index
		-- → O(n log n) statt O(n²) pro Bucket.
		partial AS (
			SELECT ST_UnaryUnion(ST_Collect(geom)) AS geom
			FROM buffered
			GROUP BY bucket
		),
		merged AS (
			SELECT ST_UnaryUnion(ST_Collect(geom)) AS geom
			FROM partial
		)
		SELECT
			COALESCE(NULLIF(ST_AsGeoJSON(ST_Buffer(geom::geography, -$2 * 0.30)::geometry, 4), 'null'), '%s'),
			COALESCE(NULLIF(ST_AsGeoJSON(ST_Buffer(geom::geography, -$2 * 0.15)::geometry, 4), 'null'), '%s'),
			COALESCE(ST_AsGeoJSON(geom, 4), '%s'),
			COALESCE(ST_AsGeoJSON(ST_Buffer(geom::geography,  $2 * 0.30)::geometry, 4), '%s'),
			COALESCE(ST_AsGeoJSON(ST_Buffer(geom::geography,  $2 * 0.60)::geometry, 4), '%s'),
			COALESCE(ST_Area(geom::geography) / 1000000, 0)
		FROM merged`,
		emptyGeom, emptyGeom, emptyGeom, emptyGeom, emptyGeom)

	var rings [5]string
	var areaKm2 float64
	err := db.Pool.QueryRow(ctx, query, args...).Scan(
		&rings[0], &rings[1], &rings[2], &rings[3], &rings[4], &areaKm2,
	)
	if err != nil {
		empty := make([]string, 5)
		for i := range empty {
			empty[i] = emptyGeom
		}
		return empty, 0, fmt.Errorf("exploration coverage: %w", err)
	}

	return rings[:], areaKm2, nil
}

// InvalidateTerraIncognitaCache deletes the cache entry for a user.
// Call this after new heatmap_lines are inserted or the radius setting changes.
func (db *DB) InvalidateTerraIncognitaCache(ctx context.Context, userID string) {
	_, _ = db.Pool.Exec(ctx, `DELETE FROM terra_incognita_cache WHERE user_id = $1`, userID)
}

// GetTerraIncognitaCoverageCached returns the terra incognita rings from cache
// when the heatmap_lines count and configured radius match. On a cache miss it
// calls GetTerraIncognitaCoverage (always the full dataset) and persists the
// result for subsequent requests.
func (db *DB) GetTerraIncognitaCoverageCached(ctx context.Context, userID string, radiusM int) ([]string, float64, error) {
	// Count current heatmap lines to detect staleness.
	var currentCount int
	_ = db.Pool.QueryRow(ctx,
		`SELECT count(*) FROM heatmap_lines WHERE user_id = $1`, userID,
	).Scan(&currentCount)

	// Try to read cached entry.
	var (
		cachedRings  []string
		cachedArea   float64
		cachedRadius int
		cachedCount  int
	)
	err := db.Pool.QueryRow(ctx, `
		SELECT rings, area_km2, radius_m, heatmap_count
		FROM terra_incognita_cache
		WHERE user_id = $1`, userID,
	).Scan(&cachedRings, &cachedArea, &cachedRadius, &cachedCount)

	// Cache hit: radius and heatmap line count both match.
	if err == nil && cachedRadius == radiusM && cachedCount == currentCount {
		return cachedRings, cachedArea, nil
	}

	// Cache miss: use singleflight to prevent concurrent computation for the same user.
	// If a computation is already in-flight for this user, all callers block and share
	// the single result — avoiding parallel 30 s PostGIS queries.
	//
	// context.Background() is used inside the lambda so that if the first caller
	// disconnects mid-way through the ~30 s PostGIS query, the computation
	// continues and the result is shared with all other waiting callers.
	// The request context is only used for the cheap cache-read above and the
	// cache-write below, not for the heavy computation.
	v, err, _ := db.terraInflight.Do(userID, func() (any, error) {
		rings, areaKm2, err := db.GetTerraIncognitaCoverage(context.Background(), userID, radiusM)
		if err != nil {
			return nil, err
		}
		// Persist result (UPSERT — one row per user).
		// Use context.Background() here too: the heavy work is done; we must
		// still write the cache even if the triggering request has gone away.
		_, _ = db.Pool.Exec(context.Background(), `
			INSERT INTO terra_incognita_cache (user_id, radius_m, rings, area_km2, heatmap_count, computed_at)
			VALUES ($1, $2, $3, $4, $5, NOW())
			ON CONFLICT (user_id) DO UPDATE SET
				radius_m      = EXCLUDED.radius_m,
				rings         = EXCLUDED.rings,
				area_km2      = EXCLUDED.area_km2,
				heatmap_count = EXCLUDED.heatmap_count,
				computed_at   = EXCLUDED.computed_at`,
			userID, radiusM, rings, areaKm2, currentCount,
		)
		return &terraResult{rings: rings, areaKm2: areaKm2}, nil
	})
	if err != nil {
		empty := make([]string, 5)
		for i := range empty {
			empty[i] = `{"type":"Polygon","coordinates":[]}`
		}
		return empty, 0, err
	}
	res := v.(*terraResult)
	return res.rings, res.areaKm2, nil
}
