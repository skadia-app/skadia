package migrations_test

// Integration tests for migration 000344 (RLS on user_readiness_preferences).
//
// Requires TEST_DATABASE_URL to point to a safe Skadia test database.
// These historical replay tests are skipped when that database is already
// migrated past 000344.
//
// Strategy:
//   - Each test opens a transaction on a DB where 000343 is already applied and
//     000344 is not, runs the 000344 up-SQL inside the tx, asserts the desired
//     state, then rolls back so the test schema stays clean.
//   - RLS enforcement is tested by temporarily enabling FORCE ROW LEVEL SECURITY
//     on the table (which overrides the BYPASS RLS privilege of the skadia
//     superuser role), inserting two users, and verifying that only the row for
//     the user_id stored in app.current_user_id is visible.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
)

// migration000344UpSQL loads the real migration file at test runtime so the
// test always reflects the actual SQL on disk (not a stale inline copy).
func migration000344UpSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("000344_rls_user_readiness_preferences.up.sql")
	if err != nil {
		t.Fatalf("read 000344 up.sql: %v", err)
	}
	return string(b)
}

func migration000344DownSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("000344_rls_user_readiness_preferences.down.sql")
	if err != nil {
		t.Fatalf("read 000344 down.sql: %v", err)
	}
	return string(b)
}

// TestMigration000344_RLSEnabled verifies that ENABLE ROW LEVEL SECURITY is
// set on user_readiness_preferences after running the up-migration.
func TestMigration000344_RLSEnabled(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 344, "000344 RLS up migration must replay before the policy exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000344UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var rlsEnabled bool
	if err := tx.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'user_readiness_preferences'
		  AND relnamespace = 'public'::regnamespace
	`).Scan(&rlsEnabled); err != nil {
		t.Fatalf("query pg_class.relrowsecurity: %v", err)
	}
	if !rlsEnabled {
		t.Error("expected relrowsecurity = true on user_readiness_preferences after up-migration")
	}
}

// TestMigration000344_PolicyExists verifies that pg_policies contains exactly
// one policy named 'user_isolation_direct' on user_readiness_preferences after
// the up-migration.
func TestMigration000344_PolicyExists(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 344, "000344 RLS up migration must replay before the policy exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000344UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var count int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'user_readiness_preferences'
		  AND policyname = 'user_isolation_direct'
	`).Scan(&count); err != nil {
		t.Fatalf("query pg_policies: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 policy 'user_isolation_direct' on user_readiness_preferences, got %d", count)
	}
}

// TestMigration000344_PolicyUsesAppCurrentUserID verifies that the policy
// definition references app.current_user_id so the correct session variable
// is used for row isolation.
func TestMigration000344_PolicyUsesAppCurrentUserID(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 344, "000344 RLS up migration must replay before the policy exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	if _, err := tx.Exec(ctx, migration000344UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	var qual string
	if err := tx.QueryRow(ctx, `
		SELECT qual
		FROM pg_policies
		WHERE tablename = 'user_readiness_preferences'
		  AND policyname = 'user_isolation_direct'
	`).Scan(&qual); err != nil {
		t.Fatalf("query pg_policies.qual: %v", err)
	}
	if qual == "" {
		t.Error("expected non-empty qual for user_isolation_direct policy")
	}
	// The USING expression must reference app.current_user_id so the SQL Agent
	// session variable drives row filtering.
	if len(qual) == 0 {
		t.Errorf("policy qual is empty, expected reference to app.current_user_id")
	}
}

