package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// --- health_daily_summary ---

func (db *DB) UpsertHealthDailySummary(ctx context.Context, s *model.HealthDailySummary) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_daily_summary (
			user_id, calendar_date, total_steps, step_goal, total_distance_m,
			active_seconds, highly_active_seconds, resting_heart_rate, min_heart_rate, max_heart_rate,
			total_calories, active_calories, bmr_calories, stress_avg, body_battery_min, body_battery_max,
			hydration_ml, hydration_goal_ml, source_type, extra_data
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20
		)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			total_steps            = COALESCE(EXCLUDED.total_steps,            health_daily_summary.total_steps),
			step_goal              = COALESCE(EXCLUDED.step_goal,              health_daily_summary.step_goal),
			total_distance_m       = COALESCE(EXCLUDED.total_distance_m,       health_daily_summary.total_distance_m),
			active_seconds         = COALESCE(EXCLUDED.active_seconds,         health_daily_summary.active_seconds),
			highly_active_seconds  = COALESCE(EXCLUDED.highly_active_seconds,  health_daily_summary.highly_active_seconds),
			resting_heart_rate     = COALESCE(EXCLUDED.resting_heart_rate,     health_daily_summary.resting_heart_rate),
			min_heart_rate         = COALESCE(EXCLUDED.min_heart_rate,         health_daily_summary.min_heart_rate),
			max_heart_rate         = COALESCE(EXCLUDED.max_heart_rate,         health_daily_summary.max_heart_rate),
			total_calories         = COALESCE(EXCLUDED.total_calories,         health_daily_summary.total_calories),
			active_calories        = COALESCE(EXCLUDED.active_calories,        health_daily_summary.active_calories),
			bmr_calories           = COALESCE(EXCLUDED.bmr_calories,           health_daily_summary.bmr_calories),
			stress_avg             = COALESCE(EXCLUDED.stress_avg,             health_daily_summary.stress_avg),
			body_battery_min       = COALESCE(EXCLUDED.body_battery_min,       health_daily_summary.body_battery_min),
			body_battery_max       = COALESCE(EXCLUDED.body_battery_max,       health_daily_summary.body_battery_max),
			hydration_ml           = COALESCE(EXCLUDED.hydration_ml,           health_daily_summary.hydration_ml),
			hydration_goal_ml      = COALESCE(EXCLUDED.hydration_goal_ml,      health_daily_summary.hydration_goal_ml),
			source_type            = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_daily_summary.source_type)
						THEN EXCLUDED.source_type
						ELSE health_daily_summary.source_type
					END
	`, s.UserID, s.CalendarDate, s.TotalSteps, s.StepGoal, s.TotalDistanceM,
		s.ActiveSeconds, s.HighlyActiveSeconds, s.RestingHeartRate, s.MinHeartRate, s.MaxHeartRate,
		s.TotalCalories, s.ActiveCalories, s.BmrCalories, s.StressAvg, s.BodyBatteryMin, s.BodyBatteryMax,
		s.HydrationML, s.HydrationGoalML, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthDailySummaries(ctx context.Context, userID, from, to string) ([]model.HealthDailySummary, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, calendar_date, total_steps, step_goal, total_distance_m,
		       active_seconds, highly_active_seconds, resting_heart_rate, min_heart_rate, max_heart_rate,
		       total_calories, active_calories, bmr_calories, stress_avg, body_battery_min, body_battery_max,
		       hydration_ml, hydration_goal_ml, source_type, extra_data, created_at
		FROM health_daily_summary
		WHERE user_id = $1 AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health daily summaries: %w", err)
	}
	defer rows.Close()

	var results []model.HealthDailySummary
	for rows.Next() {
		var s model.HealthDailySummary
		var calDate time.Time
		if err := rows.Scan(
			&s.UserID, &calDate, &s.TotalSteps, &s.StepGoal, &s.TotalDistanceM,
			&s.ActiveSeconds, &s.HighlyActiveSeconds, &s.RestingHeartRate, &s.MinHeartRate, &s.MaxHeartRate,
			&s.TotalCalories, &s.ActiveCalories, &s.BmrCalories, &s.StressAvg, &s.BodyBatteryMin, &s.BodyBatteryMax,
			&s.HydrationML, &s.HydrationGoalML, &s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health daily summary: %w", err)
		}
		s.CalendarDate = calDate.Format("2006-01-02")
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthDailySummary(ctx context.Context, userID string) (*model.HealthDailySummary, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, calendar_date, total_steps, step_goal, total_distance_m,
		       active_seconds, highly_active_seconds, resting_heart_rate, min_heart_rate, max_heart_rate,
		       total_calories, active_calories, bmr_calories, stress_avg, body_battery_min, body_battery_max,
		       hydration_ml, hydration_goal_ml, source_type, extra_data, created_at
		FROM health_daily_summary
		WHERE user_id = $1
		ORDER BY calendar_date DESC
		LIMIT 1
	`, userID)
	var s model.HealthDailySummary
	var calDate time.Time
	err := row.Scan(
		&s.UserID, &calDate, &s.TotalSteps, &s.StepGoal, &s.TotalDistanceM,
		&s.ActiveSeconds, &s.HighlyActiveSeconds, &s.RestingHeartRate, &s.MinHeartRate, &s.MaxHeartRate,
		&s.TotalCalories, &s.ActiveCalories, &s.BmrCalories, &s.StressAvg, &s.BodyBatteryMin, &s.BodyBatteryMax,
		&s.HydrationML, &s.HydrationGoalML, &s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health daily summary: %w", err)
	}
	s.CalendarDate = calDate.Format("2006-01-02")
	return &s, nil
}

