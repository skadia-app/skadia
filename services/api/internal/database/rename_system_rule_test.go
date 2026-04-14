//go:build integration

package database

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestRenameSystemRule_OnlyTouchesSystemRule is the regression test for
// Finding #26: migrations 000298 and 000299 did a blind
//
//	UPDATE automation_rules SET name = '<new>' WHERE name = '<old>';
//
// clobbering manually-created user rules that happened to share the same name.
// RenameSystemRule() must filter on is_system=true so that a user-owned rule
// with the exact same name survives untouched.
func TestRenameSystemRule_OnlyTouchesSystemRule(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	username := fmt.Sprintf("rename-sysrule-%d", time.Now().UnixNano())

	var userID string
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'dev')
		 RETURNING id`,
		username, username+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	// Seed a system rule and a user-owned rule that share the initial name.
	// system_type must be one of the values whitelisted by
	// automation_rules_system_type_check. "round_trip" is used solely as a
	// slot here — the test does not depend on its semantics. Since the user is
	// freshly created and SeedRecommendedRules() has not run for this user,
	// the partial unique index on (user_id, system_type) WHERE is_system=true
	// will not conflict.
	var systemRuleID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO automation_rules
		    (user_id, name, is_system, system_type, is_active, priority,
		     conditions, conditions_logic, actions)
		VALUES ($1, 'Intervall-Training', true, 'round_trip', true, -50,
		        '[]'::jsonb, 'and', '[]'::jsonb)
		RETURNING id`,
		userID,
	).Scan(&systemRuleID); err != nil {
		t.Fatalf("seed system rule: %v", err)
	}

	var userRuleID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO automation_rules
		    (user_id, name, is_active, priority,
		     conditions, conditions_logic, actions)
		VALUES ($1, 'Intervall-Training', true, 10,
		        '[]'::jsonb, 'and', '[]'::jsonb)
		RETURNING id`,
		userID,
	).Scan(&userRuleID); err != nil {
		t.Fatalf("seed user rule: %v", err)
	}

	// Invoke the helper.
	affected, err := db.RenameSystemRule(ctx, userID, "Intervall-Training", "Intervalltraining")
	if err != nil {
		t.Fatalf("RenameSystemRule: %v", err)
	}
	if affected != 1 {
		t.Errorf("expected 1 row affected, got %d", affected)
	}

	// System rule: renamed.
	var sysName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM automation_rules WHERE id = $1`, systemRuleID,
	).Scan(&sysName); err != nil {
		t.Fatalf("read system rule: %v", err)
	}
	if sysName != "Intervalltraining" {
		t.Errorf("system rule name: want %q, got %q", "Intervalltraining", sysName)
	}

	// User rule: UNTOUCHED — this is the core invariant Finding #26 is about.
	var userName string
	if err := db.Pool.QueryRow(ctx,
		`SELECT name FROM automation_rules WHERE id = $1`, userRuleID,
	).Scan(&userName); err != nil {
		t.Fatalf("read user rule: %v", err)
	}
	if userName != "Intervall-Training" {
		t.Errorf("user rule was clobbered: want %q, got %q", "Intervall-Training", userName)
	}
}

// TestRenameSystemRule_NoMatchIsNotAnError verifies that calling the helper
// when no matching system rule exists returns (0, nil) — not an error. A
// caller that iterates through a rename list must not trip on a gap.
func TestRenameSystemRule_NoMatchIsNotAnError(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	username := fmt.Sprintf("rename-nomatch-%d", time.Now().UnixNano())
	var userID string
	if err := db.Pool.QueryRow(ctx,
		`INSERT INTO users (username, email, registration_source)
		 VALUES ($1, $2, 'dev')
		 RETURNING id`,
		username, username+"@test.localhost",
	).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	})

	affected, err := db.RenameSystemRule(ctx, userID, "does-not-exist", "neither-does-this")
	if err != nil {
		t.Fatalf("RenameSystemRule: %v", err)
	}
	if affected != 0 {
		t.Errorf("expected 0 rows affected, got %d", affected)
	}
}

// TestRenameSystemRule_RejectsEmptyNames guards against accidental wipeouts:
// passing an empty oldName or newName should be refused at the helper level
// rather than silently running an UPDATE with '' as a literal value.
func TestRenameSystemRule_RejectsEmptyNames(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := db.RenameSystemRule(ctx, "whatever", "", "new"); err == nil {
		t.Error("expected error for empty oldName")
	}
	if _, err := db.RenameSystemRule(ctx, "whatever", "old", ""); err == nil {
		t.Error("expected error for empty newName")
	}
}
