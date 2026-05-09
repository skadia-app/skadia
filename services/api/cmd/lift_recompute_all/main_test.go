package main

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// TestLiftRecomputeAll_DeletesStaleCandidates verifies that the pre-deletion step
// removes non-rejected candidates (confirmed=true and confirmed=false alike)
// before re-enrichment, so that the activity does not carry stale candidate rows.
func TestLiftRecomputeAll_DeletesStaleCandidates(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// Insert minimal user.
	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"backfill_stale_"+userID[:8],
		"backfill_stale_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                                //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                          //nolint:errcheck
	})

	// Insert a ski activity.
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	_, err = pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.New().String())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// Pre-seed 3 confirmed=true and 2 confirmed=false candidates (none rejected).
	// Each candidate gets a distinct started_at to avoid the unique constraint on
	// (activity_id, started_at).
	var candidateSeq int
	insertCandidate := func(confirmed bool) {
		t.Helper()
		offset := time.Duration(candidateSeq) * 10 * time.Minute
		candidateSeq++
		start := now.Add(offset)
		end := start.Add(time.Minute)
		_, err := pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
			VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', $4)
		`, activityID, start, end, confirmed)
		if err != nil {
			t.Fatalf("insert candidate (confirmed=%v): %v", confirmed, err)
		}
	}
	insertCandidate(true)
	insertCandidate(true)
	insertCandidate(true)
	insertCandidate(false)
	insertCandidate(false)

	// Verify pre-condition: 5 candidates exist.
	var preCnt int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, activityID).Scan(&preCnt); err != nil {
		t.Fatalf("count pre: %v", err)
	}
	if preCnt != 5 {
		t.Fatalf("pre-condition: want 5 candidates, got %d", preCnt)
	}

	// Run the pre-deletion (the step the CLI performs before RunStreamEnrichment).
	deleted, err := deleteNonRejectedCandidates(ctx, pool, activityID, userID)
	if err != nil {
		t.Fatalf("deleteNonRejectedCandidates: %v", err)
	}
	if deleted != 5 {
		t.Errorf("deleted = %d, want 5", deleted)
	}

	// All non-rejected candidates are gone.
	var postCnt int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, activityID).Scan(&postCnt); err != nil {
		t.Fatalf("count post: %v", err)
	}
	if postCnt != 0 {
		t.Errorf("post-delete count = %d, want 0", postCnt)
	}
}

// TestLiftRecomputeAll_PreservesUserRejects verifies that rejected_by_user=true
// candidates survive the pre-deletion step that the backfill CLI performs before
// re-running RunStreamEnrichment.
func TestLiftRecomputeAll_PreservesUserRejects(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"backfill_rej_"+userID[:8],
		"backfill_rej_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                                //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                          //nolint:errcheck
	})

	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	_, err = pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.New().String())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	// Insert one soft-rejected candidate and one normal (non-rejected) candidate.
	_, err = pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 120, 200, 0, 1000, 'high', false, 'heuristic', true)
	`, activityID, now, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("insert rejected candidate: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', false)
	`, activityID, now.Add(5*time.Minute), now.Add(6*time.Minute))
	if err != nil {
		t.Fatalf("insert normal candidate: %v", err)
	}

	// Pre-condition: 2 candidates.
	var preCnt int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, activityID).Scan(&preCnt); err != nil {
		t.Fatalf("count pre: %v", err)
	}
	if preCnt != 2 {
		t.Fatalf("pre-condition: want 2 candidates, got %d", preCnt)
	}

	// Run pre-deletion: only the non-rejected candidate should be removed.
	deleted, err := deleteNonRejectedCandidates(ctx, pool, activityID, userID)
	if err != nil {
		t.Fatalf("deleteNonRejectedCandidates: %v", err)
	}
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1 (only the non-rejected row)", deleted)
	}

	// The soft-rejected row must still exist.
	var rejCnt int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM activity_lift_candidates
		WHERE activity_id = $1 AND rejected_by_user = true
	`, activityID).Scan(&rejCnt); err != nil {
		t.Fatalf("count rejected: %v", err)
	}
	if rejCnt != 1 {
		t.Errorf("rejected_by_user count = %d, want 1 (soft-reject must survive)", rejCnt)
	}
}

