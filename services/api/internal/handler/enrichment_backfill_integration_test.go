package handler

// Integration test for GeocodingBackfill idempotency (#30).
//
// Uses the per-user unique partial index idx_bg_jobs_one_running to verify
// that a second concurrent call for the same (user, job_type) is rejected at
// the DB layer.  Requires TEST_DATABASE_URL to be set.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

func backfillTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func backfillTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	userID := uuid.New().String()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"backfill_test_"+userID[:8],
		"backfill_test_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE user_id = $1", userID) //nolint:errcheck
		pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)                //nolint:errcheck
	})
	return userID
}

// TestGeocodingBackfill_IdempotencyGuard verifies that a second
// InsertBackgroundJobGuarded call for the same (user, 'geocoding_backfill')
// returns inserted=false when the first is still running (#30).
func TestGeocodingBackfill_IdempotencyGuard(t *testing.T) {
	pool := backfillTestPool(t)
	db := &database.DB{Pool: pool}
	userID := backfillTestUser(t, pool)
	ctx := context.Background()

	firstID, firstInserted, err := db.InsertBackgroundJobGuarded(
		ctx, userID, "geocoding_backfill", nil, nil, 100)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if !firstInserted {
		t.Fatal("first InsertBackgroundJobGuarded should have inserted")
	}
	if firstID == "" {
		t.Fatal("first job ID is empty")
	}

	// Second call must be rejected because the first is still 'running'.
	secondID, secondInserted, err := db.InsertBackgroundJobGuarded(
		ctx, userID, "geocoding_backfill", nil, nil, 100)
	if err != nil {
		t.Fatalf("second insert: %v", err)
	}
	if secondInserted {
		t.Error("second InsertBackgroundJobGuarded should have been blocked by partial unique index")
	}
	if secondID != "" {
		t.Errorf("second job ID should be empty when blocked, got %q", secondID)
	}

	// After completing the first job, a new insert must succeed.
	if err := db.CompleteBackgroundJob(ctx, firstID, false, ""); err != nil {
		t.Fatalf("complete first job: %v", err)
	}
	thirdID, thirdInserted, err := db.InsertBackgroundJobGuarded(
		ctx, userID, "geocoding_backfill", nil, nil, 100)
	if err != nil {
		t.Fatalf("third insert: %v", err)
	}
	if !thirdInserted {
		t.Error("third InsertBackgroundJobGuarded should succeed after first completed")
	}
	if thirdID == "" {
		t.Error("third job ID is empty")
	}
}
