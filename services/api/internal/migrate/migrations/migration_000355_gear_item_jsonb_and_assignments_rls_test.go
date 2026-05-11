package migrations_test

// RLS and write-isolation tests for tables created by migration 000355.
//
// Asserts:
//   - skadia_readonly with one user's current_user_id cannot see the other user's rows
//   - each of the five new tables has exactly one policy (SELECT for skadia_readonly)
//   - activity_gear_assignments_user_match trigger rejects cross-user inserts
//   - gear_component_swap_events_user_match trigger rejects cross-user inserts

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestRLSPolicySelect_ActivityGearAssignments verifies that skadia_readonly
// connected as userB cannot read rows owned by userA.
func TestRLSPolicySelect_ActivityGearAssignments(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// Create two users and an activity + gear for userA.
	userA := uuid.NewString()
	userB := uuid.NewString()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	for _, u := range []struct{ id, name string }{
		{userA, "rls_a_" + userA[:8]},
		{userB, "rls_b_" + userB[:8]},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, u.id, u.name, u.name+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	activityID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at)
		VALUES ($1, $2, 'cycling', now())
	`, activityID, userA); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	gearID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope, applicable_sport_predicates)
		VALUES ($1, $2, 'RLS Test Gear', 'bike', 'bike', '[]')
	`, gearID, userA); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	assignmentID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_gear_assignments (id, user_id, activity_id, gear_id)
		VALUES ($1, $2, $3, $4)
	`, assignmentID, userA, activityID, gearID); err != nil {
		t.Fatalf("insert assignment: %v", err)
	}

	// Set session variable to userB and verify userA's row is invisible.
	if _, err := tx.Exec(ctx, fmt.Sprintf(
		`SET LOCAL app.current_user_id = '%s'`, userB,
	)); err != nil {
		t.Fatalf("set current_user_id: %v", err)
	}

	// Switch to skadia_readonly role for the check.
	if _, err := tx.Exec(ctx, `SET LOCAL ROLE skadia_readonly`); err != nil {
		t.Fatalf("set role skadia_readonly: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM activity_gear_assignments WHERE user_id = $1`, userA,
	).Scan(&count); err != nil {
		t.Fatalf("count as userB: %v", err)
	}
	if count != 0 {
		t.Errorf("skadia_readonly as userB should see 0 rows from userA, got %d", count)
	}
}

// TestRLSOnlySelectPoliciesForGearTables asserts that each of the five tables
// has exactly one policy, and that policy is SELECT for skadia_readonly.
func TestRLSOnlySelectPoliciesForGearTables(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tables := []string{
		"activity_gear_assignments",
		"gear_schema_definitions",
		"gear_component_swap_events",
		"gear_idempotency_keys",
		"gear_rule_action_state",
	}

	for _, tbl := range tables {
		tbl := tbl
		t.Run(tbl, func(t *testing.T) {
			rows, err := pool.Query(ctx, `
				SELECT cmd, roles::text
				FROM pg_policies
				WHERE schemaname = 'public' AND tablename = $1
			`, tbl)
			if err != nil {
				t.Fatalf("query pg_policies for %s: %v", tbl, err)
			}
			defer rows.Close()

			var policies []struct{ cmd, roles string }
			for rows.Next() {
				var p struct{ cmd, roles string }
				if err := rows.Scan(&p.cmd, &p.roles); err != nil {
					t.Fatalf("scan: %v", err)
				}
				policies = append(policies, p)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows err: %v", err)
			}

			if len(policies) != 1 {
				t.Errorf("%s: expected 1 policy, got %d", tbl, len(policies))
				return
			}
			p := policies[0]
			if p.cmd != "SELECT" {
				t.Errorf("%s: policy cmd = %q, want SELECT", tbl, p.cmd)
			}
			if !strings.Contains(p.roles, "skadia_readonly") {
				t.Errorf("%s: policy roles = %q, want to contain skadia_readonly", tbl, p.roles)
			}
		})
	}
}