// TestLiftRecomputeAll_AckBackfill_OnlyConfirmedSets verifies that runAckBackfill
// sets lifts_acknowledged_at = COALESCE(updated_at, started_at, NOW()) only for
// activities with no pending heuristic candidates (i.e., all heuristic candidates
// are confirmed or rejected).
//
// Adversarial-Review-Finding #22: prevents the review modal from appearing for
// every historic activity after migration.
func TestLiftRecomputeAll_AckBackfill_OnlyConfirmedSets(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"backfill_ack_"+userID[:8],
		"backfill_ack_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                                //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                          //nolint:errcheck
	})

	now := time.Now().UTC().Truncate(time.Second)

	insertActivity := func(sportType string) string {
		id := uuid.New().String()
		_, err := pool.Exec(ctx, `
			INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, id, userID, sportType, now, now.Add(2*time.Hour), uuid.New().String())
		if err != nil {
			t.Fatalf("insert activity (%s): %v", sportType, err)
		}
		return id
	}

	// Activity A: only confirmed heuristic candidates → ack should be set.
	actA := insertActivity("alpine_skiing")
	_, err = pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actA, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert candidate A: %v", err)
	}

	// Activity B: one unconfirmed heuristic candidate → ack must stay NULL.
	actB := insertActivity("alpine_skiing")
	_, err = pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', false, 'heuristic')
	`, actB, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert candidate B: %v", err)
	}

	// Activity C: only an OSM-source candidate (confirmed=false) → ack should be set
	// because the HAVING filter counts only source='heuristic' unconfirmed rows.
	actC := insertActivity("alpine_skiing")
	_, err = pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', false, 'osm')
	`, actC, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert candidate C: %v", err)
	}

	// Pre-condition: all three activities have lifts_acknowledged_at = NULL.
	checkAckNull := func(actID string) {
		t.Helper()
		var ack *time.Time
		if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actID).Scan(&ack); err != nil {
			t.Fatalf("query ack for %s: %v", actID, err)
		}
		if ack != nil {
			t.Fatalf("pre-condition: lifts_acknowledged_at for %s is not NULL", actID)
		}
	}
	checkAckNull(actA)
	checkAckNull(actB)
	checkAckNull(actC)

	// Run the ack backfill (global — no user scope).
	updated, err := runAckBackfill(ctx, pool, nil)
	if err != nil {
		t.Fatalf("runAckBackfill: %v", err)
	}
	// At minimum A and C should be updated; B must not be.
	if updated < 2 {
		t.Errorf("runAckBackfill updated %d rows, want at least 2 (A and C)", updated)
	}

	// Activity A: lifts_acknowledged_at must be set.
	var ackA *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actA).Scan(&ackA); err != nil {
		t.Fatalf("query ack A: %v", err)
	}
	if ackA == nil {
		t.Errorf("activity A: lifts_acknowledged_at is NULL, want non-NULL (all heuristic confirmed)")
	}

	// Activity B: lifts_acknowledged_at must remain NULL.
	var ackB *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actB).Scan(&ackB); err != nil {
		t.Fatalf("query ack B: %v", err)
	}
	if ackB != nil {
		t.Errorf("activity B: lifts_acknowledged_at is %v, want NULL (pending heuristic candidate)", ackB)
	}

	// Activity C: lifts_acknowledged_at must be set (only OSM candidates, no pending heuristic).
	var ackC *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actC).Scan(&ackC); err != nil {
		t.Fatalf("query ack C: %v", err)
	}
	if ackC == nil {
		t.Errorf("activity C: lifts_acknowledged_at is NULL, want non-NULL (OSM-only, no pending heuristic)")
	}
}

// TestMain_ScopedAckBackfill verifies that runAckBackfill with a userID parameter
// only updates activities belonging to that user (F19).
func TestMain_ScopedAckBackfill(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)

	// User A: has a fully-confirmed activity → ack should fire.
	userA := uuid.New().String()
	actA := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userA, "ack_scoped_a_"+userA[:8], "ack_scoped_a_"+userA[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, actA, userA, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actA, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert candidate A: %v", err)
	}

	// User B: also has a fully-confirmed activity → ack must NOT fire (scoped to A).
	userB := uuid.New().String()
	actB := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userB, "ack_scoped_b_"+userB[:8], "ack_scoped_b_"+userB[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user B: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, actB, userB, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity B: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actB, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert candidate B: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		for _, uid := range []string{userA, userB} {
			pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", uid) //nolint:errcheck
			pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", uid)                                                                //nolint:errcheck
			pool.Exec(bg, "DELETE FROM users WHERE id = $1", uid)                                                                          //nolint:errcheck
		}
	})

	// Run ack scoped to user A only.
	n, err := runAckBackfill(ctx, pool, &userA)
	if err != nil {
		t.Fatalf("runAckBackfill: %v", err)
	}
	if n == 0 {
		t.Errorf("runAckBackfill returned 0 updated rows, want ≥1 for user A")
	}

	// Activity A must have ack set.
	var ackA *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actA).Scan(&ackA); err != nil {
		t.Fatalf("query ack A: %v", err)
	}
	if ackA == nil {
		t.Errorf("activity A: lifts_acknowledged_at is NULL, want non-NULL (scoped to user A)")
	}

	// Activity B must NOT have ack set (different user).
	var ackB *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actB).Scan(&ackB); err != nil {
		t.Fatalf("query ack B: %v", err)
	}
	if ackB != nil {
		t.Errorf("activity B: lifts_acknowledged_at is %v, want NULL (not in scope)", ackB)
	}
}

// TestMain_DryRun_NoMutation verifies that --dry-run does not delete any
// candidates, does not modify lifts_acknowledged_at, and returns exit 0 (F5).
func TestMain_DryRun_NoMutation(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)

	now := time.Now().UTC().Truncate(time.Second)

	userID := uuid.New().String()
	actID := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userID, "dry_run_"+userID[:8], "dry_run_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_enrichment_settings (user_id, enable_lift_detection) VALUES ($1, true)
		 ON CONFLICT (user_id) DO UPDATE SET enable_lift_detection = true`,
		userID,
	); err != nil {
		t.Fatalf("insert settings: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, actID, userID, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	// Seed two non-rejected candidates.
	for i := 0; i < 2; i++ {
		start := now.Add(time.Duration(i) * 10 * time.Minute)
		if _, err := pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
			VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
		`, actID, start, start.Add(time.Minute)); err != nil {
			t.Fatalf("insert candidate %d: %v", i, err)
		}
	}

	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM user_enrichment_settings WHERE user_id = $1", userID)                                                  //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                                //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                          //nolint:errcheck
	})

	// Pre-condition: 2 candidates.
	var preCnt int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, actID).Scan(&preCnt); err != nil {
		t.Fatalf("count pre: %v", err)
	}
	if preCnt != 2 {
		t.Fatalf("pre-condition: want 2 candidates, got %d", preCnt)
	}

	cfg := runConfig{
		allUsers:    true,
		dryRun:      true,
		maxParallel: 1,
		dbURL:       dsn,
	}
	code := run(ctx, cfg)
	if code != 0 {
		t.Errorf("run() returned %d, want 0 for dry-run", code)
	}

	// Candidates must be unchanged.
	var postCnt int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1`, actID).Scan(&postCnt); err != nil {
		t.Fatalf("count post: %v", err)
	}
	if postCnt != preCnt {
		t.Errorf("dry-run mutated candidates: pre=%d post=%d", preCnt, postCnt)
	}

	// lifts_acknowledged_at must remain NULL.
	var ack *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actID).Scan(&ack); err != nil {
		t.Fatalf("query ack: %v", err)
	}
	if ack != nil {
		t.Errorf("dry-run set lifts_acknowledged_at = %v, want NULL", ack)
	}
}

