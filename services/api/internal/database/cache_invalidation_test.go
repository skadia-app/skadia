//go:build integration

package database

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// cacheInvTestPool returns a *pgxpool.Pool connected to TEST_DATABASE_URL.
// Skips the test if the env var is unset.
func cacheInvTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("cacheInvTestPool: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedUserAndActivities inserts one user and two activities (merged + source)
// with the minimal required columns. Returns their string IDs and registers
// t.Cleanup to delete all three rows in the correct FK order.
func seedUserAndActivities(t *testing.T, pool *pgxpool.Pool) (userID, mergedID, sourceID string) {
	t.Helper()
	ctx := context.Background()

	// Unique suffix so parallel test runs don't collide on email/username.
	suffix := fmt.Sprintf("%d-%s", time.Now().UnixNano(), uuid.New().String()[:8])

	// Insert user.  username + email + registration_source are the only
	// NOT NULL columns that lack a DEFAULT (observed from rename_system_rule_test.go
	// which uses the identical minimal shape).
	var uid string
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'test')
		 RETURNING id`,
		"ci-fusion-"+suffix,
		"ci-fusion-"+suffix+"@test.localhost",
	).Scan(&uid); err != nil {
		t.Fatalf("seedUserAndActivities: insert user: %v", err)
	}

	// Insert two minimal activities (reuse the same minimal shape as
	// InsertMultisportParent in activities_parent.go).
	insertAct := `INSERT INTO activities (id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status)
	              VALUES ($1, $2, $3, $4, $5, $6, 'private', 'complete')`

	mID := uuid.New().String()
	sID := uuid.New().String()
	now := time.Now().UTC()

	if _, err := pool.Exec(ctx, insertAct,
		mID, uid, "running", "fusion-merged-"+suffix, now, "hash-m-"+suffix,
	); err != nil {
		t.Fatalf("seedUserAndActivities: insert merged activity: %v", err)
	}

	if _, err := pool.Exec(ctx, insertAct,
		sID, uid, "running", "fusion-source-"+suffix, now.Add(time.Second), "hash-s-"+suffix,
	); err != nil {
		t.Fatalf("seedUserAndActivities: insert source activity: %v", err)
	}

	t.Cleanup(func() {
		// Delete in FK-safe order: memberships first (FK on activities), then
		// activities (FK on users), then user.
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM activity_fusion_memberships WHERE user_id = $1`, uid)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM activities WHERE id IN ($1, $2)`, mID, sID)
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM users WHERE id = $1`, uid)
	})

	return uid, mID, sID
}

// readFusionInvSeq reads users.fusion_invalidation_seq for the given userID.
func readFusionInvSeq(t *testing.T, pool *pgxpool.Pool, userID string) int64 {
	t.Helper()
	var seq int64
	if err := pool.QueryRow(context.Background(),
		`SELECT fusion_invalidation_seq FROM users WHERE id = $1`, userID,
	).Scan(&seq); err != nil {
		t.Fatalf("readFusionInvSeq: %v", err)
	}
	return seq
}

// TestMembershipInsertBumpsInvalidationSeq verifies that inserting a row into
// activity_fusion_memberships increments users.fusion_invalidation_seq.
func TestMembershipInsertBumpsInvalidationSeq(t *testing.T) {
	pool := cacheInvTestPool(t)
	ctx := context.Background()

	userID, mergedID, sourceID := seedUserAndActivities(t, pool)

	before := readFusionInvSeq(t, pool, userID)

	if _, err := pool.Exec(ctx,
		`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		 VALUES ($1, $2, 'gps', $3)`,
		mergedID, sourceID, userID,
	); err != nil {
		t.Fatalf("insert membership: %v", err)
	}

	after := readFusionInvSeq(t, pool, userID)
	if after <= before {
		t.Errorf("INSERT: expected fusion_invalidation_seq to increase, got before=%d after=%d", before, after)
	}
}

// TestMembershipDeleteBumpsInvalidationSeq verifies that deleting a row from
// activity_fusion_memberships increments users.fusion_invalidation_seq.
func TestMembershipDeleteBumpsInvalidationSeq(t *testing.T) {
	pool := cacheInvTestPool(t)
	ctx := context.Background()

	userID, mergedID, sourceID := seedUserAndActivities(t, pool)

	// Pre-insert a membership (trigger fires here too — we read seq after).
	if _, err := pool.Exec(ctx,
		`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		 VALUES ($1, $2, 'gps', $3)`,
		mergedID, sourceID, userID,
	); err != nil {
		t.Fatalf("pre-insert membership: %v", err)
	}

	before := readFusionInvSeq(t, pool, userID)

	if _, err := pool.Exec(ctx,
		`DELETE FROM activity_fusion_memberships
		 WHERE merged_id = $1 AND source_id = $2 AND field_key = 'gps'`,
		mergedID, sourceID,
	); err != nil {
		t.Fatalf("delete membership: %v", err)
	}

	after := readFusionInvSeq(t, pool, userID)
	if after <= before {
		t.Errorf("DELETE: expected fusion_invalidation_seq to increase, got before=%d after=%d", before, after)
	}
}

// TestMembershipUpsertBumpsInvalidationSeq verifies that an INSERT … ON CONFLICT
// DO UPDATE (which fires the UPDATE path of the trigger) also increments
// users.fusion_invalidation_seq.
//
// The PRIMARY KEY is (merged_id, source_id, field_key).  Since none of the PK
// columns are updatable we set user_id = EXCLUDED.user_id, which is technically
// a no-op value-wise but still fires the AFTER UPDATE trigger row — sufficient
// to validate the UPDATE trigger path.
func TestMembershipUpsertBumpsInvalidationSeq(t *testing.T) {
	pool := cacheInvTestPool(t)
	ctx := context.Background()

	userID, mergedID, sourceID := seedUserAndActivities(t, pool)

	// Insert the membership for the first time.
	if _, err := pool.Exec(ctx,
		`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		 VALUES ($1, $2, 'gps', $3)`,
		mergedID, sourceID, userID,
	); err != nil {
		t.Fatalf("initial insert: %v", err)
	}

	before := readFusionInvSeq(t, pool, userID)

	// Upsert the same row — conflicts on PK, UPDATE triggers.
	if _, err := pool.Exec(ctx,
		`INSERT INTO activity_fusion_memberships (merged_id, source_id, field_key, user_id)
		 VALUES ($1, $2, 'gps', $3)
		 ON CONFLICT (merged_id, source_id, field_key)
		 DO UPDATE SET user_id = EXCLUDED.user_id`,
		mergedID, sourceID, userID,
	); err != nil {
		t.Fatalf("upsert membership: %v", err)
	}

	after := readFusionInvSeq(t, pool, userID)
	if after <= before {
		t.Errorf("UPSERT/UPDATE: expected fusion_invalidation_seq to increase, got before=%d after=%d", before, after)
	}
}
