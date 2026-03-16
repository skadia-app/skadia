package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// InsertWeatherReadings replaces all weather readings for an activity atomically.
func (db *DB) InsertWeatherReadings(ctx context.Context, activityID string, readings []model.WeatherReading) error {
	if len(readings) == 0 {
		return nil
	}
	return pgx.BeginFunc(ctx, db.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`DELETE FROM activity_weather WHERE activity_id = $1`, activityID,
		); err != nil {
			return fmt.Errorf("delete existing weather readings: %w", err)
		}

		batch := &pgx.Batch{}
		for _, r := range readings {
			batch.Queue(`
				INSERT INTO activity_weather (
					activity_id, reading_type, label, recorded_at, lat, lng, elevation_m,
					temp_c, apparent_temp_c, precipitation_mm,
					wind_speed_ms, wind_gusts_ms, humidity,
					uv_index, freezing_level_m, condition, tags, model
				) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
				activityID, r.ReadingType, r.Label, r.RecordedAt, r.Lat, r.Lng, r.ElevationM,
				r.TempC, r.ApparentTempC, r.PrecipitationMM,
				r.WindSpeedMS, r.WindGustsMS, r.Humidity,
				r.UVIndex, r.FreezingLevelM, r.Condition, r.Tags, r.Model,
			)
		}
		br := tx.SendBatch(ctx, batch)
		defer br.Close()
		for range readings {
			if _, err := br.Exec(); err != nil {
				return fmt.Errorf("insert weather reading: %w", err)
			}
		}
		return nil
	})
}

// GetWeatherReadings returns all weather readings for an activity, ordered by creation time.
func (db *DB) GetWeatherReadings(ctx context.Context, activityID string) ([]model.WeatherReading, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT id, reading_type, label, recorded_at, lat, lng, elevation_m,
		       temp_c, apparent_temp_c, precipitation_mm,
		       wind_speed_ms, wind_gusts_ms, humidity,
		       uv_index, freezing_level_m, condition, tags, model
		FROM activity_weather
		WHERE activity_id = $1
		ORDER BY created_at`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get weather readings: %w", err)
	}
	defer rows.Close()

	var readings []model.WeatherReading
	for rows.Next() {
		var r model.WeatherReading
		if err := rows.Scan(
			&r.ID, &r.ReadingType, &r.Label, &r.RecordedAt, &r.Lat, &r.Lng, &r.ElevationM,
			&r.TempC, &r.ApparentTempC, &r.PrecipitationMM,
			&r.WindSpeedMS, &r.WindGustsMS, &r.Humidity,
			&r.UVIndex, &r.FreezingLevelM, &r.Condition, &r.Tags, &r.Model,
		); err != nil {
			return nil, fmt.Errorf("scan weather reading: %w", err)
		}
		readings = append(readings, r)
	}
	return readings, rows.Err()
}

// ActivityWeatherCandidate holds data needed for weather backfill queries.
type ActivityWeatherCandidate struct {
	ID            string
	StartedAt     time.Time
	StartLat      float64
	StartLng      float64
	MinElevationM *float64
	MaxElevationM *float64
	ElapsedTimeS  *int
	IsIndoor      bool
	HasGPS        bool
}

// GetActivitiesForWeatherBackfill returns activities without weather readings yet.
func (db *DB) GetActivitiesForWeatherBackfill(ctx context.Context, userID string) ([]ActivityWeatherCandidate, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT a.id, a.started_at,
		       ST_Y(a.start_location::geometry) AS start_lat,
		       ST_X(a.start_location::geometry) AS start_lng,
		       a.min_elevation_m, a.max_elevation_m, a.elapsed_time_s,
		       a.is_indoor, a.has_gps
		FROM activities a
		WHERE a.user_id = $1
		  AND a.start_location IS NOT NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM activity_weather aw WHERE aw.activity_id = a.id
		  )
		ORDER BY a.started_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get activities for weather backfill: %w", err)
	}
	defer rows.Close()

	var result []ActivityWeatherCandidate
	for rows.Next() {
		var c ActivityWeatherCandidate
		if err := rows.Scan(&c.ID, &c.StartedAt, &c.StartLat, &c.StartLng,
			&c.MinElevationM, &c.MaxElevationM, &c.ElapsedTimeS,
			&c.IsIndoor, &c.HasGPS); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetWeatherPrefs returns the weather preferences for a user.
func (db *DB) GetWeatherPrefs(ctx context.Context, userID string) (model.WeatherPrefs, error) {
	var p model.WeatherPrefs
	err := db.Pool.QueryRow(ctx, `
		SELECT weather_reading_mode, weather_mountain_threshold_m,
		       weather_long_duration_s, weather_display_fields
		FROM users WHERE id = $1`, userID,
	).Scan(&p.ReadingMode, &p.MountainThresholdM, &p.LongDurationS, &p.DisplayFields)
	if err != nil {
		return model.DefaultWeatherPrefs(), fmt.Errorf("get weather prefs: %w", err)
	}
	if p.MountainThresholdM == 0 {
		p.MountainThresholdM = 500
	}
	if p.LongDurationS == 0 {
		p.LongDurationS = 14400
	}
	if len(p.DisplayFields) == 0 {
		p.DisplayFields = model.DefaultWeatherPrefs().DisplayFields
	}
	return p, nil
}

// UpdateWeatherPrefs updates the user's weather preference columns.
func (db *DB) UpdateWeatherPrefs(ctx context.Context, userID string, p model.WeatherPrefs) error {
	_, err := db.Pool.Exec(ctx, `
		UPDATE users
		SET weather_reading_mode         = $2,
		    weather_mountain_threshold_m = $3,
		    weather_long_duration_s      = $4,
		    weather_display_fields       = $5
		WHERE id = $1`,
		userID, p.ReadingMode, p.MountainThresholdM, p.LongDurationS, p.DisplayFields,
	)
	return err
}
