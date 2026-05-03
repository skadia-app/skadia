package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func bestEffortsCleanupDB(t *testing.T) (*DB, string, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	suffix := uuid.NewString()[:8]
	var userID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"best-efforts-cleanup-"+suffix,
		"best-efforts-cleanup-"+suffix+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})

	activityID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
		 VALUES ($1, $2, 'running', 'Cleanup Test', $3, $4, 'private', 'complete')`,
		activityID, userID, time.Now().UTC(), "best-efforts-cleanup-"+suffix,
	); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	return &DB{Pool: pool}, userID, activityID
}

func TestDeleteOrphanedBestEffortsRemovesInvalidSportGroups(t *testing.T) {
	db, userID, activityID := bestEffortsCleanupDB(t)
	ctx := context.Background()

	for _, sportGroup := range []string{"running", "trail_running"} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO best_efforts (user_id, activity_id, category, sport_group, elapsed_time_s, achieved_at)
			 VALUES ($1, $2, 'dist_5km', $3, 1200, $4)`,
			userID, activityID, sportGroup, time.Now().UTC(),
		); err != nil {
			t.Fatalf("insert best_effort %s: %v", sportGroup, err)
		}
	}

	if err := db.DeleteOrphanedBestEfforts(ctx, userID, []string{activityID}); err != nil {
		t.Fatalf("DeleteOrphanedBestEfforts: %v", err)
	}

	var sportGroups []string
	rows, err := db.Pool.Query(ctx, `SELECT sport_group FROM best_efforts WHERE user_id = $1 ORDER BY sport_group`, userID)
	if err != nil {
		t.Fatalf("query best_efforts: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sportGroup string
		if err := rows.Scan(&sportGroup); err != nil {
			t.Fatalf("scan sport_group: %v", err)
		}
		sportGroups = append(sportGroups, sportGroup)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	if len(sportGroups) != 1 || sportGroups[0] != "running" {
		t.Fatalf("sport_groups after cleanup = %#v, want [running]", sportGroups)
	}
}
