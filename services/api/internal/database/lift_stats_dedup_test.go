package database_test

// Phase 4c.4 — recomputeLiftStats dedup tests.
//
// The pipeline emits two parallel rows for every snapped ride: one in
// segment_efforts (the count-of-truth for activities.lift_*) and one
// audit-only row in activity_lift_candidates with segment_effort_id set.
// Without dedup the recompute would double-count the audit row when its
// confirmed flag is true. The fix: the heuristic-only sub-query filters
// `WHERE confirmed = true AND segment_effort_id IS NULL`.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

func liftDedupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testdb.RequireSafeDSN(t)
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type liftDedupFixture struct {
	pool       *pgxpool.Pool
	userID     string
	activityID string
	segmentID  string
	versionID  string
	cleanup    func()
}

func newLiftDedupFixture(t *testing.T) liftDedupFixture {
	t.Helper()
	pool := liftDedupPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "liftdedup_"+userID[:8], "liftdedup_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("user: %v", err)
	}

	activityID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'skiing', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.NewString()); err != nil {
		t.Fatalf("activity: %v", err)
	}

	segmentID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segments
			(id, name, segment_type, distance_m, elevation_gain_m, sport_group, created_by)
		VALUES ($1, 'Lift Dedup Fixture', 'lift', 600, 100, 'skiing', $2)
	`, segmentID, userID); err != nil {
		t.Fatalf("segment: %v", err)
	}

	versionID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO segment_versions
			(id, segment_id, version_number, track, valid_from, created_by)
		VALUES ($1, $2, 1,
		        ST_GeomFromText('LINESTRING(11.0 47.0, 11.005 47.0)', 4326),
		        CURRENT_DATE, $3)
	`, versionID, segmentID, userID); err != nil {
		t.Fatalf("segment_version: %v", err)
	}

	cleanup := func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id = $1", activityID)
		pool.Exec(bg, "DELETE FROM segment_efforts WHERE activity_id = $1", activityID)
		pool.Exec(bg, "DELETE FROM segment_versions WHERE id = $1", versionID)
		pool.Exec(bg, "DELETE FROM segments WHERE id = $1", segmentID)
		pool.Exec(bg, "DELETE FROM activities WHERE id = $1", activityID)
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)
	}
	t.Cleanup(cleanup)

	return liftDedupFixture{pool, userID, activityID, segmentID, versionID, cleanup}
}

