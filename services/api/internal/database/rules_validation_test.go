package database

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/rules"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestValidateAutomationRuleJSONRejectsInvalidSportTuple(t *testing.T) {
	err := validateAutomationRuleJSON(rules.SportTaxonomyRuleJSON{
		Conditions: json.RawMessage(`[{"field":"sport_tuple","op":"in","value":[{"sport_type":"running","sub_type":"mountain"}]}]`),
	})
	if err == nil || !strings.Contains(err.Error(), "sub_type") {
		t.Fatalf("error = %v, want sub_type rejection", err)
	}
}

func TestReadonlySyncRuleTypeIncludesIndoor(t *testing.T) {
	for _, systemType := range []string{"electric_sync", "indoor_sync", "virtual_sync"} {
		t.Run(systemType, func(t *testing.T) {
			if !isReadonlySyncRuleType(&systemType) {
				t.Fatalf("%s should be read-only", systemType)
			}
		})
	}
	roundTrip := "round_trip"
	if isReadonlySyncRuleType(&roundTrip) {
		t.Fatal("round_trip must remain editable/backfillable")
	}
}

func TestCreateRuleRejectsInvalidSportTupleJSON(t *testing.T) {
	db, userID := ruleValidationDB(t)
	_, err := db.CreateRule(context.Background(), userID, &model.AutomationRuleCreate{
		Name:       "Invalid Tuple",
		Conditions: json.RawMessage(`[{"field":"sport_tuple","op":"in","value":[{"sport_type":"running","sub_type":"mountain"}]}]`),
		Actions:    json.RawMessage(`[]`),
	})
	if err == nil || !strings.Contains(err.Error(), "validate rule") || !strings.Contains(err.Error(), "sub_type") {
		t.Fatalf("error = %v, want validate rule sub_type rejection", err)
	}
}

func TestUpdateRuleRejectsPartialMixedSportTupleAndLegacyList(t *testing.T) {
	db, userID := ruleValidationDB(t)
	rule, err := db.CreateRule(context.Background(), userID, &model.AutomationRuleCreate{
		Name:       "Legacy",
		Conditions: json.RawMessage(`[{"field":"sport_type","op":"in","value":["running"]}]`),
		Actions:    json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	groups := json.RawMessage(`[{"logic":"and","conditions":[{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}]}]`)
	_, err = db.UpdateRule(context.Background(), rule.ID, userID, &model.AutomationRuleUpdate{ConditionGroups: &groups})
	if err == nil || !strings.Contains(err.Error(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("error = %v, want mixed-shape rejection", err)
	}
}

func TestUpdateRuleRejectsPartialMixedLegacyListAndSportTuple(t *testing.T) {
	db, userID := ruleValidationDB(t)
	rule, err := db.CreateRule(context.Background(), userID, &model.AutomationRuleCreate{
		Name: "Tuple",
		ConditionGroups: json.RawMessage(`[
			{"logic":"and","conditions":[{"field":"sport_tuple","op":"in","value":[{"sport_type":"cycling"}]}]}
		]`),
		Actions: json.RawMessage(`[]`),
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	conditions := json.RawMessage(`[{"field":"sport_type","op":"in","value":["running"]}]`)
	_, err = db.UpdateRule(context.Background(), rule.ID, userID, &model.AutomationRuleUpdate{Conditions: &conditions})
	if err == nil || !strings.Contains(err.Error(), "mixed sport_type in and sport_tuple in") {
		t.Fatalf("error = %v, want mixed-shape rejection", err)
	}
}

func TestUpdateRuleRejectsIndoorSyncRule(t *testing.T) {
	db, userID := ruleValidationDB(t)
	ctx := context.Background()
	var ruleID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions)
		VALUES ($1, 'Indoor', true, 'indoor_sync', true, -100, '[]'::jsonb, 'and', '[]'::jsonb)
		RETURNING id`, userID).Scan(&ruleID); err != nil {
		t.Fatalf("insert indoor sync rule: %v", err)
	}
	active := false
	_, err := db.UpdateRule(ctx, ruleID, userID, &model.AutomationRuleUpdate{IsActive: &active})
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("error = %v, want read-only rejection", err)
	}
}

func TestListRulesUsesSystemTagCountsForReadonlySyncRules(t *testing.T) {
	db, userID := ruleValidationDB(t)
	ctx := context.Background()

	for _, row := range []struct {
		name       string
		systemType string
		count      int
	}{
		{name: systemTagNameEBike, systemType: "electric_sync", count: 11},
		{name: systemTagNameIndoor, systemType: "indoor_sync", count: 22},
		{name: systemTagNameVirtual, systemType: "virtual_sync", count: 33},
	} {
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO tags (user_id, name, color, text_color, category, is_system, activity_count)
			VALUES ($1, $2, '#111111', '#ffffff', 'system', true, $3)
			ON CONFLICT (user_id, name) DO UPDATE SET is_system = true, activity_count = EXCLUDED.activity_count
		`, userID, row.name, row.count); err != nil {
			t.Fatalf("insert tag %s: %v", row.name, err)
		}
		if _, err := db.Pool.Exec(ctx, `
			INSERT INTO automation_rules (user_id, name, is_system, system_type, is_active, priority, conditions, conditions_logic, actions, times_applied)
			VALUES ($1, $2, true, $3, true, -100, '[]'::jsonb, 'and', '[]'::jsonb, 0)
		`, userID, row.name, row.systemType); err != nil {
			t.Fatalf("insert rule %s: %v", row.systemType, err)
		}
	}

	rules, err := db.ListRules(ctx, userID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	got := map[string]int{}
	for _, rule := range rules {
		if rule.SystemType != nil {
			got[*rule.SystemType] = rule.TimesApplied
		}
	}
	for systemType, want := range map[string]int{
		"electric_sync": 11,
		"indoor_sync":   22,
		"virtual_sync":  33,
	} {
		if got[systemType] != want {
			t.Fatalf("%s times_applied = %d, want %d (all counts: %#v)", systemType, got[systemType], want, got)
		}
	}
}

func ruleValidationDB(t *testing.T) (*DB, string) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID, "rule-validation-"+userID[:8], "rule-validation-"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	return &DB{Pool: pool}, userID
}