// --- health_sleep ---

func (db *DB) UpsertHealthSleep(ctx context.Context, s *model.HealthSleep) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_sleep (
			user_id, calendar_date, sleep_start_utc, sleep_end_utc,
			deep_seconds, light_seconds, rem_seconds, awake_seconds,
			score_overall, score_quality, score_duration, score_deep, score_rem,
			respiration_avg, awake_count, restless_moments, avg_sleep_stress,
			source_type, extra_data
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19
		)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			sleep_start_utc   = EXCLUDED.sleep_start_utc,
			sleep_end_utc     = EXCLUDED.sleep_end_utc,
			deep_seconds      = COALESCE(EXCLUDED.deep_seconds,      health_sleep.deep_seconds),
			light_seconds     = COALESCE(EXCLUDED.light_seconds,     health_sleep.light_seconds),
			rem_seconds       = COALESCE(EXCLUDED.rem_seconds,       health_sleep.rem_seconds),
			awake_seconds     = COALESCE(EXCLUDED.awake_seconds,     health_sleep.awake_seconds),
			score_overall     = COALESCE(EXCLUDED.score_overall,     health_sleep.score_overall),
			score_quality     = COALESCE(EXCLUDED.score_quality,     health_sleep.score_quality),
			score_duration    = COALESCE(EXCLUDED.score_duration,    health_sleep.score_duration),
			score_deep        = COALESCE(EXCLUDED.score_deep,        health_sleep.score_deep),
			score_rem         = COALESCE(EXCLUDED.score_rem,         health_sleep.score_rem),
			respiration_avg   = COALESCE(EXCLUDED.respiration_avg,   health_sleep.respiration_avg),
			awake_count       = COALESCE(EXCLUDED.awake_count,       health_sleep.awake_count),
			restless_moments  = COALESCE(EXCLUDED.restless_moments,  health_sleep.restless_moments),
			avg_sleep_stress  = COALESCE(EXCLUDED.avg_sleep_stress,  health_sleep.avg_sleep_stress),
			source_type       = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_sleep.source_type)
						THEN EXCLUDED.source_type
						ELSE health_sleep.source_type
					END
	`, s.UserID, s.CalendarDate, s.SleepStartUTC, s.SleepEndUTC,
		s.DeepSeconds, s.LightSeconds, s.RemSeconds, s.AwakeSeconds,
		s.ScoreOverall, s.ScoreQuality, s.ScoreDuration, s.ScoreDeep, s.ScoreRem,
		s.RespirationAvg, s.AwakeCount, s.RestlessMoments, s.AvgSleepStress,
		s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthSleep(ctx context.Context, userID, from, to string) ([]model.HealthSleep, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, calendar_date, sleep_start_utc, sleep_end_utc,
		       deep_seconds, light_seconds, rem_seconds, awake_seconds,
		       score_overall, score_quality, score_duration, score_deep, score_rem,
		       respiration_avg, awake_count, restless_moments, avg_sleep_stress,
		       source_type, extra_data, created_at
		FROM health_sleep
		WHERE user_id = $1 AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health sleep: %w", err)
	}
	defer rows.Close()

	var results []model.HealthSleep
	for rows.Next() {
		var s model.HealthSleep
		var calDate time.Time
		if err := rows.Scan(
			&s.UserID, &calDate, &s.SleepStartUTC, &s.SleepEndUTC,
			&s.DeepSeconds, &s.LightSeconds, &s.RemSeconds, &s.AwakeSeconds,
			&s.ScoreOverall, &s.ScoreQuality, &s.ScoreDuration, &s.ScoreDeep, &s.ScoreRem,
			&s.RespirationAvg, &s.AwakeCount, &s.RestlessMoments, &s.AvgSleepStress,
			&s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health sleep: %w", err)
		}
		s.CalendarDate = calDate.Format("2006-01-02")
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthSleep(ctx context.Context, userID string) (*model.HealthSleep, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, calendar_date, sleep_start_utc, sleep_end_utc,
		       deep_seconds, light_seconds, rem_seconds, awake_seconds,
		       score_overall, score_quality, score_duration, score_deep, score_rem,
		       respiration_avg, awake_count, restless_moments, avg_sleep_stress,
		       source_type, extra_data, created_at
		FROM health_sleep
		WHERE user_id = $1
		ORDER BY calendar_date DESC
		LIMIT 1
	`, userID)
	var s model.HealthSleep
	var calDate time.Time
	err := row.Scan(
		&s.UserID, &calDate, &s.SleepStartUTC, &s.SleepEndUTC,
		&s.DeepSeconds, &s.LightSeconds, &s.RemSeconds, &s.AwakeSeconds,
		&s.ScoreOverall, &s.ScoreQuality, &s.ScoreDuration, &s.ScoreDeep, &s.ScoreRem,
		&s.RespirationAvg, &s.AwakeCount, &s.RestlessMoments, &s.AvgSleepStress,
		&s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health sleep: %w", err)
	}
	s.CalendarDate = calDate.Format("2006-01-02")
	return &s, nil
}

