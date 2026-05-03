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
