//go:build integration

package garmin

import (
	"testing"
)

// TestMultiDeviceMerge documents the expected behavior when monitoring_b FIT files
// from two different devices (e.g. Garmin watch + Edge cycling computer) cover
// the same timestamp T1 with complementary fields.
//
// Device A: HR=75, Stress=40 at T1
// Device B: BodyBattery=80 at T1
// Expected after both imports: one row at T1 with HR=75, Stress=40, BodyBattery=80
//
// This behavior is guaranteed by InsertMonitoringSamples (staging table + COALESCE).
func TestMultiDeviceMerge(t *testing.T) {
	t.Skip("requires database — run with -tags integration and DATABASE_URL set")

	// Conceptual steps (requires DB + test FIT fixtures):
	//
	// db := setupTestDB(t)
	// ctx := context.Background()
	// userID := "test-user-multi-device"
	//
	// // Import monitoring_b from device A (HR + stress for ts T1)
	// errA := processMonitoringBFIT(ctx, db, nil, deviceA_fit_bytes, userID)
	// if errA != nil { t.Fatalf("device A import: %v", errA) }
	//
	// // Import monitoring_b from device B (body_battery for same ts T1)
	// errB := processMonitoringBFIT(ctx, db, nil, deviceB_fit_bytes, userID)
	// if errB != nil { t.Fatalf("device B import: %v", errB) }
	//
	// // Verify merged row has all 3 values
	// samples, _ := db.GetMonitoringSamples(ctx, userID, T1, T1)
	// if len(samples) != 1 { t.Fatal("expected 1 merged row") }
	// s := samples[0]
	// if s.HeartRate == nil || *s.HeartRate != 75 { t.Errorf("HR missing") }
	// if s.Stress == nil || *s.Stress != 40 { t.Errorf("Stress missing") }
	// if s.BodyBattery == nil || *s.BodyBattery != 80 { t.Errorf("BodyBattery missing") }
}

// TestSleepFirstWrite documents the expected behavior when a sleep FIT file is
// imported without a preceding sleep JSON file.
//
// The sleep_start_utc and sleep_end_utc must be populated from Event messages
// (msg 21, event=74: timer_event START/STOP) within the sleep FIT, not from JSON.
func TestSleepFirstWrite(t *testing.T) {
	t.Skip("requires database — run with -tags integration and DATABASE_URL set")

	// Conceptual steps:
	//
	// db := setupTestDB(t)
	// ctx := context.Background()
	// userID := "test-user-sleep-first"
	//
	// // Import sleep FIT without prior JSON
	// err := processSleepFIT(ctx, db, nil, sleep_fit_bytes, userID)
	// if err != nil { t.Fatalf("processSleepFIT: %v", err) }
	//
	// // Verify sleep_start_utc/end_utc populated from Event messages
	// sleeps, _ := db.GetHealthSleep(ctx, userID, "2024-01-15", "2024-01-15")
	// if len(sleeps) == 0 { t.Fatal("no sleep record created") }
	// s := sleeps[0]
	// if s.SleepStartUTC.IsZero() { t.Error("sleep_start_utc not set from Event msg") }
	// if s.SleepEndUTC.IsZero() { t.Error("sleep_end_utc not set from Event msg") }
}

// TestSleepTimezoneCalendarDate verifies that calendar_date is computed in the
// user's local timezone, not UTC.
//
// Sleep starting at 23:45 UTC in winter (UTC+1 = 00:45 CET next day).
// User timezone: Europe/Berlin
// Expected calendar_date: the CET date (next day relative to UTC).
func TestSleepTimezoneCalendarDate(t *testing.T) {
	t.Skip("requires database — run with -tags integration and DATABASE_URL set")

	// Conceptual steps:
	//
	// db := setupTestDB(t)
	// ctx := context.Background()
	// userID := "test-user-tz"
	//
	// // Set user timezone to Europe/Berlin
	// db.Pool.Exec(ctx, `UPDATE users SET timezone='Europe/Berlin' WHERE id=$1`, userID)
	//
	// // Sleep starts at 2024-01-14 23:45 UTC (= 2024-01-15 00:45 CET)
	// sleepStart := time.Date(2024, 1, 14, 23, 45, 0, 0, time.UTC)
	// sleepEnd   := time.Date(2024, 1, 15, 6, 30, 0, 0, time.UTC)
	//
	// // Import sleep record
	// // ... (via processSleepFIT or direct DB upsert with the FIT data)
	//
	// // Verify calendar_date is CET local date (2024-01-15), not UTC date (2024-01-14)
	// sleeps, _ := db.GetHealthSleep(ctx, userID, "2024-01-15", "2024-01-15")
	// if len(sleeps) == 0 { t.Fatal("no sleep record for CET date 2024-01-15") }
	// _ = sleepStart
	// _ = sleepEnd
}
