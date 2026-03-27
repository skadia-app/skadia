package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ElevationPoint represents a single point in an elevation profile.
type ElevationPoint struct {
	Fraction  float64  `json:"fraction"`
	Elevation *float64 `json:"elevation"` // nil when no DEM coverage
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
}

// GetElevation queries DEM elevation for a list of [lng, lat] points using the
// dem.get_elevation() PostgreSQL function from Migration 000130.
// Returns the elevations in the same order as the input points, along with the
// data source identifier of the best-resolution DEM raster tile.
// Uses a single batch query via unnest to avoid N+1 roundtrips.
func (db *DB) GetElevation(ctx context.Context, points [][2]float64) ([]float64, string, error) {
	if len(points) == 0 {
		return nil, "none", nil
	}

	// Detect best available DEM source with a single query.
	var source string
	err := db.Pool.QueryRow(ctx, `
		SELECT CASE
			WHEN EXISTS (SELECT 1 FROM dem.raster_tiles WHERE source LIKE 'lidar_%') THEN 'lidar'
			WHEN EXISTS (SELECT 1 FROM dem.raster_tiles WHERE source = 'eea10') THEN 'eea10'
			WHEN EXISTS (SELECT 1 FROM dem.raster_tiles LIMIT 1) THEN
				(SELECT source FROM dem.raster_tiles ORDER BY resolution_m ASC LIMIT 1)
			ELSE 'none'
		END
	`).Scan(&source)
	if err != nil || source == "none" {
		return make([]float64, len(points)), "none", err
	}

	// Build parallel arrays for the batch query. Input points are [lng, lat].
	lngs := make([]float64, len(points))
	lats := make([]float64, len(points))
	for i, p := range points {
		lngs[i] = p[0]
		lats[i] = p[1]
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT COALESCE(dem.get_elevation(ST_SetSRID(ST_MakePoint(lng, lat), 4326)), 0)
		FROM unnest($1::float8[], $2::float8[]) WITH ORDINALITY AS t(lng, lat, ord)
		ORDER BY ord
	`, lngs, lats)
	if err != nil {
		return nil, source, fmt.Errorf("batch elevation query: %w", err)
	}
	defer rows.Close()

	elevations := make([]float64, 0, len(points))
	for rows.Next() {
		var elev float64
		if err := rows.Scan(&elev); err != nil {
			return nil, source, fmt.Errorf("scan elevation: %w", err)
		}
		elevations = append(elevations, elev)
	}
	if err := rows.Err(); err != nil {
		return nil, source, fmt.Errorf("batch elevation rows: %w", err)
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

// demCorrectionThresholdM is the maximum allowed deviation between device altitude
// and DEM value before the DEM value is rejected (device barometry is more precise
// for large local pressure deviations).
const demCorrectionThresholdM = 50.0

// getAllStreamPoints loads all columns of activity_streams for an activity as
// model.StreamPoint slices, including running-dynamics fields. This is the full
// equivalent of InsertStreams and is used internally for in-place rewrite operations
// (e.g. elevation correction) that must preserve all stream data.
func (db *DB) getAllStreamPoints(ctx context.Context, activityID string) ([]model.StreamPoint, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT time, lat, lng, altitude_m, speed_ms, distance_m,
		       heart_rate, power_w, cadence, temperature_c,
		       ground_contact_time_ms, vertical_oscillation_mm, stance_time_percent, left_right_balance
		FROM activity_streams
		WHERE activity_id = $1
		ORDER BY time ASC
	`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get all stream points: %w", err)
	}
	defer rows.Close()

	var streams []model.StreamPoint
	for rows.Next() {
		var s model.StreamPoint
		if err := rows.Scan(
			&s.Time, &s.Lat, &s.Lng, &s.AltitudeM, &s.SpeedMS, &s.DistanceM,
			&s.HeartRate, &s.PowerW, &s.Cadence, &s.TemperatureC,
			&s.GroundContactTimeMs, &s.VerticalOscillationMm, &s.StanceTimePercent, &s.LeftRightBalance,
		); err != nil {
			return nil, fmt.Errorf("scan full stream point: %w", err)
		}
		streams = append(streams, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate full stream points: %w", err)
	}
	return streams, nil
}

// CorrectActivityElevationFromDEM queries PostGIS DEM elevations for all GPS-bearing
// stream points of an activity and applies corrections in-memory, then rewrites the
// stream (DELETE + INSERT) to avoid issues with TimescaleDB compressed chunks.
//
// Returns the number of stream points whose altitude was corrected and the DEM source
// identifier of the last successfully queried raster tile.
func (db *DB) CorrectActivityElevationFromDEM(ctx context.Context, activityID string) (corrected int, source string, err error) {
	// 1. Load all stream points for the activity (all columns preserved for re-insert).
	streams, err := db.getAllStreamPoints(ctx, activityID)
	if err != nil {
		return 0, "none", fmt.Errorf("load streams for DEM correction: %w", err)
	}
	if len(streams) == 0 {
		return 0, "none", nil
	}

	// 2. Collect GPS-bearing points and their indices for a single batch DEM query.
	source = "none"
	corrected = 0

	type gpsIdx struct {
		streamIdx int
		lat, lng  float64
	}
	gpsPoints := make([]gpsIdx, 0, len(streams))
	for i := range streams {
		if streams[i].Lat == nil || streams[i].Lng == nil {
			continue
		}
		gpsPoints = append(gpsPoints, gpsIdx{streamIdx: i, lat: *streams[i].Lat, lng: *streams[i].Lng})
	}
	if len(gpsPoints) == 0 {
		return 0, "none", nil
	}

	// Build parallel arrays for unnest batch query.
	lats := make([]float64, len(gpsPoints))
	lngs := make([]float64, len(gpsPoints))
	for i, p := range gpsPoints {
		lats[i] = p.lat
		lngs[i] = p.lng
	}

	rows, err := db.Pool.Query(ctx, `
		SELECT
			COALESCE(dem.get_elevation(ST_SetSRID(ST_MakePoint(lng, lat), 4326)), -9999),
			COALESCE((
				SELECT source FROM dem.raster_tiles
				WHERE ST_Intersects(rast, ST_SetSRID(ST_MakePoint(lng, lat), 4326))
				ORDER BY resolution_m ASC LIMIT 1
			), 'none')
		FROM unnest($1::float8[], $2::float8[]) WITH ORDINALITY AS t(lng, lat, ord)
		ORDER BY ord
	`, lngs, lats)
	if err != nil {
		return 0, "none", fmt.Errorf("batch DEM correction query: %w", err)
	}
	defer rows.Close()

	idx := 0
	for rows.Next() {
		var demElev float64
		var src string
		if err := rows.Scan(&demElev, &src); err != nil {
			return 0, "none", fmt.Errorf("scan DEM correction row: %w", err)
		}
		si := gpsPoints[idx].streamIdx
		idx++

		// Skip points with no DEM coverage (sentinel value).
		if demElev == -9999 || src == "none" {
			continue
		}

		// Reject correction when deviation exceeds threshold (barometer may be more precise).
		if streams[si].AltitudeM != nil {
			diff := demElev - *streams[si].AltitudeM
			if diff < 0 {
				diff = -diff
			}
			if diff > demCorrectionThresholdM {
				continue
			}
		}

		v := demElev
		streams[si].AltitudeM = &v
		source = src
		corrected++
	}
	if err := rows.Err(); err != nil {
		return 0, "none", fmt.Errorf("batch DEM correction rows: %w", err)
	}

	if corrected == 0 {
		return 0, source, nil
	}

	// 3. Rewrite stream: DELETE existing rows then re-INSERT with corrected altitudes.
	// This pattern is consistent with recompute.go and avoids DML issues on
	// TimescaleDB compressed chunks.
	if err := db.DeleteStreams(ctx, activityID); err != nil {
		return 0, source, fmt.Errorf("delete streams before elevation rewrite: %w", err)
	}
	if err := db.InsertStreams(ctx, activityID, streams); err != nil {
		return 0, source, fmt.Errorf("insert streams after elevation correction: %w", err)
	}

	return corrected, source, nil
}
