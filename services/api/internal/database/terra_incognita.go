package database

import (
	"context"
	"fmt"
)

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
// Only ONE expensive ST_Union is performed; the 4 extra ST_Buffer calls
// operate on the already-merged result (single polygon) and are fast (~50ms each).
//
// The frontend stacks 5 fill layers with additive alpha compositing to create
// a smooth fog gradient. The configured radius sits at ~50% opacity.
//
// An optional bbox [west, south, east, north] limits the query to a viewport.
func (db *DB) GetTerraIncognitaCoverage(ctx context.Context, userID string, bbox *[4]float64, radiusM int) ([]string, float64, error) {
	var bboxFilter string
	args := []any{userID, radiusM} // $1=userID, $2=radiusM

	if bbox != nil {
		bboxFilter = `AND geom && ST_MakeEnvelope($3, $4, $5, $6, 4326)`
		args = append(args, bbox[0], bbox[1], bbox[2], bbox[3])
	}

	emptyGeom := `{"type":"Polygon","coordinates":[]}`

	query := fmt.Sprintf(`
		WITH snapped AS (
			SELECT ST_SimplifyPreserveTopology(geom, 0.0001) AS geom
			FROM heatmap_lines
			WHERE user_id = $1
			%s
		),
		merged AS (
			SELECT ST_Union(
				ST_Buffer(geom::geography, $2)::geometry
			) AS geom
			FROM snapped
			WHERE geom IS NOT NULL
				AND ST_NumPoints(geom) >= 2
		)
		SELECT
			COALESCE(NULLIF(ST_AsGeoJSON(ST_Buffer(geom::geography, -$2 * 0.30)::geometry, 4), 'null'), '%s'),
			COALESCE(NULLIF(ST_AsGeoJSON(ST_Buffer(geom::geography, -$2 * 0.15)::geometry, 4), 'null'), '%s'),
			COALESCE(ST_AsGeoJSON(geom, 4), '%s'),
			COALESCE(ST_AsGeoJSON(ST_Buffer(geom::geography,  $2 * 0.30)::geometry, 4), '%s'),
			COALESCE(ST_AsGeoJSON(ST_Buffer(geom::geography,  $2 * 0.60)::geometry, 4), '%s'),
			COALESCE(ST_Area(geom::geography) / 1000000, 0)
		FROM merged`,
		bboxFilter, emptyGeom, emptyGeom, emptyGeom, emptyGeom, emptyGeom)

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
