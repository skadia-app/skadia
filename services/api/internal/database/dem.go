package database

import (
	"context"
	"fmt"

	"github.com/skadia-app/skadia/services/api/internal/model"
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

	// 2. Query DEM elevation per GPS point via PostGIS and apply correction.
	source = "none"
	corrected = 0
	for i := range streams {
		if streams[i].Lat == nil || streams[i].Lng == nil {
			continue
		}
		var demElev float64
		var src string
		qErr := db.Pool.QueryRow(ctx, `
			SELECT dem.get_elevation(ST_SetSRID(ST_MakePoint($1, $2), 4326)),
			       COALESCE((
			           SELECT source FROM dem.raster_tiles
			           WHERE ST_Intersects(rast, ST_SetSRID(ST_MakePoint($1, $2), 4326))
			           ORDER BY resolution_m ASC LIMIT 1
			       ), 'none')
		`, *streams[i].Lng, *streams[i].Lat).Scan(&demElev, &src)
		if qErr != nil {
			// DEM tile missing for this point — keep current altitude.
			continue
		}

		// Reject correction when deviation exceeds threshold (barometer may be more precise).
		if streams[i].AltitudeM != nil {
			diff := demElev - *streams[i].AltitudeM
			if diff < 0 {
				diff = -diff
			}
			if diff > demCorrectionThresholdM {
				continue
			}
		}

		v := demElev
		streams[i].AltitudeM = &v
		source = src
		corrected++
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

