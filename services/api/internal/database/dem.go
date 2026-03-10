package database

import (
	"context"
	"fmt"
)

// ElevationPoint represents a single point in an elevation profile.
type ElevationPoint struct {
	Fraction  float64 `json:"fraction"`
	Elevation float64 `json:"elevation"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
}

// GetElevation queries DEM elevation for a list of [lng, lat] points using the
// dem.get_elevation() PostgreSQL function from Migration 000130.
// Returns the elevations in the same order as the input points, along with the
// data source identifier of the best-resolution raster tile at the last queried
// point.
func (db *DB) GetElevation(ctx context.Context, points [][2]float64) ([]float64, string, error) {
	elevations := make([]float64, len(points))
	source := "none"

	for i, p := range points {
		var elev float64
		var src string
		err := db.Pool.QueryRow(ctx, `
			SELECT dem.get_elevation(ST_SetSRID(ST_MakePoint($1, $2), 4326)),
			       COALESCE((
			           SELECT source FROM dem.raster_tiles
			           WHERE ST_Intersects(rast, ST_SetSRID(ST_MakePoint($1, $2), 4326))
			           ORDER BY resolution_m ASC LIMIT 1
			       ), 'none')
		`, p[0], p[1]).Scan(&elev, &src)
		if err != nil {
			// Gracefully skip points where DEM data is unavailable.
			continue
		}
		elevations[i] = elev
		source = src
	}
	return elevations, source, nil
}

// GetElevationProfile samples elevation along a GeoJSON geometry using the
// dem.get_elevation_profile() set-returning PostgreSQL function from Migration 000130.
// geojson must be a valid GeoJSON geometry string (LineString / MultiLineString).
// numPoints controls the sampling density (default 100 if <= 0).
func (db *DB) GetElevationProfile(ctx context.Context, geojson string, numPoints int) ([]ElevationPoint, error) {
	if numPoints <= 0 {
		numPoints = 100
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT fraction, elevation, ST_X(point), ST_Y(point)
		FROM dem.get_elevation_profile(ST_GeomFromGeoJSON($1), $2)
	`, geojson, numPoints)
	if err != nil {
		return nil, fmt.Errorf("get elevation profile: %w", err)
	}
	defer rows.Close()

	var profile []ElevationPoint
	for rows.Next() {
		var p ElevationPoint
		if err := rows.Scan(&p.Fraction, &p.Elevation, &p.Lng, &p.Lat); err != nil {
			return nil, fmt.Errorf("scan elevation profile row: %w", err)
		}
		profile = append(profile, p)
	}
	// E15: always check rows.Err() after iteration.
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate elevation profile rows: %w", err)
	}
	return profile, nil
}