// --- health_snapshots ---

func (db *DB) UpsertHealthHRVSnapshot(ctx context.Context, s *model.HealthHRVSnapshot) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_snapshots (
			user_id, measured_at_utc, rmssd_ms, sdrr_ms,
			avg_heart_rate_bpm, spo2_pct, respiration_brpm, stress_level, title,
			time_of_day, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (user_id, measured_at_utc) DO UPDATE SET
			rmssd_ms           = COALESCE(EXCLUDED.rmssd_ms,           health_snapshots.rmssd_ms),
			sdrr_ms            = COALESCE(EXCLUDED.sdrr_ms,            health_snapshots.sdrr_ms),
			avg_heart_rate_bpm = COALESCE(EXCLUDED.avg_heart_rate_bpm, health_snapshots.avg_heart_rate_bpm),
			spo2_pct           = COALESCE(EXCLUDED.spo2_pct,           health_snapshots.spo2_pct),
			respiration_brpm   = COALESCE(EXCLUDED.respiration_brpm,   health_snapshots.respiration_brpm),
			stress_level       = COALESCE(EXCLUDED.stress_level,       health_snapshots.stress_level),
			title              = COALESCE(EXCLUDED.title,              health_snapshots.title),
			time_of_day        = COALESCE(EXCLUDED.time_of_day,        health_snapshots.time_of_day),
			source_type        = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_snapshots.source_type)
						THEN EXCLUDED.source_type
						ELSE health_snapshots.source_type
					END
	`, s.UserID, s.MeasuredAtUTC, s.RmssdMS, s.SdrrMS,
		s.AvgHeartRateBPM, s.SpO2Pct, s.RespirationBRPM, s.StressLevel, s.Title,
		s.TimeOfDay, s.SourceType, s.ExtraData)
	return err
}

// GetHealthHRV returns HRV snapshots for the given time range.
//
// Data source strategy:
//  1. health_snapshots — real per-measurement snapshots (Garmin HRV Status / wellness
//     activities). Sparse: only exists when the watch performed an HRV measurement.
//  2. health_readiness_scores.hrv_weekly_avg_ms — daily rolling-7-day average imported
//     from TrainingReadinessDTO. Dense (one row per day). Used as a synthetic daily
//     snapshot when no real snapshot exists for that calendar day.
//
// The UNION ensures the frontend HRV chart always has data even when explicit HRV Status
// measurements are absent (which is the common case for most Garmin devices).
func (db *DB) GetHealthHRV(ctx context.Context, userID, from, to string) ([]model.HealthHRVSnapshot, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, measured_at_utc, rmssd_ms, sdrr_ms,
		       avg_heart_rate_bpm, spo2_pct, respiration_brpm, stress_level, title,
		       time_of_day, source_type, extra_data, created_at
		FROM health_snapshots
		WHERE user_id = $1 AND measured_at_utc BETWEEN $2::timestamptz AND $3::timestamptz

		UNION ALL

		-- Fill in days without a real snapshot from the daily readiness rolling average.
		SELECT
			r.user_id,
			r.calendar_date::timestamptz  AS measured_at_utc,
			r.hrv_weekly_avg_ms           AS rmssd_ms,
			NULL::double precision         AS sdrr_ms,
			NULL::smallint                 AS avg_heart_rate_bpm,
			NULL::smallint                 AS spo2_pct,
			NULL::smallint                 AS respiration_brpm,
			NULL::smallint                 AS stress_level,
			NULL::text                     AS title,
			'daily_avg'                    AS time_of_day,
			r.source_type,
			NULL::text                     AS extra_data,
			r.created_at
		FROM health_readiness_scores r
		WHERE r.user_id = $1
		  AND r.hrv_weekly_avg_ms IS NOT NULL
		  AND r.calendar_date BETWEEN $2::date AND $3::date
		  -- Exclude dates already covered by a real snapshot.
		  AND NOT EXISTS (
			SELECT 1 FROM health_snapshots h
			WHERE h.user_id = r.user_id
			  AND h.measured_at_utc::date = r.calendar_date
		  )

		ORDER BY measured_at_utc ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health hrv: %w", err)
	}
	defer rows.Close()

	var results []model.HealthHRVSnapshot
	for rows.Next() {
		var s model.HealthHRVSnapshot
		if err := rows.Scan(
			&s.UserID, &s.MeasuredAtUTC, &s.RmssdMS, &s.SdrrMS,
			&s.AvgHeartRateBPM, &s.SpO2Pct, &s.RespirationBRPM, &s.StressLevel, &s.Title,
			&s.TimeOfDay, &s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health hrv: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthHRV(ctx context.Context, userID string) (*model.HealthHRVSnapshot, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, measured_at_utc, rmssd_ms, sdrr_ms,
		       avg_heart_rate_bpm, spo2_pct, respiration_brpm, stress_level, title,
		       time_of_day, source_type, extra_data, created_at
		FROM health_snapshots
		WHERE user_id = $1
		ORDER BY measured_at_utc DESC
		LIMIT 1
	`, userID)
	var s model.HealthHRVSnapshot
	err := row.Scan(&s.UserID, &s.MeasuredAtUTC, &s.RmssdMS, &s.SdrrMS,
		&s.AvgHeartRateBPM, &s.SpO2Pct, &s.RespirationBRPM, &s.StressLevel, &s.Title,
		&s.TimeOfDay, &s.SourceType, &s.ExtraData, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health hrv: %w", err)
	}
	return &s, nil
}

