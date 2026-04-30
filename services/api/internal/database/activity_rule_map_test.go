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
