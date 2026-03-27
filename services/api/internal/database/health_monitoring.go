package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (db *DB) InsertMonitoringSamples(ctx context.Context, samples []model.MonitoringSample) error {
	if len(samples) == 0 {
		return nil
	}

	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Create temp staging table (unlogged, no constraints — plain heap table, NOT a hypertable)
	_, err = tx.Exec(ctx, `CREATE TEMP TABLE _hms_staging (LIKE health_monitoring_samples INCLUDING NOTHING) ON COMMIT DROP`)
	if err != nil {
		return fmt.Errorf("create staging table: %w", err)
	}

	// 2. CopyFrom into staging table
	columns := []string{
		"user_id", "ts", "heart_rate", "activity_type", "intensity",
		"temperature_c", "temperature_min_c", "temperature_max_c",
		"stress", "body_battery", "body_battery_delta",
		"respiration_rate", "sleep_level",
		"spo2_pct", "spo2_confidence", "wrist_temperature_c",
	}
	rows := make([][]any, len(samples))
	for i, s := range samples {
		rows[i] = []any{
			s.UserID, s.TS, s.HeartRate, s.ActivityType, s.Intensity,
			s.TemperatureC, s.TemperatureMinC, s.TemperatureMaxC,
			s.Stress, s.BodyBattery, s.BodyBatteryDelta,
			s.RespirationRate, s.SleepLevel,
			s.SpO2Pct, s.SpO2Confidence, s.WristTemperatureC,
		}
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"_hms_staging"}, columns, pgx.CopyFromRows(rows))
	if err != nil {
		return fmt.Errorf("copy to staging: %w", err)
	}

	// 3. Upsert from staging into real table — COALESCE preserves existing non-NULL values
	_, err = tx.Exec(ctx, `
		INSERT INTO health_monitoring_samples (
			user_id, ts, heart_rate, activity_type, intensity,
			temperature_c, temperature_min_c, temperature_max_c,
			stress, body_battery, body_battery_delta,
			respiration_rate, sleep_level,
			spo2_pct, spo2_confidence, wrist_temperature_c
		)
		SELECT
			user_id, ts, heart_rate, activity_type, intensity,
			temperature_c, temperature_min_c, temperature_max_c,
			stress, body_battery, body_battery_delta,
			respiration_rate, sleep_level,
			spo2_pct, spo2_confidence, wrist_temperature_c
		FROM _hms_staging
		ON CONFLICT (user_id, ts) DO UPDATE SET
			heart_rate          = COALESCE(EXCLUDED.heart_rate,          health_monitoring_samples.heart_rate),
			activity_type       = COALESCE(EXCLUDED.activity_type,       health_monitoring_samples.activity_type),
			intensity           = COALESCE(EXCLUDED.intensity,           health_monitoring_samples.intensity),
			temperature_c       = COALESCE(EXCLUDED.temperature_c,       health_monitoring_samples.temperature_c),
			temperature_min_c   = COALESCE(EXCLUDED.temperature_min_c,   health_monitoring_samples.temperature_min_c),
			temperature_max_c   = COALESCE(EXCLUDED.temperature_max_c,   health_monitoring_samples.temperature_max_c),
			stress              = COALESCE(EXCLUDED.stress,              health_monitoring_samples.stress),
			body_battery        = COALESCE(EXCLUDED.body_battery,        health_monitoring_samples.body_battery),
			body_battery_delta  = COALESCE(EXCLUDED.body_battery_delta,  health_monitoring_samples.body_battery_delta),
			respiration_rate    = COALESCE(EXCLUDED.respiration_rate,    health_monitoring_samples.respiration_rate),
			sleep_level         = COALESCE(EXCLUDED.sleep_level,         health_monitoring_samples.sleep_level),
			spo2_pct            = COALESCE(EXCLUDED.spo2_pct,            health_monitoring_samples.spo2_pct),
			spo2_confidence     = COALESCE(EXCLUDED.spo2_confidence,     health_monitoring_samples.spo2_confidence),
			wrist_temperature_c = COALESCE(EXCLUDED.wrist_temperature_c, health_monitoring_samples.wrist_temperature_c)
	`)
	if err != nil {
		return fmt.Errorf("upsert from staging: %w", err)
	}

	return tx.Commit(ctx)
}