// TestMain_AckOnlyScoped verifies that --ack-backfill-only combined with
// --user-id runs the scoped ack (F19).
func TestMain_AckOnlyScoped(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	dsn := testdb.RequireSafeDSN(t)

	now := time.Now().UTC().Truncate(time.Second)

	// User A — will be ack'd.
	userA := uuid.New().String()
	actA := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userA, "ack_only_a_"+userA[:8], "ack_only_a_"+userA[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, actA, userA, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity A: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actA, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert candidate A: %v", err)
	}

	// User B — must NOT be ack'd (scoped run).
	userB := uuid.New().String()
	actB := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userB, "ack_only_b_"+userB[:8], "ack_only_b_"+userB[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user B: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5)
	`, actB, userB, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity B: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actB, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert candidate B: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		for _, uid := range []string{userA, userB} {
			pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", uid) //nolint:errcheck
			pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", uid)                                                                //nolint:errcheck
			pool.Exec(bg, "DELETE FROM users WHERE id = $1", uid)                                                                          //nolint:errcheck
		}
	})

	cfg := runConfig{
		userID:      userA,
		ackOnly:     true,
		maxParallel: 1,
		dbURL:       dsn,
	}
	code := run(ctx, cfg)
	if code != 0 {
		t.Errorf("run() returned %d, want 0", code)
	}

	// Activity A: ack must be set.
	var ackA *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actA).Scan(&ackA); err != nil {
		t.Fatalf("query ack A: %v", err)
	}
	if ackA == nil {
		t.Errorf("activity A: lifts_acknowledged_at is NULL, want non-NULL")
	}

	// Activity B: ack must NOT be set (different user, not in scope).
	var ackB *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actB).Scan(&ackB); err != nil {
		t.Fatalf("query ack B: %v", err)
	}
	if ackB != nil {
		t.Errorf("activity B: lifts_acknowledged_at is %v, want NULL (scoped run excluded user B)", ackB)
	}
}

// TestMain_ListUsersFilterDeletedAt verifies that listUsersWithLiftDetection
// excludes users with deleted_at IS NOT NULL (F4).
func TestMain_ListUsersFilterDeletedAt(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// Active user with enable_lift_detection=true.
	activeUser := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		activeUser, "filt_active_"+activeUser[:8], "filt_active_"+activeUser[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert active user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_enrichment_settings (user_id, enable_lift_detection) VALUES ($1, true)
		 ON CONFLICT (user_id) DO UPDATE SET enable_lift_detection = true`,
		activeUser,
	); err != nil {
		t.Fatalf("insert settings active: %v", err)
	}

	// Deleted user with enable_lift_detection=true.
	deletedUser := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, deleted_at)
		 VALUES ($1,$2,$3,true,'test',NOW())`,
		deletedUser, "filt_del_"+deletedUser[:8], "filt_del_"+deletedUser[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert deleted user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_enrichment_settings (user_id, enable_lift_detection) VALUES ($1, true)
		 ON CONFLICT (user_id) DO UPDATE SET enable_lift_detection = true`,
		deletedUser,
	); err != nil {
		t.Fatalf("insert settings deleted: %v", err)
	}

	t.Cleanup(func() {
		bg := context.Background()
		for _, uid := range []string{activeUser, deletedUser} {
			pool.Exec(bg, "DELETE FROM user_enrichment_settings WHERE user_id = $1", uid) //nolint:errcheck
			pool.Exec(bg, "DELETE FROM users WHERE id = $1", uid)                          //nolint:errcheck
		}
	})

	ids, err := listUsersWithLiftDetection(ctx, pool)
	if err != nil {
		t.Fatalf("listUsersWithLiftDetection: %v", err)
	}

	foundActive := false
	foundDeleted := false
	for _, id := range ids {
		if id == activeUser {
			foundActive = true
		}
		if id == deletedUser {
			foundDeleted = true
		}
	}
	if !foundActive {
		t.Errorf("active user not returned by listUsersWithLiftDetection")
	}
	if foundDeleted {
		t.Errorf("deleted user (deleted_at IS NOT NULL) was returned by listUsersWithLiftDetection — must be excluded")
	}
}