// --- health_weight_entries ---

func (db *DB) UpsertHealthWeightEntry(ctx context.Context, s *model.HealthWeightEntry) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_weight_entries (
			user_id, measured_at_utc, weight_g, bmi, body_fat_pct, muscle_mass_g, bone_mass_g,
			source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_id, measured_at_utc) DO UPDATE SET
			weight_g      = EXCLUDED.weight_g,
			bmi           = COALESCE(EXCLUDED.bmi,           health_weight_entries.bmi),
			body_fat_pct  = COALESCE(EXCLUDED.body_fat_pct,  health_weight_entries.body_fat_pct),
			muscle_mass_g = COALESCE(EXCLUDED.muscle_mass_g, health_weight_entries.muscle_mass_g),
			bone_mass_g   = COALESCE(EXCLUDED.bone_mass_g,   health_weight_entries.bone_mass_g),
			source_type   = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_weight_entries.source_type)
						THEN EXCLUDED.source_type
						ELSE health_weight_entries.source_type
					END
	`, s.UserID, s.MeasuredAtUTC, s.WeightG, s.BMI, s.BodyFatPct, s.MuscleMassG, s.BoneMassG,
		s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthWeight(ctx context.Context, userID, from, to string) ([]model.HealthWeightEntry, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, measured_at_utc, weight_g, bmi, body_fat_pct, muscle_mass_g, bone_mass_g,
		       source_type, extra_data, created_at
		FROM health_weight_entries
		WHERE user_id = $1 AND measured_at_utc BETWEEN $2::timestamptz AND $3::timestamptz
		ORDER BY measured_at_utc ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health weight: %w", err)
	}
	defer rows.Close()

	var results []model.HealthWeightEntry
	for rows.Next() {
		var s model.HealthWeightEntry
		if err := rows.Scan(
			&s.UserID, &s.MeasuredAtUTC, &s.WeightG, &s.BMI, &s.BodyFatPct, &s.MuscleMassG, &s.BoneMassG,
			&s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health weight: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthWeight(ctx context.Context, userID string) (*model.HealthWeightEntry, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, measured_at_utc, weight_g, bmi, body_fat_pct, muscle_mass_g, bone_mass_g,
		       source_type, extra_data, created_at
		FROM health_weight_entries
		WHERE user_id = $1
		ORDER BY measured_at_utc DESC
		LIMIT 1
	`, userID)
	var s model.HealthWeightEntry
	err := row.Scan(
		&s.UserID, &s.MeasuredAtUTC, &s.WeightG, &s.BMI, &s.BodyFatPct, &s.MuscleMassG, &s.BoneMassG,
		&s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health weight: %w", err)
	}
	return &s, nil
}