func (db *DB) GetMonitoringSamples(ctx context.Context, userID string, from, to time.Time) ([]model.MonitoringSample, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, ts, heart_rate, activity_type, intensity,
		       temperature_c, temperature_min_c, temperature_max_c,
		       stress, body_battery, body_battery_delta,
		       respiration_rate, sleep_level,
		       spo2_pct, spo2_confidence, wrist_temperature_c
		FROM health_monitoring_samples
		WHERE user_id = $1 AND ts BETWEEN $2 AND $3
		ORDER BY ts
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query monitoring samples: %w", err)
	}
	defer rows.Close()

	var results []model.MonitoringSample
	for rows.Next() {
		var s model.MonitoringSample
		if err := rows.Scan(
			&s.UserID, &s.TS, &s.HeartRate, &s.ActivityType, &s.Intensity,
			&s.TemperatureC, &s.TemperatureMinC, &s.TemperatureMaxC,
			&s.Stress, &s.BodyBattery, &s.BodyBatteryDelta,
			&s.RespirationRate, &s.SleepLevel,
			&s.SpO2Pct, &s.SpO2Confidence, &s.WristTemperatureC,
		); err != nil {
			return nil, fmt.Errorf("scan monitoring sample: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetMonitoringSamplesBucketed(ctx context.Context, userID string, from, to time.Time, bucket string) ([]model.MonitoringSample, error) {
	interval := bucketToInterval(bucket)
	rows, err := db.Pool.Query(ctx, `
		SELECT $1::UUID AS user_id,
		       time_bucket($4::INTERVAL, ts) AS ts,
		       avg(heart_rate)::SMALLINT, NULL::SMALLINT, NULL::SMALLINT,
		       avg(temperature_c)::REAL, min(temperature_min_c)::REAL, max(temperature_max_c)::REAL,
		       avg(stress)::SMALLINT, min(body_battery)::SMALLINT, NULL::SMALLINT,
		       avg(respiration_rate)::REAL, NULL::SMALLINT,
		       avg(spo2_pct)::SMALLINT, NULL::SMALLINT, avg(wrist_temperature_c)::REAL
		FROM health_monitoring_samples
		WHERE user_id = $1 AND ts BETWEEN $2 AND $3
		GROUP BY time_bucket($4::INTERVAL, ts)
		ORDER BY ts
	`, userID, from, to, interval)
	if err != nil {
		return nil, fmt.Errorf("query bucketed monitoring: %w", err)
	}
	defer rows.Close()

	var results []model.MonitoringSample
	for rows.Next() {
		var s model.MonitoringSample
		if err := rows.Scan(
			&s.UserID, &s.TS, &s.HeartRate, &s.ActivityType, &s.Intensity,
			&s.TemperatureC, &s.TemperatureMinC, &s.TemperatureMaxC,
			&s.Stress, &s.BodyBattery, &s.BodyBatteryDelta,
			&s.RespirationRate, &s.SleepLevel,
			&s.SpO2Pct, &s.SpO2Confidence, &s.WristTemperatureC,
		); err != nil {
			return nil, fmt.Errorf("scan bucketed sample: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetMonitoringDaily(ctx context.Context, userID string, from, to time.Time) ([]model.MonitoringSample, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, day,
		       heart_rate_avg::SMALLINT, NULL::SMALLINT, NULL::SMALLINT,
		       NULL::REAL, NULL::REAL, NULL::REAL,
		       stress_avg::SMALLINT, body_battery_min, NULL::SMALLINT,
		       respiration_rate_avg, NULL::SMALLINT,
		       spo2_avg::SMALLINT, NULL::SMALLINT, NULL::REAL
		FROM health_monitoring_daily
		WHERE user_id = $1 AND day BETWEEN $2 AND $3
		ORDER BY day
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query monitoring daily: %w", err)
	}
	defer rows.Close()

	var results []model.MonitoringSample
	for rows.Next() {
		var s model.MonitoringSample
		if err := rows.Scan(
			&s.UserID, &s.TS, &s.HeartRate, &s.ActivityType, &s.Intensity,
			&s.TemperatureC, &s.TemperatureMinC, &s.TemperatureMaxC,
			&s.Stress, &s.BodyBattery, &s.BodyBatteryDelta,
			&s.RespirationRate, &s.SleepLevel,
			&s.SpO2Pct, &s.SpO2Confidence, &s.WristTemperatureC,
		); err != nil {
			return nil, fmt.Errorf("scan monitoring daily: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func bucketToInterval(bucket string) string {
	switch bucket {
	case "1m":
		return "1 minute"
	case "15m":
		return "15 minutes"
	case "1h":
		return "1 hour"
	case "1d":
		return "1 day"
	default:
		return "15 minutes"
	}
}
