package database

// Integration tests for descent (elevation_loss_m) analytics surfaces.
// Restored from backup/gear-descent-analytics-pre-history-cleanup after the
// tabs-refactor deleted the file.
//
// Adaptation from backup:
//   - stats.TotalElevationGainM → stats.TotalElevationM (field was renamed;
//     TotalElevationM holds gain, TotalElevationLossM holds loss).
//
// Requires TEST_DATABASE_URL pointing at the per-worktree test DB
// (managed by scripts/test_db.sh). Tests skip if TEST_DATABASE_URL is unset.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// insertAnalyticsActivity inserts a minimal activity row for analytics tests.
// gainM and lossM are the elevation_gain_m / elevation_loss_m values.
func insertAnalyticsActivity(t *testing.T, db *DB, userID string, startedAt time.Time, gainM, lossM float64) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, sport_type, started_at, distance_m, moving_time_s, elevation_gain_m, elevation_loss_m)
		VALUES ($1, $2, 'cycling', $3, 10000, 1800, $4, $5)`,
		uuid.NewString(), userID, startedAt, gainM, lossM)
	if err != nil {
		t.Fatalf("insertAnalyticsActivity: %v", err)
	}
}

// TestLifetimeStatsIncludesElevationLoss verifies that GetLifetimeStats
// returns the correct total_elevation_loss_m for the user.
// TotalElevationM holds gain (the field was TotalElevationGainM in an earlier
// iteration); TotalElevationLossM holds descent.
func TestLifetimeStatsIncludesElevationLoss(t *testing.T) {
	db := mapLayerTestDB(t)
	userID := insertTestUser(t, db)
	insertAnalyticsActivity(t, db, userID, time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC), 100, 700)
	insertAnalyticsActivity(t, db, userID, time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), 300, 900)

	stats, err := db.GetLifetimeStats(context.Background(), userID, "", nil)
	if err != nil {
		t.Fatalf("GetLifetimeStats: %v", err)
	}
	// TotalElevationM holds gain (100 + 300 = 400).
	if stats.TotalElevationM != 400 {
		t.Fatalf("gain (TotalElevationM) = %v, want 400", stats.TotalElevationM)
	}
	// TotalElevationLossM holds descent (700 + 900 = 1600).
	if stats.TotalElevationLossM != 1600 {
		t.Fatalf("loss = %v, want 1600", stats.TotalElevationLossM)
	}
}

// TestAnalyticsMetricWhitelistIncludesElevationLoss verifies that
// GetYearProgression accepts "elevation_loss_m" as a metric key and returns
// the correct aggregated value.
func TestAnalyticsMetricWhitelistIncludesElevationLoss(t *testing.T) {
	db := mapLayerTestDB(t)
	userID := insertTestUser(t, db)
	insertAnalyticsActivity(t, db, userID, time.Date(2026, 2, 1, 10, 0, 0, 0, time.UTC), 100, 700)

	rows, err := db.GetYearProgression(context.Background(), userID, "elevation_loss_m", "", nil)
	if err != nil {
		t.Fatalf("GetYearProgression: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != 700 {
		t.Fatalf("year progression rows = %#v, want one row value 700", rows)
	}
}

// TestDistributionCanAggregateElevationLoss verifies that GetDistribution
// accepts "elevation_loss_m" as the metric and sums values correctly.
func TestDistributionCanAggregateElevationLoss(t *testing.T) {
	db := mapLayerTestDB(t)
	userID := insertTestUser(t, db)
	insertAnalyticsActivity(t, db, userID, time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC), 100, 700)
	insertAnalyticsActivity(t, db, userID, time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC), 100, 900)

	rows, err := db.GetDistribution(context.Background(), userID, "sport", "elevation_loss_m", "", nil)
	if err != nil {
		t.Fatalf("GetDistribution: %v", err)
	}
	if len(rows) != 1 || rows[0].Value != 1600 {
		t.Fatalf("distribution rows = %#v, want one row value 1600", rows)
	}
}