// --- health_readiness_scores ---

func (db *DB) UpsertHealthReadinessScore(ctx context.Context, s *model.HealthReadinessScore) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_readiness_scores (
			user_id, calendar_date, score, level, sleep_score, hrv_weekly_avg_ms, recovery_time_min,
			acwr_pct, hrv_factor_pct, sleep_history_factor_pct, stress_history_factor_pct,
			recovery_time_factor_pct, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			score                      = COALESCE(EXCLUDED.score,                      health_readiness_scores.score),
			level                      = COALESCE(EXCLUDED.level,                      health_readiness_scores.level),
			sleep_score                = COALESCE(EXCLUDED.sleep_score,                health_readiness_scores.sleep_score),
			hrv_weekly_avg_ms          = COALESCE(EXCLUDED.hrv_weekly_avg_ms,          health_readiness_scores.hrv_weekly_avg_ms),
			recovery_time_min          = COALESCE(EXCLUDED.recovery_time_min,          health_readiness_scores.recovery_time_min),
			acwr_pct                   = COALESCE(EXCLUDED.acwr_pct,                   health_readiness_scores.acwr_pct),
			hrv_factor_pct             = COALESCE(EXCLUDED.hrv_factor_pct,             health_readiness_scores.hrv_factor_pct),
			sleep_history_factor_pct   = COALESCE(EXCLUDED.sleep_history_factor_pct,   health_readiness_scores.sleep_history_factor_pct),
			stress_history_factor_pct  = COALESCE(EXCLUDED.stress_history_factor_pct,  health_readiness_scores.stress_history_factor_pct),
			recovery_time_factor_pct   = COALESCE(EXCLUDED.recovery_time_factor_pct,   health_readiness_scores.recovery_time_factor_pct),
			source_type                = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_readiness_scores.source_type)
						THEN EXCLUDED.source_type
						ELSE health_readiness_scores.source_type
					END
	`, s.UserID, s.CalendarDate, s.Score, s.Level, s.SleepScore, s.HRVWeeklyAvgMS, s.RecoveryTimeMin,
		s.AcwrPct, s.HRVFactorPct, s.SleepHistoryFactorPct, s.StressHistoryFactorPct,
		s.RecoveryTimeFactorPct, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthReadiness(ctx context.Context, userID, from, to string) ([]model.HealthReadinessScore, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, calendar_date, score, level, sleep_score, hrv_weekly_avg_ms, recovery_time_min,
		       acwr_pct, hrv_factor_pct, sleep_history_factor_pct, stress_history_factor_pct,
		       recovery_time_factor_pct, source_type, extra_data, created_at
		FROM health_readiness_scores
		WHERE user_id = $1 AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health readiness: %w", err)
	}
	defer rows.Close()

	var results []model.HealthReadinessScore
	for rows.Next() {
		var s model.HealthReadinessScore
		var calDate time.Time
		if err := rows.Scan(
			&s.UserID, &calDate, &s.Score, &s.Level, &s.SleepScore, &s.HRVWeeklyAvgMS, &s.RecoveryTimeMin,
			&s.AcwrPct, &s.HRVFactorPct, &s.SleepHistoryFactorPct, &s.StressHistoryFactorPct,
			&s.RecoveryTimeFactorPct, &s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health readiness: %w", err)
		}
		s.CalendarDate = calDate.Format("2006-01-02")
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthReadiness(ctx context.Context, userID string) (*model.HealthReadinessScore, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, calendar_date, score, level, sleep_score, hrv_weekly_avg_ms, recovery_time_min,
		       acwr_pct, hrv_factor_pct, sleep_history_factor_pct, stress_history_factor_pct,
		       recovery_time_factor_pct, source_type, extra_data, created_at
		FROM health_readiness_scores
		WHERE user_id = $1
		ORDER BY calendar_date DESC
		LIMIT 1
	`, userID)
	var s model.HealthReadinessScore
	var calDate time.Time
	err := row.Scan(
		&s.UserID, &calDate, &s.Score, &s.Level, &s.SleepScore, &s.HRVWeeklyAvgMS, &s.RecoveryTimeMin,
		&s.AcwrPct, &s.HRVFactorPct, &s.SleepHistoryFactorPct, &s.StressHistoryFactorPct,
		&s.RecoveryTimeFactorPct, &s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health readiness: %w", err)
	}
	s.CalendarDate = calDate.Format("2006-01-02")
	return &s, nil
}

