package migrations_test

// Integration tests for migration 000356_drop_system_type_check_for_code_validation.
//
// Originally written as a 000999 placeholder; finalize-migration.sh renumbers
// the file at merge time. The test name stays attached regardless of final number.
//
// Phase 4b.A — System-Artefakt-Architektur entkoppeln. The migration removes
// the static SQL CHECK whitelist on automation_rules.system_type (validation
// moves into Go code, services/api/internal/automation/system_artifacts.go),
// adds users.system_artifacts_version (gates the startup sweep), and creates
// idx_users_system_artifacts_version (BTree, NOT partial — see migration
// rationale comment).
//
// Tests are skipped when TEST_DATABASE_URL is unset.

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestMigration_DropSystemTypeCheck_ConstraintAbsent verifies that after the
// migration runs, the SQL CHECK constraint automation_rules_system_type_check
// is gone. Subsequent inserts with arbitrary system_type values must succeed.
func TestMigration_DropSystemTypeCheck_ConstraintAbsent(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_constraint
			WHERE conname = 'automation_rules_system_type_check'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("query pg_constraint: %v", err)
	}
	if exists {
		t.Errorf("expected automation_rules_system_type_check to be absent after migration, but it still exists")
	}
}

// TestMigration_DropSystemTypeCheck_AcceptsNewSystemType verifies that the
// migration enables new system_type values like 'lift_detected' to be inserted
// without violating any CHECK constraint. Cleanup deletes the test row.
func TestMigration_DropSystemTypeCheck_AcceptsNewSystemType(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	// Create a temporary user as FK target.
	userID := uuid.NewString()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, username, email, email_verified, registration_source)
		 VALUES ($1, $2, $3, true, 'test')`,
		userID,
		"sysmig999_"+userID[:8],
		"sysmig999_"+userID[:8]+"@example.com",
	); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID)
	})

	// Insert with a system_type that is NOT in the historical whitelist.
	// Pre-migration this would fail with constraint violation.
	var ruleID string
	err := pool.QueryRow(ctx, `
		INSERT INTO automation_rules
			(user_id, name, is_system, system_type, is_active, priority,
			 conditions, conditions_logic, actions, trigger_event)
		VALUES ($1, 'TestLift', true, 'lift_detected', true, -96,
			'[]'::jsonb, 'and', '[]'::jsonb, 'on_enrichment_complete')
		RETURNING id`,
		userID,
	).Scan(&ruleID)
	if err != nil {
		t.Fatalf("expected insert with system_type='lift_detected' to succeed after migration, got: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(ctx, "DELETE FROM automation_rules WHERE id = $1", ruleID)
	})
}

// TestMigration_AddSystemArtifactsVersionColumn verifies the migration added
// users.system_artifacts_version as INT NOT NULL DEFAULT 0.
func TestMigration_AddSystemArtifactsVersionColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var dataType, isNullable string
	var columnDefault *string
	err := pool.QueryRow(ctx, `
		SELECT data_type, column_default, is_nullable
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = 'users'
		  AND column_name  = 'system_artifacts_version'
	`).Scan(&dataType, &columnDefault, &isNullable)
	if err != nil {
		t.Fatalf("query column metadata: %v — column may not exist (migration not applied?)", err)
	}

	if dataType != "integer" {
		t.Errorf("expected data_type='integer', got %q", dataType)
	}
	if isNullable != "NO" {
		t.Errorf("expected is_nullable='NO', got %q", isNullable)
	}
	if columnDefault == nil || !strings.Contains(*columnDefault, "0") {
		got := "<nil>"
		if columnDefault != nil {
			got = *columnDefault
		}
		t.Errorf("expected column_default to contain '0', got %q", got)
	}
}

// TestMigration_AddSystemArtifactsVersionIndex verifies that the BTree index
// idx_users_system_artifacts_version exists. The plan documents why a partial
// index is intentionally avoided.
func TestMigration_AddSystemArtifactsVersionIndex(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var exists bool
	err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename  = 'users'
			  AND indexname  = 'idx_users_system_artifacts_version'
		)
	`).Scan(&exists)
	if err != nil {
		t.Fatalf("query pg_indexes: %v", err)
	}
	if !exists {
		t.Errorf("expected idx_users_system_artifacts_version to exist after migration")
	}
}
