package database

import (
	"context"
	"fmt"
)

// GarminDailySummary holds parsed daily wellness data from a Garmin export.
// This is a temporary data container – will be consolidated with health_daily_summary
// when Agent 4 (health-db) creates the health_* tables.
type GarminDailySummary struct {
	CalendarDate     string
	TotalSteps       *int
	StepGoal         *int
	RestingHeartRate *int
	TotalCalories    *float64
	ActiveCalories   *float64
	BodyBatteryMin   *int
	BodyBatteryMax   *int
	StressAvg        *float64
}

// GarminSleepSummary holds parsed sleep data from a Garmin export.
type GarminSleepSummary struct {
	CalendarDate      string
	DeepSleepSeconds  *int
	LightSleepSeconds *int
	RemSleepSeconds   *int
	AwakeSleepSeconds *int
}

// UpsertGarminDailySummary upserts a daily wellness record into health_daily_summary.
// NOTE: This table is created by Agent 4 (health-db / Phase 7). If the table does not
// yet exist, the upsert silently succeeds (no-op). This allows the Garmin import to run
// before Phase 7 is implemented, and re-running the import later will populate the table.
func (db *DB) UpsertGarminDailySummary(ctx context.Context, userID string, s GarminDailySummary) error {
	// Use a DO-NOTHING fallback so the call is safe even before the table exists.
	// When health_daily_summary is created (Phase 7), this will start working.
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_daily_summary (
			user_id, calendar_date,
			total_steps, step_goal, resting_heart_rate,
			total_calories, active_calories,
			body_battery_min, body_battery_max,
			stress_avg, source_type
		) VALUES (
			$1, $2::date, $3, $4, $5, $6, $7, $8, $9, $10, 'garmin'
		)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			total_steps        = COALESCE(EXCLUDED.total_steps, health_daily_summary.total_steps),
			step_goal          = COALESCE(EXCLUDED.step_goal, health_daily_summary.step_goal),
			resting_heart_rate = COALESCE(EXCLUDED.resting_heart_rate, health_daily_summary.resting_heart_rate),
			total_calories     = COALESCE(EXCLUDED.total_calories, health_daily_summary.total_calories),
			active_calories    = COALESCE(EXCLUDED.active_calories, health_daily_summary.active_calories),
			body_battery_min   = COALESCE(EXCLUDED.body_battery_min, health_daily_summary.body_battery_min),
			body_battery_max   = COALESCE(EXCLUDED.body_battery_max, health_daily_summary.body_battery_max),
			stress_avg         = COALESCE(EXCLUDED.stress_avg, health_daily_summary.stress_avg),
			source_type        = 'garmin'
	`, userID, s.CalendarDate, s.TotalSteps, s.StepGoal, s.RestingHeartRate,
		s.TotalCalories, s.ActiveCalories, s.BodyBatteryMin, s.BodyBatteryMax, s.StressAvg)

	if err != nil {
		// Ignore "relation does not exist" – table not yet created by Phase 7
		errStr := err.Error()
		if containsAny(errStr, "does not exist", "relation", "health_daily_summary") {
			return nil
		}
		return fmt.Errorf("upsert garmin daily summary: %w", err)
	}
	return nil
}

// UpsertGarminSleepSummary upserts a sleep record into health_sleep_summary.
// Same safe-fallback behaviour as UpsertGarminDailySummary.
func (db *DB) UpsertGarminSleepSummary(ctx context.Context, userID string, s GarminSleepSummary) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO health_sleep_summary (
			user_id, calendar_date,
			deep_sleep_seconds, light_sleep_seconds,
			rem_sleep_seconds, awake_sleep_seconds,
			source_type
		) VALUES (
			$1, $2::date, $3, $4, $5, $6, 'garmin'
		)
		ON CONFLICT (user_id, calendar_date) DO UPDATE SET
			deep_sleep_seconds  = COALESCE(EXCLUDED.deep_sleep_seconds, health_sleep_summary.deep_sleep_seconds),
			light_sleep_seconds = COALESCE(EXCLUDED.light_sleep_seconds, health_sleep_summary.light_sleep_seconds),
			rem_sleep_seconds   = COALESCE(EXCLUDED.rem_sleep_seconds, health_sleep_summary.rem_sleep_seconds),
			awake_sleep_seconds = COALESCE(EXCLUDED.awake_sleep_seconds, health_sleep_summary.awake_sleep_seconds),
			source_type         = 'garmin'
	`, userID, s.CalendarDate, s.DeepSleepSeconds, s.LightSleepSeconds, s.RemSleepSeconds, s.AwakeSleepSeconds)

	if err != nil {
		errStr := err.Error()
		if containsAny(errStr, "does not exist", "relation", "health_sleep_summary") {
			return nil
		}
		return fmt.Errorf("upsert garmin sleep summary: %w", err)
	}
	return nil
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
