package enrichment

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// liftTestPool opens a pgxpool from TEST_DATABASE_URL; skips if not set.
func liftTestPool(t *testing.T) *pgxpool.Pool {
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

// liftTestUser inserts a minimal user row and registers cleanup.
func liftTestUser(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx := context.Background()
	userID := uuid.New().String()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"lift_test_"+userID[:8],
		"lift_test_"+userID[:8]+"@example.com",
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		bg := context.Background()
		pool.Exec(bg, "DELETE FROM activity_lift_candidates WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID) //nolint:errcheck
		pool.Exec(bg, "DELETE FROM segment_efforts WHERE user_id = $1", userID)                                                          //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_streams WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID)        //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activity_tracks WHERE activity_id IN (SELECT id FROM activities WHERE user_id = $1)", userID)         //nolint:errcheck
		pool.Exec(bg, "DELETE FROM activities WHERE user_id = $1", userID)                                                               //nolint:errcheck
		pool.Exec(bg, "DELETE FROM users WHERE id = $1", userID)                                                                         //nolint:errcheck
	})
	return userID
}

// liftTestActivity inserts a minimal hiking activity and registers cleanup.
func liftTestActivity(t *testing.T, pool *pgxpool.Pool, userID string) string {
	t.Helper()
	ctx := context.Background()
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)
	_, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'hiking', $3, $4, $5)
	`, activityID, userID, now, now.Add(2*time.Hour), uuid.New().String())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

// TestEnrichStreamLifts_NoOSM verifies that enrichStreamLifts returns nil and
// performs no INSERT when there is no OSM lift data (common in test environments).
func TestEnrichStreamLifts_NoOSM(t *testing.T) {
	pool := liftTestPool(t)

	// Skip if there IS osm_lifts data — this test is specifically for the empty case.
	var hasOSM bool
	if err := pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM osm_lifts LIMIT 1)").Scan(&hasOSM); err != nil {
		t.Skipf("cannot check osm_lifts: %v", err)
	}
	if hasOSM {
		t.Skip("osm_lifts has data — skipping no-OSM path test")
	}

	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	// Insert a minimal track so the track-exists check passes.
	_, err := pool.Exec(ctx, `
		INSERT INTO activity_tracks (activity_id, track_full, point_count)
		VALUES ($1, ST_GeomFromText('LINESTRING(11.0 47.0, 11.01 47.01)', 4326), 2)
	`, activityID)
	if err != nil {
		t.Fatalf("insert track: %v", err)
	}

	db := &database.DB{Pool: pool}

	stream := []EnrichmentStreamPoint{}
	if err := enrichStreamLifts(ctx, db, userID, activityID, stream); err != nil {
		t.Errorf("enrichStreamLifts: got error %v, want nil", err)
	}

	// Verify no lift candidates were inserted.
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1", activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count candidates: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 lift candidates, got %d", count)
	}
}

// TestEnrichStreamLifts_HeuristicCandidateHasSourceHeuristic asserts the
// Phase 4c contract: a heuristic candidate inserted via the integrated
// pipeline carries source='heuristic' and segment_effort_id IS NULL. The
// test bypasses the heuristic detector path (which needs a lengthy
// realistic stream) by inserting a candidate via the same SQL the adapter
// uses. The unit-level coverage of the snap pipeline lives in snap_test.go;
// the integration test here only verifies the column wiring + default.
func TestEnrichStreamLifts_HeuristicCandidateHasSourceHeuristic(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := liftTestActivity(t, pool, userID)

	now := time.Now().UTC().Truncate(time.Second)
	_, err := pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m, confidence, confirmed)
		VALUES ($1, $2, $3, 60, 100, 0, 600, 'high', false)
	`, activityID, now, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("insert candidate: %v", err)
	}

	var source string
	var segEffortID *string
	if err := pool.QueryRow(ctx, `
		SELECT source, segment_effort_id
		FROM activity_lift_candidates WHERE activity_id = $1
	`, activityID).Scan(&source, &segEffortID); err != nil {
		t.Fatalf("query: %v", err)
	}
	if source != "heuristic" {
		t.Errorf("source = %q, want 'heuristic' (default value contract)", source)
	}
	if segEffortID != nil {
		t.Errorf("segment_effort_id = %q, want NULL (default contract)", *segEffortID)
	}
}

// TestEnrichStreamLifts_IrrelevantSport verifies that a non-lift sport (running)
// causes an early return without any DB writes.
func TestEnrichStreamLifts_IrrelevantSport(t *testing.T) {
	pool := liftTestPool(t)
	ctx := context.Background()
	userID := liftTestUser(t, pool)
	activityID := uuid.New().String()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := pool.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at, ended_at, file_hash)
		VALUES ($1, $2, 'running', $3, $4, $5)
	`, activityID, userID, now, now.Add(30*time.Minute), uuid.New().String())
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM activities WHERE id = $1", activityID) //nolint:errcheck
	})

	db := &database.DB{Pool: pool}
	if err := enrichStreamLifts(ctx, db, userID, activityID, nil); err != nil {
		t.Errorf("expected nil error for irrelevant sport, got %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM activity_lift_candidates WHERE activity_id = $1", activityID,
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 candidates, got %d", count)
	}
}