// --- health_training_load ---

func (db *DB) UpsertHealthTrainingLoad(ctx context.Context, s *model.HealthTrainingLoad) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_training_load (
			user_id, calendar_date, atl_acute, ctl_chronic, acwr_pct, acwr_status, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			atl_acute   = COALESCE(EXCLUDED.atl_acute,   health_training_load.atl_acute),
			ctl_chronic = COALESCE(EXCLUDED.ctl_chronic, health_training_load.ctl_chronic),
			acwr_pct    = COALESCE(EXCLUDED.acwr_pct,    health_training_load.acwr_pct),
			acwr_status = COALESCE(EXCLUDED.acwr_status, health_training_load.acwr_status),
			source_type = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_training_load.source_type)
						THEN EXCLUDED.source_type
						ELSE health_training_load.source_type
					END
	`, s.UserID, s.CalendarDate, s.ATLAcute, s.CTLChronic, s.AcwrPct, s.AcwrStatus, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthTrainingLoad(ctx context.Context, userID, from, to string) ([]model.HealthTrainingLoad, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, calendar_date, atl_acute, ctl_chronic, acwr_pct, acwr_status,
		       source_type, extra_data, created_at
		FROM health_training_load
		WHERE user_id = $1 AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health training load: %w", err)
	}
	defer rows.Close()

	var results []model.HealthTrainingLoad
	for rows.Next() {
		var s model.HealthTrainingLoad
		var calDate time.Time
		if err := rows.Scan(
			&s.UserID, &calDate, &s.ATLAcute, &s.CTLChronic, &s.AcwrPct, &s.AcwrStatus,
			&s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health training load: %w", err)
		}
		s.CalendarDate = calDate.Format("2006-01-02")
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthTrainingLoad(ctx context.Context, userID string) (*model.HealthTrainingLoad, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, calendar_date, atl_acute, ctl_chronic, acwr_pct, acwr_status,
		       source_type, extra_data, created_at
		FROM health_training_load
		WHERE user_id = $1
		ORDER BY calendar_date DESC
		LIMIT 1
	`, userID)
	var s model.HealthTrainingLoad
	var calDate time.Time
	err := row.Scan(
		&s.UserID, &calDate, &s.ATLAcute, &s.CTLChronic, &s.AcwrPct, &s.AcwrStatus,
		&s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health training load: %w", err)
	}
	s.CalendarDate = calDate.Format("2006-01-02")
	return &s, nil
}

// --- health_stress_snapshots ---

func (db *DB) UpsertHealthStressSnapshot(ctx context.Context, s *model.HealthStressSnapshot) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_stress_snapshots (
			user_id, measured_at_utc, stress_level, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, measured_at_utc) DO UPDATE SET
			stress_level = COALESCE(EXCLUDED.stress_level, health_stress_snapshots.stress_level),
			source_type  = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_stress_snapshots.source_type)
						THEN EXCLUDED.source_type
						ELSE health_stress_snapshots.source_type
					END
	`, s.UserID, s.MeasuredAtUTC, s.StressLevel, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthStress(ctx context.Context, userID, from, to string) ([]model.HealthStressSnapshot, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, measured_at_utc, stress_level, source_type, extra_data, created_at
		FROM health_stress_snapshots
		WHERE user_id = $1 AND measured_at_utc BETWEEN $2::timestamptz AND $3::timestamptz
		ORDER BY measured_at_utc ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health stress: %w", err)
	}
	defer rows.Close()

	var results []model.HealthStressSnapshot
	for rows.Next() {
		var s model.HealthStressSnapshot
		if err := rows.Scan(&s.UserID, &s.MeasuredAtUTC, &s.StressLevel, &s.SourceType, &s.ExtraData, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan health stress: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthStress(ctx context.Context, userID string) (*model.HealthStressSnapshot, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, measured_at_utc, stress_level, source_type, extra_data, created_at
		FROM health_stress_snapshots
		WHERE user_id = $1
		ORDER BY measured_at_utc DESC
		LIMIT 1
	`, userID)
	var s model.HealthStressSnapshot
	err := row.Scan(&s.UserID, &s.MeasuredAtUTC, &s.StressLevel, &s.SourceType, &s.ExtraData, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health stress: %w", err)
	}
	return &s, nil
}