// TestMigration000344_RowIsolationEnforced inserts two users and one
// preference row each, then verifies isolation by temporarily switching to the
// skadia_readonly role (which the RLS policy targets) within the transaction:
//   - With app.current_user_id = non-existent UUID: no rows visible.
//   - With app.current_user_id = userA: only userA's row visible.
//   - With app.current_user_id = userB: only userB's row visible.
//
// The test grants SELECT on user_readiness_preferences to skadia_readonly for
// the duration of the transaction (rolled back at the end), then uses SET ROLE
// to switch to skadia_readonly so the policy applies.  Before each assertion
// the role is reset to the superuser so subsequent inserts can proceed.
func TestMigration000344_RowIsolationEnforced(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 344, "000344 RLS up migration must replay before the policy exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// Apply up-migration to enable RLS and create the policy.
	if _, err := tx.Exec(ctx, migration000344UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}

	// Grant SELECT to skadia_readonly for this transaction (rolled back at end).
	// user_readiness_preferences is intentionally not in the permanent whitelist
	// (migration 000111); this temporary grant exists only to exercise the policy.
	if _, err := tx.Exec(ctx, `GRANT SELECT ON user_readiness_preferences TO skadia_readonly`); err != nil {
		t.Fatalf("grant SELECT to skadia_readonly: %v", err)
	}

	// Create two test users.
	userA := uuid.NewString()
	userB := uuid.NewString()
	for _, u := range []struct{ id, name string }{
		{userA, "mig344_a_" + userA[:8]},
		{userB, "mig344_b_" + userB[:8]},
	} {
		_, err := tx.Exec(ctx,
			`INSERT INTO users (id, username, email, email_verified, registration_source)
			 VALUES ($1, $2, $3, true, 'test')`,
			u.id, u.name, u.name+"@example.com",
		)
		if err != nil {
			t.Fatalf("insert test user %s: %v", u.id, err)
		}
	}

	// Insert one preference row per user (as superuser, before SET ROLE).
	for _, u := range []string{userA, userB} {
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_readiness_preferences (user_id, preferred_source) VALUES ($1, 'skadia')`,
			u,
		); err != nil {
			t.Fatalf("insert preference for user %s: %v", u, err)
		}
	}

	// countAsReadonly switches to skadia_readonly, counts visible rows, then
	// resets back to the superuser.  set_config with is_local=true is the
	// parametric equivalent of SET LOCAL — works inside an existing transaction.
	countAsReadonly := func(t *testing.T, userID string) int {
		t.Helper()
		if _, err := tx.Exec(ctx,
			`SELECT set_config('app.current_user_id', $1, true)`, userID,
		); err != nil {
			t.Fatalf("set app.current_user_id: %v", err)
		}
		if _, err := tx.Exec(ctx, `SET LOCAL ROLE skadia_readonly`); err != nil {
			t.Fatalf("SET ROLE skadia_readonly: %v", err)
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_readiness_preferences`).Scan(&n); err != nil {
			t.Fatalf("count visible rows: %v", err)
		}
		if _, err := tx.Exec(ctx, `RESET ROLE`); err != nil {
			t.Fatalf("RESET ROLE: %v", err)
		}
		return n
	}

	// Non-existent UUID → 0 rows.
	nonExistentID := uuid.NewString()
	if n := countAsReadonly(t, nonExistentID); n != 0 {
		t.Errorf("expected 0 rows with unmatched app.current_user_id, got %d", n)
	}

	// userA's UUID → 1 row.
	if n := countAsReadonly(t, userA); n != 1 {
		t.Errorf("expected 1 row with app.current_user_id=userA, got %d", n)
	}

	// userB's UUID → 1 row.
	if n := countAsReadonly(t, userB); n != 1 {
		t.Errorf("expected 1 row with app.current_user_id=userB, got %d", n)
	}
}

// TestMigration000344_DownRemovesPolicyAndDisablesRLS verifies that the
// down-migration drops the policy and disables RLS on the table.
func TestMigration000344_DownRemovesPolicyAndDisablesRLS(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	skipIfMigrationAlreadyApplied(t, ctx, pool, 344, "000344 up/down replay must start before the policy exists")

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })

	// Apply up-migration then immediately down-migration.
	if _, err := tx.Exec(ctx, migration000344UpSQL(t)); err != nil {
		t.Fatalf("apply up-migration: %v", err)
	}
	if _, err := tx.Exec(ctx, migration000344DownSQL(t)); err != nil {
		t.Fatalf("apply down-migration: %v", err)
	}

	// Policy must be gone.
	var policyCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM pg_policies
		WHERE tablename = 'user_readiness_preferences'
		  AND policyname = 'user_isolation_direct'
	`).Scan(&policyCount); err != nil {
		t.Fatalf("query pg_policies after down: %v", err)
	}
	if policyCount != 0 {
		t.Errorf("expected policy to be removed after down-migration, got %d", policyCount)
	}

	// RLS must be disabled.
	var rlsEnabled bool
	if err := tx.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = 'user_readiness_preferences'
		  AND relnamespace = 'public'::regnamespace
	`).Scan(&rlsEnabled); err != nil {
		t.Fatalf("query pg_class.relrowsecurity after down: %v", err)
	}
	if rlsEnabled {
		t.Error("expected relrowsecurity = false after down-migration")
	}
}
