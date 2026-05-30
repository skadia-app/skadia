package database_test

// Phase-4 regression case— IsOSMRejectedInWindow.
//
// Unit-level coverage of the new DB helper that the OSM-snap-pipeline calls
// before inserting a fresh segment_effort + audit-row. Mirrors the
// boundary-delta predicate from Slot-2-F4 (|Δstarted| <= 5s AND
// |Δended| <= 5s) so OSM-reject and heuristic-reject use the same
// drift tolerance.

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
)

// TestIsOSMRejectedInWindow_ExactMatchSuppressed: a rejected OSM audit-row
// at [t, t+300s] suppresses a fresh insert at exactly the same window.
func TestIsOSMRejectedInWindow_ExactMatchSuppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', true)
	`, fx.activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, fx.activityID, now, now.Add(300*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if !suppressed {
		t.Error("suppressed = false, want true: exact-match window must report rejected")
	}
}

// TestIsOSMRejectedInWindow_DriftWithinTolerance_Suppressed: ±3s on both
// boundaries (within ±5s) → suppressed.
func TestIsOSMRejectedInWindow_DriftWithinTolerance_Suppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', true)
	`, fx.activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, fx.activityID,
		now.Add(3*time.Second), now.Add(303*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if !suppressed {
		t.Error("suppressed = false, want true: ±3s drift on both boundaries must be suppressed")
	}
}

// TestIsOSMRejectedInWindow_LargeDriftBeyondTolerance_NotSuppressed: ±30s
// drift far beyond ±5s on both ends → not suppressed (treated as a
// distinct lift detection).
func TestIsOSMRejectedInWindow_LargeDriftBeyondTolerance_NotSuppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm', true)
	`, fx.activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, fx.activityID,
		now.Add(30*time.Second), now.Add(330*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if suppressed {
		t.Error("suppressed = true, want false: ±30s drift exceeds ±5s tolerance — must be treated as distinct lift")
	}
}

// TestIsOSMRejectedInWindow_NonRejectedOSMRow_NotSuppressed: an OSM
// audit-row that is NOT rejected (the normal case) must not suppress.
// Without this gate the helper would short-circuit every re-enrichment.
func TestIsOSMRejectedInWindow_NonRejectedOSMRow_NotSuppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', true, 'osm')
	`, fx.activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert non-rejected OSM audit-row: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, fx.activityID, now, now.Add(300*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if suppressed {
		t.Error("suppressed = true, want false: non-rejected OSM row must not suppress fresh insert")
	}
}

// TestIsOSMRejectedInWindow_HeuristicRejectedRow_NotSuppressed: a rejected
// HEURISTIC row must not suppress the OSM-snap pipeline. The two sources
// have separate suppression namespaces so a user can reject a heuristic
// detection without preventing the same time-window from snapping to OSM
// later (e.g. after an OSM data-import that filled in missing lifts).
func TestIsOSMRejectedInWindow_HeuristicRejectedRow_NotSuppressed(t *testing.T) {
	fx := newLiftDedupFixture(t)
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := fx.pool.Exec(ctx, `
		INSERT INTO activity_lift_candidates
			(activity_id, started_at, ended_at, duration_s, ascent_m, descent_m, distance_m,
			 confidence, confirmed, source, rejected_by_user)
		VALUES ($1, $2, $3, 300, 100, 0, 600, 'high', false, 'heuristic', true)
	`, fx.activityID, now, now.Add(300*time.Second)); err != nil {
		t.Fatalf("insert rejected heuristic row: %v", err)
	}

	db := &database.DB{Pool: fx.pool}
	suppressed, err := db.IsOSMRejectedInWindow(ctx, fx.activityID, now, now.Add(300*time.Second))
	if err != nil {
		t.Fatalf("IsOSMRejectedInWindow: %v", err)
	}
	if suppressed {
		t.Error("suppressed = true, want false: rejected heuristic row must not suppress OSM-snap insert")
	}
}