// TestActivityGearAssignmentsUserMatchRejectsCrossUserFKs verifies the trigger
// rejects an insert where user_id does not match the activity's owner.
func TestActivityGearAssignmentsUserMatchRejectsCrossUserFKs(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userA := uuid.NewString()
	userB := uuid.NewString()

	for _, u := range []struct{ id, name string }{
		{userA, "trg_a_" + userA[:8]},
		{userB, "trg_b_" + userB[:8]},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, u.id, u.name, u.name+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	activityA := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO activities (id, user_id, sport_type, started_at)
		VALUES ($1, $2, 'cycling', now())
	`, activityA, userA); err != nil {
		t.Fatalf("insert activity: %v", err)
	}

	gearA := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope, applicable_sport_predicates)
		VALUES ($1, $2, 'Gear A', 'bike', 'bike', '[]')
	`, gearA, userA); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	// Insert with user_id = userB but activity owned by userA — must be rejected.
	if _, err := tx.Exec(ctx, `SAVEPOINT cross_user_check`); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, triggerErr := tx.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id)
		VALUES ($1, $2, $3)
	`, userB, activityA, gearA)
	if triggerErr == nil {
		t.Error("expected trigger rejection for cross-user assignment, got none")
	} else if !strings.Contains(triggerErr.Error(), "user_id mismatch") &&
		!strings.Contains(triggerErr.Error(), "activity_gear_assignments_user_match") {
		t.Errorf("unexpected error (expected user_id mismatch): %v", triggerErr)
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT cross_user_check`); err != nil {
		t.Fatalf("rollback to savepoint: %v", err)
	}

	// Insert with correct user_id = userA must succeed.
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_gear_assignments (user_id, activity_id, gear_id)
		VALUES ($1, $2, $3)
	`, userA, activityA, gearA); err != nil {
		t.Errorf("valid same-user assignment rejected: %v", err)
	}
}

// TestGearComponentSwapEventsUserMatchRejectsCrossUserFKs verifies the trigger
// on gear_component_swap_events rejects inserts where user_id != gear owner.
func TestGearComponentSwapEventsUserMatchRejectsCrossUserFKs(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
			t.Logf("rollback: %v", err)
		}
	})

	userA := uuid.NewString()
	userB := uuid.NewString()

	for _, u := range []struct{ id, name string }{
		{userA, "swap_a_" + userA[:8]},
		{userB, "swap_b_" + userB[:8]},
	} {
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, u.id, u.name, u.name+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
	}

	gearA := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear (id, user_id, name, type, scope, applicable_sport_predicates)
		VALUES ($1, $2, 'Swap Gear A', 'bike', 'bike', '[]')
	`, gearA, userA); err != nil {
		t.Fatalf("insert gear: %v", err)
	}

	// Insert with user_id = userB but gear owned by userA — must be rejected.
	if _, err := tx.Exec(ctx, `SAVEPOINT swap_cross_user`); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, triggerErr := tx.Exec(ctx, `
		INSERT INTO gear_component_swap_events (user_id, gear_id, slot_key, swap_date)
		VALUES ($1, $2, 'drivetrain', now())
	`, userB, gearA)
	if triggerErr == nil {
		t.Error("expected trigger rejection for cross-user swap event, got none")
	} else if !strings.Contains(triggerErr.Error(), "user_id mismatch") &&
		!strings.Contains(triggerErr.Error(), "gear_component_swap_events_user_match") {
		t.Errorf("unexpected error (expected user_id mismatch): %v", triggerErr)
	}
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT swap_cross_user`); err != nil {
		t.Fatalf("rollback to savepoint: %v", err)
	}

	// Insert with correct user_id = userA must succeed.
	if _, err := tx.Exec(ctx, `
		INSERT INTO gear_component_swap_events (user_id, gear_id, slot_key, swap_date)
		VALUES ($1, $2, 'drivetrain', now())
	`, userA, gearA); err != nil {
		t.Errorf("valid same-user swap event rejected: %v", err)
	}
}
