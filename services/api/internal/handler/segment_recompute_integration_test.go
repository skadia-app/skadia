package handler

// segment_recompute_integration_test.go — integration test for the transactional
// recomputeSegmentEffortsForActivity fix (R2-10).
//
// Requires TEST_DATABASE_URL to be set; the whole file is skipped otherwise.
//
// What is being tested:
//
//	TestRecomputeSegmentEffortsForActivity_TxFailureRollsBack
//	  Pre-seeds one segment_effort for (segment_id, activity_id).
//	  Injects a TxFactory that fails on the DELETE inside the transaction.
//	  Verifies that the pre-seeded effort is still present after the failed
//	  recompute — i.e. the DELETE was rolled back, not committed.
//
// Before R2-10: the DELETE was on Pool.Exec (auto-commit), so a failure *after*
// the DELETE would leave the pair with zero efforts.  The new implementation
// wraps both DELETE and INSERT in a single transaction, so any mid-tx failure
// preserves the pre-recompute state.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/google/uuid"
)

func TestRecomputeSegmentEffortsForActivity_TxFailureRollsBack(t *testing.T) {
	pool := fusionTestPool(t) // skips if TEST_DATABASE_URL is not set
	ctx := context.Background()
	db := &database.DB{Pool: pool}
	userID := insertTestUser(t, pool)

	// Insert a minimal activity.
	actID := insertMinimalActivity(t, db, userID)

	// Insert a minimal segment.
	segID := uuid.New().String()
	_, err := pool.Exec(ctx, `
		INSERT INTO segments (id, created_by, name, sport_group, segment_type)
		VALUES ($1, $2, 'r2-10-rollback-test', 'cycling', 'standard')
	`, segID, userID)
	if err != nil {
		t.Fatalf("insert segment: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM segments WHERE id = $1", segID) //nolint:errcheck
	})

	// Insert a minimal segment_version so the FK on segment_efforts is satisfied.
	segVersionID := uuid.New().String()
	_, err = pool.Exec(ctx, `
		INSERT INTO segment_versions (id, segment_id, version_number, track, valid_from)
		VALUES ($1, $2, 1,
			ST_GeomFromText('LINESTRING(16.361 48.201, 16.362 48.202)', 4326),
			'2000-01-01')
	`, segVersionID, segID)
	if err != nil {
		t.Fatalf("insert segment_version: %v", err)
	}

	// Pre-seed one segment_effort for (segment_id, activity_id).
	// This is the effort that must survive the failed recompute.
	startedAt := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `
		INSERT INTO segment_efforts
		  (segment_id, segment_version_id, activity_id, user_id,
		   started_at, elapsed_time_s, sport_type, sub_type)
		VALUES ($1, $2, $3, $4, $5, 120, 'cycling', 'generic')
	`, segID, segVersionID, actID, userID, startedAt)
	if err != nil {
		t.Fatalf("pre-seed segment_effort: %v", err)
	}

	// Verify pre-seed is there.
	assertEffortCount := func(wantCount int) {
		t.Helper()
		var count int
		if err := pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM segment_efforts WHERE segment_id = $1 AND activity_id = $2`,
			segID, actID,
		).Scan(&count); err != nil {
			t.Fatalf("count segment_efforts: %v", err)
		}
		if count != wantCount {
			t.Errorf("segment_efforts count: got %d, want %d", count, wantCount)
		}
	}
	assertEffortCount(1)

	// Inject a TxFactory that fails on the DELETE inside the tx.
	// This simulates any mid-transaction failure that would previously leave
	// the (segment_id, activity_id) pair with zero efforts.
	injectedErr := fmt.Errorf("injected: DELETE failure for R2-10 rollback test")
	h := &Handler{
		DB: db,
		TxFactory: func(txCtx context.Context) (pgx.Tx, error) {
			realTx, err := db.Pool.Begin(txCtx)
			if err != nil {
				return nil, err
			}
			return &mockTx{
				Tx:          realTx,
				shouldFail:  sqlContains("delete from segment_efforts"),
				injectedErr: injectedErr,
			}, nil
		},
	}

	// recomputeSegmentEffortsForActivity must return an error (the injected one).
	err = h.recomputeSegmentEffortsForActivity(ctx, segID, actID, userID)
	if err == nil {
		t.Fatal("expected error from recomputeSegmentEffortsForActivity, got nil")
	}

	// The pre-seeded effort must still be present — the DELETE was rolled back.
	assertEffortCount(1)
}

