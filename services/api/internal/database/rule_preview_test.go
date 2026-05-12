package database

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestLoadRuleForPreviewReadonlyRLS(t *testing.T) {
	db, userA := chatPrefsTestDB(t)
	_, userB := chatPrefsTestDB(t)
	ctx := context.Background()

	rule, err := db.CreateRule(ctx, userA, &model.AutomationRuleCreate{
		Name:       "Preview Rule",
		Conditions: []byte(`[]`),
		Actions:    []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	readonly := agentAuditReadonlyPool(t)
	got, err := LoadRuleForPreview(ctx, readonly, userA, rule.ID)
	if err != nil {
		t.Fatalf("LoadRuleForPreview userA: %v", err)
	}
	if got == nil || got.ID != rule.ID {
		t.Fatalf("preview rule = %#v, want %s", got, rule.ID)
	}

	got, err = LoadRuleForPreview(ctx, readonly, userB, rule.ID)
	if err != nil {
		t.Fatalf("LoadRuleForPreview userB: %v", err)
	}
	if got != nil {
		t.Fatalf("foreign rule visible: %#v", got)
	}
}

func TestAutomationRulesReadonlyRequiresGUCAndCannotWrite(t *testing.T) {
	db, userID := chatPrefsTestDB(t)
	ctx := context.Background()
	rule, err := db.CreateRule(ctx, userID, &model.AutomationRuleCreate{
		Name:       "Readonly Rule",
		Conditions: []byte(`[]`),
		Actions:    []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}

	readonly := agentAuditReadonlyPool(t)
	var count int
	err = readonly.QueryRow(ctx, `SELECT count(*) FROM automation_rules`).Scan(&count)
	if err == nil && count != 0 {
		t.Fatalf("readonly without GUC visible rows = %d, want 0 or error", count)
	}

	if _, err := readonly.Exec(ctx, "SELECT set_config('app.current_user_id', $1, false)", userID); err != nil {
		t.Fatalf("set readonly RLS context: %v", err)
	}
	if err := readonly.QueryRow(ctx, `SELECT count(*) FROM automation_rules WHERE id = $1`, rule.ID).Scan(&count); err != nil {
		t.Fatalf("readonly select: %v", err)
	}
	if count != 1 {
		t.Fatalf("readonly visible rows = %d, want 1", count)
	}
	if _, err := readonly.Exec(ctx, `UPDATE automation_rules SET name = 'forged' WHERE id = $1`, rule.ID); err == nil {
		t.Fatal("readonly UPDATE succeeded, want permission/RLS failure")
	}
}

func TestAutomationRuleAPIsWorkWithForcedRLS(t *testing.T) {
	db, userID := chatPrefsTestDB(t)
	ctx := context.Background()

	rule, err := db.CreateRule(ctx, userID, &model.AutomationRuleCreate{
		Name:       "RLS API Rule",
		Conditions: []byte(`[]`),
		Actions:    []byte(`[]`),
	})
	if err != nil {
		t.Fatalf("CreateRule: %v", err)
	}
	rules, err := db.ListRules(ctx, userID)
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 || rules[0].ID != rule.ID {
		t.Fatalf("ListRules = %#v, want rule %s", rules, rule.ID)
	}

	name := "RLS API Rule Updated"
	updated, err := db.UpdateRule(ctx, rule.ID, userID, &model.AutomationRuleUpdate{Name: &name})
	if err != nil {
		t.Fatalf("UpdateRule: %v", err)
	}
	if updated == nil || updated.Name != name {
		t.Fatalf("updated rule = %#v", updated)
	}
	if err := db.IncrementRuleApplied(ctx, rule.ID, userID); err != nil {
		t.Fatalf("IncrementRuleApplied: %v", err)
	}
	count := 2
	if err := db.SetRuleEvaluation(ctx, rule.ID, userID, &count); err != nil {
		t.Fatalf("SetRuleEvaluation: %v", err)
	}
	if err := db.DeleteRule(ctx, rule.ID, userID); err != nil {
		t.Fatalf("DeleteRule: %v", err)
	}
}
