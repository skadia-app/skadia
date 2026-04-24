package migrations_test

// Integration tests for migration 000326 (library_settings_admin_model).
//
// Requires TEST_DATABASE_URL to point to a fully-migrated Skadia database.
// Tests are skipped when the env var is not set so CI without a live DB stays
// green.
//
// Strategy: each test inserts its own test user, re-applies the migration SQL
// inline (UPDATE + audit INSERT), then inspects the result, and cleans up via
// t.Cleanup.  This is safe because the migration SQL is idempotent — running the
// UPDATE and audit INSERT again does not corrupt real data.

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationTestPool returns a pgxpool connected to TEST_DATABASE_URL or skips.
func migrationTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping migration integration test")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// applyMigration000326 re-runs the UPDATE part of migration 000326.
// The audit INSERT is intentionally omitted here so we do not spam auth_audit_log
// during testing; the audit-log test has its own controlled INSERT.
const migration000326UpdateSQL = `
UPDATE users
SET library_dir_name = NULL
WHERE library_dir_name IS NOT NULL
  AND library_dir_name ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{8,12}$'
`

// TestMigration000326ResetsUUIDShapedLabels verifies that a canonical 36-char
// UUID-shaped library_dir_name is set to NULL by the migration.
func TestMigration000326ResetsUUIDShapedLabels(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// Insert a test user with a UUID-shaped label.
	userID := uuid.NewString()
	label := "11111111-1111-4111-8111-111111111111" // canonical 36-char UUID
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, library_dir_name)
		 VALUES ($1, $2, $3, true, 'test', $4)
		 ON CONFLICT (id) DO UPDATE SET library_dir_name = EXCLUDED.library_dir_name`,
		userID,
		"mig326_uuid_full_"+userID[:8],
		"mig326_uuid_full_"+userID[:8]+"@example.com",
		label,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	// Re-apply migration UPDATE.
	if _, err := pool.Exec(ctx, migration000326UpdateSQL); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var name *string
	if err := pool.QueryRow(ctx, `SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&name); err != nil {
		t.Fatalf("query: %v", err)
	}
	if name != nil {
		t.Errorf("expected library_dir_name = NULL after migration, got %q", *name)
	}
}