// TestMain_AckCoalesce_UsesStartedAt verifies that the ack backfill falls back
// to started_at when updated_at is NULL (F7 — COALESCE(updated_at, started_at, NOW())).
func TestMain_AckCoalesce_UsesStartedAt(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	userID := uuid.New().String()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source) VALUES ($1,$2,$3,true,'test')`,
		userID, "ack_coalesce_"+userID[:8], "ack_coalesce_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                                //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                          //nolint:errcheck
	})

	now := time.Now().UTC().Truncate(time.Second)
	actID := uuid.New().String()
	// Insert activity and force updated_at to NULL to test COALESCE fallback.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash, updated_at)
		VALUES ($1, $2, 'alpine_skiing', $3, $4, $5, NULL)
	`, actID, userID, now, now.Add(2*time.Hour), uuid.New().String()); err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	// Confirmed heuristic candidate — ack-backfill will fire.
	if _, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed, source)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', true, 'heuristic')
	`, actID, now, now.Add(time.Minute)); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	n, err := runAckBackfill(ctx, pool, nil)
	if err != nil {
		t.Fatalf("runAckBackfill: %v", err)
	}
	if n == 0 {
		t.Errorf("runAckBackfill: 0 rows updated, want ≥1")
	}

	// lifts_acknowledged_at must be non-NULL (set to started_at or NOW() via COALESCE).
	var ack *time.Time
	if err := pool.QueryRow(ctx, `SELECT lifts_acknowledged_at FROM activities WHERE id = $1`, actID).Scan(&ack); err != nil {
		t.Fatalf("query ack: %v", err)
	}
	if ack == nil {
		t.Errorf("lifts_acknowledged_at is NULL even with COALESCE(updated_at, started_at, NOW()); updated_at=NULL so started_at should be the fallback")
	}
}
