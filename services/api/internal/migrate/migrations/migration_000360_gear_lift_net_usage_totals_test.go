package migrations_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func TestMigration_GearLiftNetUsageTotals_RecomputesNetGearTotals(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	sqlBytes, err := os.ReadFile("000360_gear_lift_net_usage_totals.up.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)

	userID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "migration-gear-net-"+userID[:8], "migration-gear-net-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	gearID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear
			(id, user_id, name, type, scope, total_distance_m, total_elevation_gain_m, total_elevation_loss_m)
		VALUES ($1, $2, 'Migration Net Gear', 'bike', 'bike', 10000, 900, 600)`,
		gearID, userID); err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	activityID := uuid.NewString()
	startedAt := time.Now().UTC().Truncate(time.Second)
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, started_at, ended_at, file_hash,
			 distance_m, elapsed_time_s, elevation_gain_m, elevation_loss_m,
			 lift_distance_m, lift_ascent_m, lift_descent_m,
			 exclude_lift_subtraction, is_group_secondary)
		VALUES ($1, $2, 'cycling', $3, $4, $5,
		        10000, 1800, 900, 600,
		        2000, 250, 120,
		        false, false)`,
		activityID, userID, startedAt, startedAt.Add(30*time.Minute), uuid.NewString()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id, role_key, source)
		VALUES ($1, $2, $3, 'primary', 'user')`,
		userID, activityID, gearID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}
	groupSecondaryID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities
			(id, user_id, sport_type, started_at, ended_at, file_hash,
			 distance_m, moving_time_s, elapsed_time_s, elevation_gain_m, elevation_loss_m,
			 lift_distance_m, lift_ascent_m, lift_descent_m,
			 exclude_lift_subtraction, is_group_secondary)
		VALUES ($1, $2, 'cycling', $3, $4, $5,
		        50000, 3600, 4000, 5000, 3000,
		        1000, 100, 100,
		        false, true)`,
		groupSecondaryID, userID, startedAt, startedAt.Add(30*time.Minute), uuid.NewString()); err != nil {
		t.Fatalf("insert group-secondary activity: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_gear_assignments
			(user_id, activity_id, gear_id, role_key, source, distance_m_override, moving_time_s_override)
		VALUES ($1, $2, $3, 'primary', 'user', 99999, 9999)`,
		userID, groupSecondaryID, gearID); err != nil {
		t.Fatalf("insert group-secondary assignment: %v", err)
	}

	if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	var totalDistance, totalGain, totalLoss float64
	if err := tx.QueryRow(ctx, `
		SELECT total_distance_m, total_elevation_gain_m, total_elevation_loss_m
		FROM gear WHERE id = $1`, gearID).Scan(&totalDistance, &totalGain, &totalLoss); err != nil {
		t.Fatalf("query gear totals: %v", err)
	}
	if totalDistance != 8000 {
		t.Fatalf("total_distance_m = %.0f, want 8000", totalDistance)
	}
	if totalGain != 650 {
		t.Fatalf("total_elevation_gain_m = %.0f, want 650", totalGain)
	}
	if totalLoss != 480 {
		t.Fatalf("total_elevation_loss_m = %.0f, want 480", totalLoss)
	}
}
