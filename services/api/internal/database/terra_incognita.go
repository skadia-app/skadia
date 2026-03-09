package database

import (
	"context"
	"fmt"
)

// GetTerraIncognitaCoverage computes the terra incognita area as a single GeoJSON polygon
// by buffering and unioning all heatmap_lines for a user.
// Uses ST_SnapToGrid for performance (~200m grid) before buffering.
// Returns the GeoJSON string and area in km².
// An optional bbox [west, south, east, north] limits the query to a viewport.
func (db *DB) GetTerraIncognitaCoverage(ctx context.Context, userID string, bbox *[4]float64, radiusM int) (string, float64, error) {
	var bboxFilter string
	args := []any{userID, radiusM} // $1=userID, $2=radiusM

	if bbox != nil {
		bboxFilter = `AND geom && ST_MakeEnvelope($3, $4, $5, $6, 4326)`
		args = append(args, bbox[0], bbox[1], bbox[2], bbox[3])
	}

	query := fmt.Sprintf(`
		WITH snapped AS (
			-- ST_SimplifyPreserveTopology statt ST_SnapToGrid:
			-- 0.0001° ≈ 8m Toleranz (lat 47°) — glatte Kurven statt 200m-Pixelraster.
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
			COALESCE(ST_AsGeoJSON(geom, 5), '{"type":"Polygon","coordinates":[]}'),
			COALESCE(ST_Area(geom::geography) / 1000000, 0)
		FROM merged`,
		bboxFilter)

	var geojson string
	var areaKm2 float64
	err := db.Pool.QueryRow(ctx, query, args...).Scan(&geojson, &areaKm2)
	if err != nil {
		return `{"type":"Polygon","coordinates":[]}`, 0, fmt.Errorf("exploration coverage: %w", err)
	}

	return geojson, areaKm2, nil
}