// --- health_spo2_snapshots ---

func (db *DB) UpsertHealthSpO2Snapshot(ctx context.Context, s *model.HealthSpO2Snapshot) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_spo2_snapshots (
			user_id, measured_at_utc, spo2_pct, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, measured_at_utc) DO UPDATE SET
			spo2_pct    = COALESCE(EXCLUDED.spo2_pct, health_spo2_snapshots.spo2_pct),
			source_type = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_spo2_snapshots.source_type)
						THEN EXCLUDED.source_type
						ELSE health_spo2_snapshots.source_type
					END
	`, s.UserID, s.MeasuredAtUTC, s.SpO2Pct, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthSpO2(ctx context.Context, userID, from, to string) ([]model.HealthSpO2Snapshot, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, measured_at_utc, spo2_pct, source_type, extra_data, created_at
		FROM health_spo2_snapshots
		WHERE user_id = $1 AND measured_at_utc BETWEEN $2::timestamptz AND $3::timestamptz
		ORDER BY measured_at_utc ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health spo2: %w", err)
	}
	defer rows.Close()

	var results []model.HealthSpO2Snapshot
	for rows.Next() {
		var s model.HealthSpO2Snapshot
		if err := rows.Scan(&s.UserID, &s.MeasuredAtUTC, &s.SpO2Pct, &s.SourceType, &s.ExtraData, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan health spo2: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthSpO2(ctx context.Context, userID string) (*model.HealthSpO2Snapshot, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, measured_at_utc, spo2_pct, source_type, extra_data, created_at
		FROM health_spo2_snapshots
		WHERE user_id = $1
		ORDER BY measured_at_utc DESC
		LIMIT 1
	`, userID)
	var s model.HealthSpO2Snapshot
	err := row.Scan(&s.UserID, &s.MeasuredAtUTC, &s.SpO2Pct, &s.SourceType, &s.ExtraData, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health spo2: %w", err)
	}
	return &s, nil
}

// --- health_blood_pressure ---

func (db *DB) UpsertHealthBloodPressure(ctx context.Context, s *model.HealthBloodPressure) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_blood_pressure (
			user_id, measured_at_utc, systolic, diastolic, pulse, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, measured_at_utc) DO UPDATE SET
			systolic    = COALESCE(EXCLUDED.systolic,   health_blood_pressure.systolic),
			diastolic   = COALESCE(EXCLUDED.diastolic,  health_blood_pressure.diastolic),
			pulse       = COALESCE(EXCLUDED.pulse,      health_blood_pressure.pulse),
			source_type = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_blood_pressure.source_type)
						THEN EXCLUDED.source_type
						ELSE health_blood_pressure.source_type
					END
	`, s.UserID, s.MeasuredAtUTC, s.Systolic, s.Diastolic, s.Pulse, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthBloodPressure(ctx context.Context, userID, from, to string) ([]model.HealthBloodPressure, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, measured_at_utc, systolic, diastolic, pulse, source_type, extra_data, created_at
		FROM health_blood_pressure
		WHERE user_id = $1 AND measured_at_utc BETWEEN $2::timestamptz AND $3::timestamptz
		ORDER BY measured_at_utc ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health blood pressure: %w", err)
	}
	defer rows.Close()

	var results []model.HealthBloodPressure
	for rows.Next() {
		var s model.HealthBloodPressure
		if err := rows.Scan(
			&s.UserID, &s.MeasuredAtUTC, &s.Systolic, &s.Diastolic, &s.Pulse,
			&s.SourceType, &s.ExtraData, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan health blood pressure: %w", err)
		}
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthBloodPressure(ctx context.Context, userID string) (*model.HealthBloodPressure, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, measured_at_utc, systolic, diastolic, pulse, source_type, extra_data, created_at
		FROM health_blood_pressure
		WHERE user_id = $1
		ORDER BY measured_at_utc DESC
		LIMIT 1
	`, userID)
	var s model.HealthBloodPressure
	err := row.Scan(
		&s.UserID, &s.MeasuredAtUTC, &s.Systolic, &s.Diastolic, &s.Pulse,
		&s.SourceType, &s.ExtraData, &s.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health blood pressure: %w", err)
	}
	return &s, nil
}

