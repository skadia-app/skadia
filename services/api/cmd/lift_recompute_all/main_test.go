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
// sets lifts_acknowledged_at = updated_at only for activities with no pending
// heuristic candidates (i.e., all heuristic candidates are confirmed or rejected).
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

	// Run the ack backfill.
	updated, err := runAckBackfill(ctx, pool)
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
