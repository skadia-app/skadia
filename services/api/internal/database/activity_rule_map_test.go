package database

import (
	"context"
	"fmt"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
	"github.com/jackc/pgx/v5/pgxpool"
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

func TestGetActivityAsMapCountsOnlyCountryBordersForRuleConditions(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
		SportType: "cycling",
		StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("InsertActivity: %v", err)
	}

	for _, border := range []struct {
		name       string
		adminLevel int
	}{
		{"Bundeslandgrenze", 4},
		{"Landesgrenze", 2},
	} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO activity_geo_tags (activity_id, category, name, admin_level)
			VALUES ($1, 'border', $2, $3)
		`, act.ID, border.name, border.adminLevel); err != nil {
			t.Fatalf("insert border %s: %v", border.name, err)
		}
	}

	m, err := db.GetActivityAsMap(ctx, act.ID)
	if err != nil {
		t.Fatalf("GetActivityAsMap: %v", err)
	}
	if got, _ := m["border_crossing_count"].(int); got != 1 {
		t.Fatalf("border_crossing_count = %v, want 1 country border only", m["border_crossing_count"])
	}
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

// Property-style coverage of every plausible activity_climbs.category value.
// The bug class — text sentinels (HC, NULL, "") routed through a numeric
// aggregate cast — generalises beyond HC. Future additions to classifyClimb
// (services/api/internal/enrichment/ascents.go) must keep GetActivityAsMap
// crash-free for any value the enrichment layer may emit.
func TestGetActivityAsMapClimbCategoryValuesNeverCrash(t *testing.T) {
	db, userID := activitiesParentTestDB(t)
	ctx := context.Background()

	newAct := func(t *testing.T) string {
		t.Helper()
		act, err := db.InsertActivity(ctx, userID, &model.ActivityData{
			SportType: "cycling",
			SubType:   "mountain",
			StartedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("InsertActivity: %v", err)
		}
		return act.ID
	}

	insertClimb := func(t *testing.T, actID string, num int, cat any) {
		t.Helper()
		if _, err := db.Pool.Exec(ctx,
			`INSERT INTO activity_climbs (activity_id, climb_number, category) VALUES ($1, $2, $3)`,
			actID, num, cat); err != nil {
			t.Fatalf("insert climb (cat=%v): %v", cat, err)
		}
	}

	cases := []struct {
		name string
		cats []any
	}{
		{"single HC", []any{"HC"}},
		{"single numeric 1", []any{"1"}},
		{"single numeric 4", []any{"4"}},
		{"mixed HC + 2 + 4", []any{"HC", "2", "4"}},
		{"multiple HC only", []any{"HC", "HC"}},
		{"empty string only", []any{""}},
		{"NULL only", []any{nil}},
		{"HC + NULL + 3", []any{"HC", nil, "3"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actID := newAct(t)
			for i, c := range tc.cats {
				insertClimb(t, actID, i+1, c)
			}
			if _, err := db.GetActivityAsMap(ctx, actID); err != nil {
				t.Fatalf("GetActivityAsMap (cats=%v): %v", tc.cats, err)
			}
		})
	}
}

// Smoke-test against real activity data: GetActivityAsMap stitches together
// six LATERAL JOINs over user-derived data (climbs, weather, geo_tags, stops,
// intervals, surface, …). Each carries its own data-shape risk that no
// unit test can enumerate ahead of time. Iterating real rows catches the
// next bug of the same class as the HC crash without anyone having to
// guess the failure mode in advance.
//
// Capped at 5000 rows (most recent first) so the test stays under a few
// minutes on a populated dev DB; raise the cap or drop the LIMIT entirely
// once we want stronger guarantees.
func TestGetActivityAsMapSmokeAgainstLiveData(t *testing.T) {
	dsn := testdb.RequireSafeDSN(t)
	if testing.Short() {
		t.Skip("smoke test skipped in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	defer pool.Close()
	db := &DB{Pool: pool}

	const limit = 5000
	rows, err := pool.Query(ctx,
		`SELECT id::text FROM activities ORDER BY started_at DESC NULLS LAST LIMIT $1`, limit)
	if err != nil {
		t.Fatalf("list activities: %v", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}
	if len(ids) == 0 {
		t.Skip("no activities present in test DB")
	}

	type failure struct {
		id  string
		err error
	}
	var failures []failure
	for _, id := range ids {
		if _, err := db.GetActivityAsMap(ctx, id); err != nil {
			failures = append(failures, failure{id, err})
		}
	}
	if len(failures) > 0 {
		msg := fmt.Sprintf("GetActivityAsMap failed for %d/%d activities", len(failures), len(ids))
		shown := len(failures)
		if shown > 10 {
			shown = 10
		}
		for _, f := range failures[:shown] {
			msg += fmt.Sprintf("\n  %s: %v", f.id, f.err)
		}
		t.Fatal(msg)
	}
	t.Logf("GetActivityAsMap clean for %d activities", len(ids))
}