// TestMigration000326PreservesCustomLabels verifies that a human-readable
// label like "my-library" is NOT touched by the migration.
func TestMigration000326PreservesCustomLabels(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	label := "my-library"
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, library_dir_name)
		 VALUES ($1, $2, $3, true, 'test', $4)
		 ON CONFLICT (id) DO UPDATE SET library_dir_name = EXCLUDED.library_dir_name`,
		userID,
		"mig326_custom_"+userID[:8],
		"mig326_custom_"+userID[:8]+"@example.com",
		label,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	if _, err := pool.Exec(ctx, migration000326UpdateSQL); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	var got *string
	if err := pool.QueryRow(ctx, `SELECT library_dir_name FROM users WHERE id = $1`, userID).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	if got == nil {
		t.Errorf("expected library_dir_name = %q to be preserved, got NULL", label)
	} else if *got != label {
		t.Errorf("expected library_dir_name = %q, got %q", label, *got)
	}
}

// TestMigration000326PreservesTruncatedUUIDs verifies that 35-char and 33-char
// UUID-shaped strings (last block truncated) are also reset to NULL.
// The regex in the migration uses {8,12} for the last block, covering these
// shorter-than-standard variants.
func TestMigration000326PreservesTruncatedUUIDs(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	cases := []struct {
		id    string
		label string // last block is shorter than 12 chars but >= 8
	}{
		{
			id:    uuid.NewString(),
			label: "de7508e1-c517-4764-8b41-3f27341433a", // 35 chars (last block = 11)
		},
		{
			id:    uuid.NewString(),
			label: "de7508e1-c517-4764-8b41-3f2734143", // 33 chars (last block = 9)
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			_, err := pool.Exec(ctx,
				`INSERT INTO users (id, username, email, email_verified, registration_source, library_dir_name)
				 VALUES ($1, $2, $3, true, 'test', $4)
				 ON CONFLICT (id) DO UPDATE SET library_dir_name = EXCLUDED.library_dir_name`,
				tc.id,
				"mig326_tr_"+tc.id[:8],
				"mig326_tr_"+tc.id[:8]+"@example.com",
				tc.label,
			)
			if err != nil {
				t.Fatalf("insert test user %q: %v", tc.id, err)
			}
			t.Cleanup(func() {
				pool.Exec(ctx, "DELETE FROM users WHERE id = $1", tc.id)
			})

			if _, err := pool.Exec(ctx, migration000326UpdateSQL); err != nil {
				t.Fatalf("apply migration: %v", err)
			}

			var got *string
			if err := pool.QueryRow(ctx, `SELECT library_dir_name FROM users WHERE id = $1`, tc.id).Scan(&got); err != nil {
				t.Fatalf("query: %v", err)
			}
			if got != nil {
				t.Errorf("label %q: expected NULL after migration, got %q", tc.label, *got)
			}
		})
	}
}

// TestMigration000326AuditLogEntry verifies that the migration CTE inserts an
// audit_log entry with event_type = 'migration', details->>'migration' = '000326',
// AND that reset_null_count reflects only the rows actually reset by the CTE
// (not the pre-existing NULL count).
//
// The real CTE from 000326_library_settings_admin_model.up.sql is run verbatim
// so we exercise the exact production SQL path (R2 MED #9).
func TestMigration000326AuditLogEntry(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// Insert a test user with a UUID-shaped label so the CTE has at least one
	// row to reset — this makes reset_null_count verifiable (must be >= 1).
	userID := uuid.NewString()
	label := "11111111-1111-4111-8111-111111111111" // canonical 36-char UUID
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source, library_dir_name)
		 VALUES ($1, $2, $3, true, 'test', $4)
		 ON CONFLICT (id) DO UPDATE SET library_dir_name = EXCLUDED.library_dir_name`,
		userID,
		"mig326_audit_"+userID[:8],
		"mig326_audit_"+userID[:8]+"@example.com",
		label,
	)
	if err != nil {
		t.Fatalf("insert test user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	// Run the REAL CTE from the migration file (verbatim copy of the production SQL).
	// This tests the audit INSERT is driven by the CTE reset_rows count, not a
	// static subquery on current NULLs (R2 MED #9).
	const realMigrationCTE = `
WITH reset_rows AS (
    UPDATE users
    SET library_dir_name = NULL
    WHERE library_dir_name IS NOT NULL
      AND library_dir_name ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{8,12}$'
    RETURNING 1
)
INSERT INTO auth_audit_log (user_id, event_type, details)
SELECT NULL, 'migration', jsonb_build_object(
  'migration', '000326',
  'name', 'library_settings_admin_model',
  'reset_null_count', (SELECT count(*) FROM reset_rows)
)`

	if _, err := pool.Exec(ctx, realMigrationCTE); err != nil {
		t.Fatalf("run CTE migration SQL: %v", err)
	}

	// Verify: at least one audit_log entry with matching migration key.
	var count int
	err = pool.QueryRow(ctx, `
		SELECT count(*)
		FROM auth_audit_log
		WHERE event_type = 'migration'
		  AND details->>'migration' = '000326'
	`).Scan(&count)
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one auth_audit_log entry with migration=000326, got 0")
	}

	// Verify: the most-recent entry has reset_null_count >= 1 (our test user was reset).
	var resetCount int
	err = pool.QueryRow(ctx, `
		SELECT (details->>'reset_null_count')::int
		FROM auth_audit_log
		WHERE event_type = 'migration'
		  AND details->>'migration' = '000326'
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&resetCount)
	if err != nil {
		t.Fatalf("query reset_null_count: %v", err)
	}
	if resetCount < 1 {
		t.Errorf("expected reset_null_count >= 1 (test user should have been reset), got %d", resetCount)
	}
}
