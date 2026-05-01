package database

import (
	"context"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
)

func TestGetActivityAsMapIncludesBooleanRuleModifiers(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		SubType:   "mountain",
		StartedAt: time.Now().UTC(),
		IsVirtual: false,
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	m, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap false modifiers: %v", err)
	}
	assertMapBool(t, m, "is_virtual", false)
	assertMapBool(t, m, "is_commute", false)
	if !rules.EvaluateGroups([]rules.ConditionGroup{{
		Logic: "and",
		Conditions: []rules.Condition{
			{Field: "sport_type", Op: "eq", Value: "cycling"},
			{Field: "sub_type", Op: "eq", Value: "mountain"},
			{Field: "is_virtual", Op: "eq", Value: false},
			{Field: "is_commute", Op: "eq", Value: false},
		},
	}}, m) {
		t.Fatal("rule evaluation did not match false modifier conditions")
	}

	if _, err := db.Pool.Exec(ctx,
		`UPDATE activities SET is_virtual = true, is_commute = true WHERE id = $1`,
		act.ID); err != nil {
		t.Fatalf("update modifiers: %v", err)
	}

	m, err = db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap true modifiers: %v", err)
	}
	assertMapBool(t, m, "is_virtual", true)
	assertMapBool(t, m, "is_commute", true)
}

func assertMapBool(t *testing.T, m map[string]any, key string, want bool) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Fatalf("activity map missing %q", key)
	}
	gotBool, ok := got.(bool)
	if !ok {
		t.Fatalf("activity map %q = %T(%v), want bool", key, got, got)
	}
	if gotBool != want {
		t.Fatalf("activity map %q = %v, want %v", key, gotBool, want)
	}
}

// Regression: an activity whose only climb is classified 'HC' (Hors Catégorie)
// must not crash GetActivityAsMap. The previous SQL did MIN(category)::int,
// which returned 'HC' lexicographically and then failed with SQLSTATE 22P02.
// climb_category_max stays absent (no numeric category present), but
// has_hc_climb signals the HC presence to the rules engine.
func TestGetActivityAsMapHCOnlyClimbDoesNotCrash(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		SubType:   "mountain",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	if _, err := db.Pool.Exec(ctx,
		`INSERT INTO activity_climbs (activity_id, climb_number, category) VALUES ($1, 1, 'HC')`,
		act.ID); err != nil {
		t.Fatalf("insert HC climb: %v", err)
	}

	m, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap with HC-only climb: %v", err)
	}
	if got, _ := m["climb_count"].(int); got != 1 {
		t.Fatalf("climb_count = %v, want 1", m["climb_count"])
	}
	assertMapBool(t, m, "has_hc_climb", true)
	if v, present := m["climb_category_max"]; present {
		t.Fatalf("climb_category_max = %v, want absent (no numeric category)", v)
	}
}

// climb_category_max must reflect the hardest *numeric* category and ignore
// HC entries (the cycling convention treats the lowest number as hardest;
// HC is even harder but is exposed via the boolean has_hc_climb).
func TestGetActivityAsMapClimbCategoryMaxIgnoresHC(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		SubType:   "mountain",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	for _, c := range []struct {
		num int
		cat string
	}{{1, "HC"}, {2, "2"}, {3, "4"}} {
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO activity_climbs (activity_id, climb_number, category) VALUES ($1, $2, $3)`,
			act.ID, c.num, c.cat); err != nil {
			t.Fatalf("insert climb %d: %v", c.num, err)
		}
	}

	m, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap: %v", err)
	}
	if got, _ := m["climb_count"].(int); got != 3 {
		t.Fatalf("climb_count = %v, want 3", m["climb_count"])
	}
	got, ok := m["climb_category_max"].(int)
	if !ok {
		t.Fatalf("climb_category_max = %T(%v), want int", m["climb_category_max"], m["climb_category_max"])
	}
	if got != 2 {
		t.Fatalf("climb_category_max = %d, want 2 (hardest numeric, HC must be ignored)", got)
	}
	assertMapBool(t, m, "has_hc_climb", true)
}