// TestRecomputeLiftStats_SnapDoesNotDoubleCount: a Stage-1/2 snap produces
// 1 segment_efforts row + 1 audit candidate (segment_effort_id set,
// confirmed=true). The recompute must count the snap exactly once — NOT
// once for the segment_effort plus once for the candidate.
func TestRecomputeLiftStats_SnapDoesNotDoubleCount(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	effortID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard, is_lift_segment_match)
		VALUES ($1, $2, $3, $4, $5, $6, 300, 600, 100, false,
		        'skiing', 'generic', false, true)
	`, effortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, now); err != nil {
		t.Fatalf("segment_effort: %v", err)
	}
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, segment_effort_id)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', $4)
	`, fx.activityID, now, now.Add(5*time.Minute), effortID); err != nil {
		t.Fatalf("audit candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var liftCount int
	var liftDur, liftAsc, liftDist float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT lift_count, lift_duration_s, lift_ascent_m, lift_distance_m
		FROM activities WHERE id = $1
	`, fx.activityID).Scan(&liftCount, &liftDur, &liftAsc, &liftDist); err != nil {
		t.Fatalf("query stats: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("lift_count = %d, want 1 (snap must dedup)", liftCount)
	}
	if liftDur != 300 {
		t.Errorf("lift_duration_s = %.0f, want 300", liftDur)
	}
	if liftAsc != 100 {
		t.Errorf("lift_ascent_m = %.0f, want 100", liftAsc)
	}
	if liftDist != 600 {
		t.Errorf("lift_distance_m = %.0f, want 600", liftDist)
	}
}

// TestRecomputeLiftStats_HeuristicOnlyConfirmed: a heuristic candidate
// (segment_effort_id IS NULL) is confirmed by the user → counts once, no
// segment_effort exists.
func TestRecomputeLiftStats_HeuristicOnlyConfirmed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		VALUES ($1, $2, $3, 200, 80, 0, 500, 'medium', true, 'heuristic')
	`, fx.activityID, now, now.Add(200*time.Second)); err != nil {
		t.Fatalf("heuristic candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	var liftCount int
	var liftDur, liftAsc, liftDist float64
	if err := fx.pool.QueryRow(ctx, `
		SELECT lift_count, lift_duration_s, lift_ascent_m, lift_distance_m
		FROM activities WHERE id = $1
	`, fx.activityID).Scan(&liftCount, &liftDur, &liftAsc, &liftDist); err != nil {
		t.Fatalf("query stats: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("lift_count = %d, want 1", liftCount)
	}
	if liftDur != 200 {
		t.Errorf("lift_duration_s = %.0f, want 200", liftDur)
	}
	if liftAsc != 80 {
		t.Errorf("lift_ascent_m = %.0f, want 80", liftAsc)
	}
	if liftDist != 500 {
		t.Errorf("lift_distance_m = %.0f, want 500", liftDist)
	}
}

// TestRecomputeLiftStats_HeuristicOnlyUnconfirmedDoesNotCount: the
// confirm=false case must not count.
func TestRecomputeLiftStats_HeuristicOnlyUnconfirmedDoesNotCount(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		VALUES ($1, $2, $3, 200, 80, 0, 500, 'medium', false, 'heuristic')
	`, fx.activityID, now, now.Add(200*time.Second)); err != nil {
		t.Fatalf("heuristic candidate: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var liftCount int
	if err := fx.pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", fx.activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 0 {
		t.Errorf("lift_count = %d, want 0 (unconfirmed heuristic must not count)", liftCount)
	}
}

// TestRecomputeLiftStats_CableHoldMergedRideCountsOnce: a Stage-2 cable-hold
// merge produces 1 segment_efforts canonical + N audit candidates (all with
// segment_effort_id set, all confirmed=true). The dedup must count them as
// ONE lift, not 1+N.
func TestRecomputeLiftStats_CableHoldMergedRideCountsOnce(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	effortID := uuid.NewString()
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO segment_efforts
			(id, activity_id, segment_id, segment_version_id, user_id,
			 started_at, elapsed_time_s, distance_m, elevation_gain_m,
			 is_reverse, sport_type, sub_type, in_leaderboard,
			 is_lift_segment_match, is_cable_hold_merge)
		VALUES ($1, $2, $3, $4, $5, $6, 600, 600, 200, false,
		        'skiing', 'generic', false, true, true)
	`, effortID, fx.activityID, fx.segmentID, fx.versionID, fx.userID, now); err != nil {
		t.Fatalf("segment_effort: %v", err)
	}
	for i := 0; i < 3; i++ {
		started := now.Add(time.Duration(i*200) * time.Second)
		ended := started.Add(150 * time.Second)
		if _, err := fx.pool.Exec(ctx, `
			INSERT INTO activity_lift_candidates
				(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
				 confidence, confirmed, source, segment_effort_id)
			VALUES ($1, $2, $3, 150, 65, 0, 200, 'high', true, 'osm', $4)
		`, fx.activityID, started, ended, effortID); err != nil {
			t.Fatalf("audit candidate %d: %v", i, err)
		}
	}

	db := &database.DB{Pool: fx.pool}
	if err := db.RecomputeLiftStats(ctx, fx.activityID); err != nil {
		t.Fatalf("recompute: %v", err)
	}
	var liftCount int
	if err := fx.pool.QueryRow(ctx,
		"SELECT lift_count FROM activities WHERE id = $1", fx.activityID,
	).Scan(&liftCount); err != nil {
		t.Fatalf("query: %v", err)
	}
	if liftCount != 1 {
		t.Errorf("lift_count = %d, want 1 (3 merged audit candidates must dedup to 1 segment_effort)", liftCount)
	}
}
