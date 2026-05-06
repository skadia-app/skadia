package database

// Phase 4b.B.2 — verify GetActivityAsMap exposes has_lift, derived from
// activities.lift_count > 0. The field powers the new "Lift erkannt" rule
// condition (services/api/internal/automation/system_artifacts.go).

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestGetActivityAsMap_HasLift(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "alpine_skiing",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	// Without lifts: has_lift must be false (rules engine relies on the
	// presence of the key, not just non-zero).
	m, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap (no lifts): %v", err)
	}
	got, ok := m["has_lift"].(bool)
	if !ok {
		t.Fatalf("activity map missing has_lift or wrong type: %T(%v)", m["has_lift"], m["has_lift"])
	}
	if got {
		t.Errorf("has_lift should be false when lift_count is 0/NULL, got true")
	}

	// Set lift_count > 0 directly. Bypasses recomputeLiftStats so the test
	// stays focused on the buildActivityMap derivation, not the recompute SQL.
	if _, err := db.Pool.Exec(ctx,
		`UPDATE activities SET lift_count = 5 WHERE id = $1`, act.ID,
	); err != nil {
		t.Fatalf("update lift_count: %v", err)
	}

	m2, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap (with lifts): %v", err)
	}
	got2, ok := m2["has_lift"].(bool)
	if !ok {
		t.Fatalf("activity map missing has_lift after update: %T(%v)", m2["has_lift"], m2["has_lift"])
	}
	if !got2 {
		t.Errorf("has_lift should be true when lift_count=5, got false")
	}
	// lift_count must also be exposed (already populated by setNN — this is
	// defensive coverage, not a new behaviour assertion).
	if v, present := m2["lift_count"]; !present {
		t.Errorf("activity map missing lift_count")
	} else if vi, ok := v.(int); !ok || vi != 5 {
		t.Errorf("lift_count = %T(%v), want int(5)", v, v)
	}
}