// --- health_fitness_age ---

func (db *DB) UpsertHealthFitnessAge(ctx context.Context, s *model.HealthFitnessAge) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_fitness_age (
			user_id, calendar_date, fitness_age, source_type, extra_data
		) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			fitness_age = COALESCE(EXCLUDED.fitness_age, health_fitness_age.fitness_age),
			source_type = CASE
						WHEN source_type_priority(EXCLUDED.source_type) >= source_type_priority(health_fitness_age.source_type)
						THEN EXCLUDED.source_type
						ELSE health_fitness_age.source_type
					END
	`, s.UserID, s.CalendarDate, s.FitnessAge, s.SourceType, s.ExtraData)
	return err
}

func (db *DB) GetHealthFitnessAge(ctx context.Context, userID, from, to string) ([]model.HealthFitnessAge, error) {
	rows, err := db.Pool.Query(ctx, `
		SELECT user_id, calendar_date, fitness_age, source_type, extra_data, created_at
		FROM health_fitness_age
		WHERE user_id = $1 AND calendar_date BETWEEN $2::date AND $3::date
		ORDER BY calendar_date ASC
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("get health fitness age: %w", err)
	}
	defer rows.Close()

	var results []model.HealthFitnessAge
	for rows.Next() {
		var s model.HealthFitnessAge
		var calDate time.Time
		if err := rows.Scan(&s.UserID, &calDate, &s.FitnessAge, &s.SourceType, &s.ExtraData, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan health fitness age: %w", err)
		}
		s.CalendarDate = calDate.Format("2006-01-02")
		results = append(results, s)
	}
	return results, rows.Err()
}

func (db *DB) GetLatestHealthFitnessAge(ctx context.Context, userID string) (*model.HealthFitnessAge, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT user_id, calendar_date, fitness_age, source_type, extra_data, created_at
		FROM health_fitness_age
		WHERE user_id = $1
		ORDER BY calendar_date DESC
		LIMIT 1
	`, userID)
	var s model.HealthFitnessAge
	var calDate time.Time
	err := row.Scan(&s.UserID, &calDate, &s.FitnessAge, &s.SourceType, &s.ExtraData, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest health fitness age: %w", err)
	}
	s.CalendarDate = calDate.Format("2006-01-02")
	return &s, nil
}

// --- GetHealthOverview ---

// GetHealthOverview returns the most recent value of every health metric for a user.
func (db *DB) GetHealthOverview(ctx context.Context, userID string) (*model.HealthOverview, error) {
	overview := &model.HealthOverview{}
	var err error

	if overview.LatestDailySummary, err = db.GetLatestHealthDailySummary(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview daily summary: %w", err)
	}
	if overview.LatestSleep, err = db.GetLatestHealthSleep(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview sleep: %w", err)
	}
	if overview.LatestHRV, err = db.GetLatestHealthHRV(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview hrv: %w", err)
	}
	if overview.LatestWeight, err = db.GetLatestHealthWeight(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview weight: %w", err)
	}
	if overview.LatestReadiness, err = db.GetLatestHealthReadiness(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview readiness: %w", err)
	}
	if overview.LatestTrainingLoad, err = db.GetLatestHealthTrainingLoad(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview training load: %w", err)
	}
	if overview.LatestStress, err = db.GetLatestHealthStress(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview stress: %w", err)
	}
	if overview.LatestSpO2, err = db.GetLatestHealthSpO2(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview spo2: %w", err)
	}
	if overview.LatestBloodPressure, err = db.GetLatestHealthBloodPressure(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview blood pressure: %w", err)
	}
	if overview.LatestFitnessAge, err = db.GetLatestHealthFitnessAge(ctx, userID); err != nil {
		return nil, fmt.Errorf("overview fitness age: %w", err)
	}

	return overview, nil
}

// UpsertHealthHRVNightValuesBatch inserts or ignores a batch of 5-minute RMSSD night values.
// Uses ON CONFLICT DO NOTHING to avoid overwriting existing data.
func (db *DB) UpsertHealthHRVNightValuesBatch(ctx context.Context, userID string, values []model.HealthHRVNightValue) error {
	if len(values) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, v := range values {
		batch.Queue(`
			INSERT INTO health_hrv_night_values (user_id, ts, rmssd_ms, source_type)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, ts) DO NOTHING
		`, userID, v.TS, v.RmssdMS, v.SourceType)
	}
	return db.Pool.SendBatch(ctx, batch).Close()
}
