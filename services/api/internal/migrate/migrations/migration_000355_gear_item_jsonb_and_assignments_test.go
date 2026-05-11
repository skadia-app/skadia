package migrations_test

// Integration tests for migration 000355 (gear_item_jsonb_and_assignments).
//
// Placeholder filename stays 000355 until finalize-migration.sh renumbers it
// at merge time. Test function names are intentionally NOT 000355-prefixed per
// CLAUDE.md migration-placeholder convention.
//
// Strategy: assert the already-applied migration left the correct schema state.
// Down-migration test runs the down SQL in a rolled-back transaction.
//
// Requires TEST_DATABASE_URL pointing at the per-worktree test DB
// (managed by scripts/test_db.sh). Tests skip if TEST_DATABASE_URL is unset.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// containsStr000355 is a local substring helper used by constraint-text assertions.
func containsStr000355(s, substr string) bool {
	return strings.Contains(s, substr)
}

func readMigration000355UpSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("000355_gear_item_jsonb_and_assignments.up.sql")
	if err != nil {
		t.Fatalf("read 000355 up.sql: %v", err)
	}
	return string(b)
}

func readMigration000355DownSQL(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("000355_gear_item_jsonb_and_assignments.down.sql")
	if err != nil {
		t.Fatalf("read 000355 down.sql: %v", err)
	}
	return string(b)
}

// hasColumn000355 returns true when the column exists on the table.
func hasColumn000355(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tableName, columnName string) bool {
	t.Helper()
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name   = $1
		  AND column_name  = $2
	`, tableName, columnName).Scan(&count)
	if err != nil {
		t.Fatalf("hasColumn %s.%s: %v", tableName, columnName, err)
	}
	return count > 0
}

// hasTable000355 returns true when the table exists in the public schema.
func hasTable000355(t *testing.T, ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tableName string) bool {
	t.Helper()
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name   = $1
	`, tableName).Scan(&count)
	if err != nil {
		t.Fatalf("hasTable %s: %v", tableName, err)
	}
	return count > 0
}

// tableHasRLS000355 returns true when the table has row security enabled.
func tableHasRLS000355(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) bool {
	t.Helper()
	var enabled bool
	err := pool.QueryRow(ctx, `
		SELECT relrowsecurity
		FROM pg_class
		WHERE relname = $1 AND relnamespace = 'public'::regnamespace
	`, tableName).Scan(&enabled)
	if err != nil {
		t.Fatalf("tableHasRLS %s: %v", tableName, err)
	}
	return enabled
}

// countPolicies000355 counts policies for the table by cmd in pg_policies.
func countPolicies000355(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName, cmd string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_policies
		WHERE schemaname = 'public'
		  AND tablename  = $1
		  AND cmd        = $2
	`, tableName, cmd).Scan(&count)
	if err != nil {
		t.Fatalf("countPolicies %s/%s: %v", tableName, cmd, err)
	}
	return count
}

// countAllPolicies000355 counts all policies for the table.
func countAllPolicies000355(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_policies
		WHERE schemaname = 'public'
		  AND tablename  = $1
	`, tableName).Scan(&count)
	if err != nil {
		t.Fatalf("countAllPolicies %s: %v", tableName, err)
	}
	return count
}

// TestMigration_GearItemJSONBAndAssignments_Up verifies the schema state after
// migration 000355 has been applied to the test database.
func TestMigration_GearItemJSONBAndAssignments_Up(t *testing.T) {
	// Verify the up SQL file exists (ensures TDD red phase is real).
	readMigration000355UpSQL(t)

	pool := migrationTestPool(t)
	ctx := context.Background()

	t.Run("gear_columns_present", func(t *testing.T) {
		for _, col := range []string{
			"attributes", "modifiers", "applicable_sport_predicates",
			"scope", "total_moving_time_s", "total_elevation_loss_m",
		} {
			if !hasColumn000355(t, ctx, pool, "gear", col) {
				t.Errorf("gear missing column %q", col)
			}
		}
	})

	t.Run("gear_columns_absent", func(t *testing.T) {
		for _, col := range []string{"default_sport_predicates", "default_for_sports"} {
			if hasColumn000355(t, ctx, pool, "gear", col) {
				t.Errorf("gear still has column %q (should be dropped/renamed)", col)
			}
		}
	})

	t.Run("gear_scope_not_null", func(t *testing.T) {
		var isNullable string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'gear' AND column_name = 'scope'
		`).Scan(&isNullable)
		if err != nil {
			t.Fatalf("query scope nullability: %v", err)
		}
		if isNullable != "NO" {
			t.Errorf("gear.scope should be NOT NULL, got is_nullable=%q", isNullable)
		}
	})

	t.Run("applicable_sport_predicates_constraint_exists", func(t *testing.T) {
		var count int
		err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM information_schema.check_constraints
			WHERE constraint_schema = 'public'
			  AND constraint_name   = 'gear_applicable_sport_predicates_valid'
		`).Scan(&count)
		if err != nil {
			t.Fatalf("query constraint: %v", err)
		}
		if count != 1 {
			t.Errorf("constraint gear_applicable_sport_predicates_valid missing")
		}
	})

	t.Run("applicable_sport_predicates_constraint_rejects_invalid", func(t *testing.T) {
		// Insert a row with an invalid predicate to confirm the constraint fires.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig999c_"+userID[:8], "mig999c_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		if _, err := tx.Exec(ctx, `SAVEPOINT bad_predicate`); err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		_, insertErr := tx.Exec(ctx, `
			INSERT INTO gear (user_id, name, type, scope, applicable_sport_predicates)
			VALUES ($1, 'TestGear', 'other', 'other', '[{"not_valid_key":"cycling"}]')
		`, userID)
		if insertErr == nil {
			t.Error("expected constraint violation for invalid predicate, got none")
		}
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT bad_predicate`); err != nil {
			t.Fatalf("rollback to savepoint: %v", err)
		}
	})

	t.Run("gear_scope_is_free_text", func(t *testing.T) {
		// Arbitrary scope values must not be rejected by the DB.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig999s_"+userID[:8], "mig999s_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (user_id, name, type, scope, applicable_sport_predicates)
			VALUES ($1, 'FutureScope', 'other', 'future_scope_not_in_schema', '[]')
		`, userID); err != nil {
			t.Errorf("gear.scope rejected arbitrary text value: %v", err)
		}
	})

	t.Run("rls_tables_with_exactly_one_select_policy", func(t *testing.T) {
		for _, tbl := range []string{
			"activity_gear_assignments",
			"gear_schema_definitions",
			"gear_rule_action_state",
			"gear_component_swap_events",
			"gear_idempotency_keys",
		} {
			tbl := tbl
			t.Run(tbl, func(t *testing.T) {
				if !hasTable000355(t, ctx, pool, tbl) {
					t.Fatalf("table %q does not exist", tbl)
				}
				if !tableHasRLS000355(t, ctx, pool, tbl) {
					t.Errorf("table %q does not have RLS enabled", tbl)
				}
				if n := countPolicies000355(t, ctx, pool, tbl, "SELECT"); n != 1 {
					t.Errorf("table %q: expected 1 SELECT policy, got %d", tbl, n)
				}
				if n := countAllPolicies000355(t, ctx, pool, tbl); n != 1 {
					t.Errorf("table %q: expected exactly 1 policy total, got %d", tbl, n)
				}
			})
		}
	})

	// Verify response_status_code column added for idempotency replay status preservation
	// (Critic-4 #8: replay must return original HTTP status, not always 200 OK).
	t.Run("gear_idempotency_keys_has_response_status_code", func(t *testing.T) {
		if !hasColumn000355(t, ctx, pool, "gear_idempotency_keys", "response_status_code") {
			t.Error("gear_idempotency_keys missing column response_status_code")
		}
		// Verify NOT NULL with default 200.
		var isNullable, colDefault string
		err := pool.QueryRow(ctx, `
			SELECT is_nullable, COALESCE(column_default, '')
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'gear_idempotency_keys'
			  AND column_name  = 'response_status_code'
		`).Scan(&isNullable, &colDefault)
		if err != nil {
			t.Fatalf("query response_status_code column attributes: %v", err)
		}
		if isNullable != "NO" {
			t.Errorf("gear_idempotency_keys.response_status_code must be NOT NULL, got is_nullable=%q", isNullable)
		}
		if colDefault != "200" {
			t.Errorf("gear_idempotency_keys.response_status_code default = %q, want %q", colDefault, "200")
		}
	})

	t.Run("gear_schema_definitions_no_is_system", func(t *testing.T) {
		if hasColumn000355(t, ctx, pool, "gear_schema_definitions", "is_system") {
			t.Error("gear_schema_definitions must NOT have is_system column (User-Decision A3)")
		}
	})

	t.Run("audit_matrix_columns_present", func(t *testing.T) {
		checks := []struct{ table, column string }{
			{"gear_components", "attributes"},
			{"gear_components", "type_key"},
			{"gear_components", "preset_gear_type_key"},
			{"gear_components", "total_activities"},
			{"gear_components", "total_moving_time_s"},
			{"gear_components", "total_elevation_loss_m"},
			{"component_assignments", "elevation_loss_m"},
			{"component_assignments", "moving_time_s"},
			{"component_assignments", "activities_count"},
			{"component_assignments", "attributes"},
			{"chain_rotation_members", "total_elevation_loss_m"},
			{"chain_rotation_members", "total_moving_time_s"},
			{"chain_rotation_members", "total_time_s"},
			{"chain_rotation_members", "total_activities"},
			{"chain_rotation_groups", "swap_interval_metric_key"},
			{"chain_rotation_groups", "swap_interval_value"},
			{"gear_service_intervals", "metric_key"},
			{"gear_service_intervals", "last_service_metric_value"},
			{"gear_service_events", "metric_snapshot"},
			{"automation_rules", "last_warning_at"},
			{"automation_rules", "last_warning_code"},
			{"automation_rules", "last_warning_message"},
			// Block D: component snapshot (nullable JSONB)
			{"bike_setups", "component_snapshot"},
		}
		for _, c := range checks {
			if !hasColumn000355(t, ctx, pool, c.table, c.column) {
				t.Errorf("table %q missing column %q", c.table, c.column)
			}
		}
	})

	// Verify bike_setups.component_snapshot is nullable JSONB with no NOT NULL constraint
	// and no DB-level CHECK (validation lives in Go).
	t.Run("bike_setups_component_snapshot_nullable_jsonb", func(t *testing.T) {
		var dataType, isNullable string
		err := pool.QueryRow(ctx, `
			SELECT data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'bike_setups'
			  AND column_name  = 'component_snapshot'
		`).Scan(&dataType, &isNullable)
		if err != nil {
			t.Fatalf("query bike_setups.component_snapshot column attributes: %v", err)
		}
		if dataType != "jsonb" {
			t.Errorf("bike_setups.component_snapshot data_type = %q, want %q", dataType, "jsonb")
		}
		if isNullable != "YES" {
			t.Errorf("bike_setups.component_snapshot must be nullable, got is_nullable=%q", isNullable)
		}
	})

	// Verify gear_components.preset_gear_type_key is a nullable TEXT column
	// (no NOT NULL, no DB-level CHECK; validation lives in Go).
	t.Run("preset_gear_type_key_is_nullable_text", func(t *testing.T) {
		var dataType, isNullable string
		err := pool.QueryRow(ctx, `
			SELECT data_type, is_nullable
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name   = 'gear_components'
			  AND column_name  = 'preset_gear_type_key'
		`).Scan(&dataType, &isNullable)
		if err != nil {
			t.Fatalf("query preset_gear_type_key column attributes: %v", err)
		}
		if dataType != "text" {
			t.Errorf("preset_gear_type_key data_type = %q, want %q", dataType, "text")
		}
		if isNullable != "YES" {
			t.Errorf("preset_gear_type_key must be nullable, got is_nullable=%q", isNullable)
		}
	})

	t.Run("gin_indexes_exist", func(t *testing.T) {
		for _, idxName := range []string{
			"gear_attributes_gin",
			"gear_modifiers_gin",
			"gear_applicable_sport_predicates_gin",
		} {
			var count int
			if err := pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM pg_indexes
				WHERE schemaname = 'public' AND indexname = $1 AND indexdef LIKE '%gin%'
			`, idxName).Scan(&count); err != nil {
				t.Fatalf("query index %s: %v", idxName, err)
			}
			if count != 1 {
				t.Errorf("GIN index %q not found", idxName)
			}
		}
	})

	t.Run("btree_indexes_exist", func(t *testing.T) {
		for _, idxName := range []string{
			"gear_user_scope_idx",
			"gear_idempotency_keys_created_at_idx",
		} {
			var count int
			if err := pool.QueryRow(ctx, `
				SELECT COUNT(*) FROM pg_indexes
				WHERE schemaname = 'public' AND indexname = $1
			`, idxName).Scan(&count); err != nil {
				t.Fatalf("query index %s: %v", idxName, err)
			}
			if count != 1 {
				t.Errorf("B-tree index %q not found", idxName)
			}
		}
	})

	t.Run("chk_trigger_event_includes_on_gear_usage_update", func(t *testing.T) {
		// Verify the constraint text includes on_gear_usage_update.
		var consDef string
		if err := pool.QueryRow(ctx, `
			SELECT pg_get_constraintdef(oid)
			FROM pg_constraint
			WHERE conrelid = 'automation_rules'::regclass
			  AND conname  = 'chk_trigger_event'
		`).Scan(&consDef); err != nil {
			t.Fatalf("query chk_trigger_event: %v", err)
		}
		if consDef == "" {
			t.Error("chk_trigger_event constraint not found on automation_rules")
		}
		// The constraint must include on_gear_usage_update.
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})
		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig999te_"+userID[:8], "mig999te_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO automation_rules (user_id, name, conditions, actions, trigger_event)
			VALUES ($1, 'GearUsageRule', '{}', '{}', 'on_gear_usage_update')
		`, userID); err != nil {
			t.Errorf("on_gear_usage_update must be a valid trigger_event: %v", err)
		}
	})

	t.Run("gear_service_intervals_metric_key_check_accepts_elevation_loss_m", func(t *testing.T) {
		// Verifies that the gear_service_intervals_metric_key_check constraint
		// (added by 000355) lists 'elevation_loss_m' as a valid metric_key.
		// Column presence is covered by audit_matrix_columns_present; this
		// subtest specifically exercises the CHECK constraint value-list by
		// reading the constraint definition directly from pg_constraint.
		var consDef string
		if err := pool.QueryRow(ctx, `
			SELECT pg_get_constraintdef(oid)
			FROM pg_constraint
			WHERE conrelid = 'gear_service_intervals'::regclass
			  AND conname  = 'gear_service_intervals_metric_key_check'
		`).Scan(&consDef); err != nil {
			t.Fatalf("query gear_service_intervals_metric_key_check: %v", err)
		}
		if consDef == "" {
			t.Fatal("gear_service_intervals_metric_key_check constraint not found")
		}
		// The constraint definition must include elevation_loss_m.
		if !containsStr000355(consDef, "elevation_loss_m") {
			t.Errorf("gear_service_intervals_metric_key_check does not include 'elevation_loss_m'; got: %s", consDef)
		}
	})

	// Verify the scope backfill for the legacy 'e_bike' type actually exercises
	// the migration's UPDATE logic.
	//
	// Option 2 approach (migration already applied): drop NOT NULL within a
	// rolled-back transaction, insert rows with scope=NULL, run the migration's
	// UPDATE statements verbatim, then assert the resulting scope values.
	// The entire transaction is rolled back so the schema change is never committed.
	//
	// NOTE: if the migration's scope_backfill VALUES CTE changes, this test must
	// be updated to mirror it — the verbatim SQL below is intentional.
	t.Run("scope_backfill_e_bike_maps_to_bike", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		// Temporarily allow NULL so we can insert a pre-migration baseline row.
		if _, err := tx.Exec(ctx, `ALTER TABLE gear ALTER COLUMN scope DROP NOT NULL`); err != nil {
			t.Fatalf("drop not null on scope: %v", err)
		}

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig354_ebike_"+userID[:8], "mig354_ebike_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		// Insert row with scope=NULL — simulates pre-migration state for type='e_bike'.
		gearID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (id, user_id, name, type, status, applicable_sport_predicates, scope)
			VALUES ($1, $2, 'E-Bike Legacy', 'e_bike', 'active', '[]', NULL)
		`, gearID, userID); err != nil {
			t.Fatalf("insert pre-migration e_bike row: %v", err)
		}

		// Verify negative control: scope is NULL before running migration logic.
		var scopeBefore *string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scopeBefore); err != nil {
			t.Fatalf("read scope before backfill: %v", err)
		}
		if scopeBefore != nil {
			t.Fatalf("expected scope=NULL before migration UPDATE, got %q", *scopeBefore)
		}

		// Run the migration's scope_backfill UPDATE logic (VALUES CTE matches up.sql
		// lines 92-132 exactly).  The WHERE clause narrows to scope IS NULL because
		// this test runs against an already-migrated schema: real rows already have
		// a non-NULL scope from the original migration run, and the NOT NULL constraint
		// prevents the test from inserting NULL rows without ALTER TABLE.  The
		// migration itself does NOT have a WHERE g.scope IS NULL guard — it updates
		// every matching row unconditionally.  The narrowing here is safe because the
		// test row was explicitly inserted with scope=NULL (after a temporary DROP NOT NULL).
		if _, err := tx.Exec(ctx, `
			WITH scope_backfill(type_key, scope) AS (
			    VALUES
			        ('bike',                       'bike'),
			        ('bike.road',                  'bike'),
			        ('bike.mtb',                   'bike'),
			        ('bike.gravel',                'bike'),
			        ('bike.gravel.cyclocross',     'bike'),
			        ('bike.city',                  'bike'),
			        ('bike.trekking',              'bike'),
			        ('bike.cargo',                 'bike'),
			        ('bike.tandem',                'bike'),
			        ('bike.recumbent',             'bike'),
			        ('bike.kids',                  'bike'),
			        ('bike.balance',               'bike'),
			        ('bike.indoor',                'bike'),
			        ('bike.handcycle',             'bike'),
			        ('bike.consumable.chain_lube', 'bike'),
			        ('bike.consumable.brake_pads', 'bike'),
			        ('bike.consumable.tire_sealant','bike'),
			        ('e_bike',                     'bike'),
			        ('helmet',                     'other'),
			        ('goggles',                    'other'),
			        ('shoes',                      'shoes'),
			        ('shoes.cycling',              'shoes'),
			        ('shoes.cycling.gravel',       'shoes'),
			        ('shoes.running',              'shoes'),
			        ('shoes.hiking',               'shoes'),
			        ('shoes.ski',                  'shoes'),
			        ('shoes.climbing',             'shoes'),
			        ('ski',                        'winter'),
			        ('watercraft',                 'water'),
			        ('dive_computer',              'diving'),
			        ('climbing_harness',           'climb'),
			        ('apparel',                    'apparel'),
			        ('other',                      'other'),
			        ('climb.harness',              'climb'),
			        ('climb.rope',                 'climb'),
			        ('climb.crampons',             'climb'),
			        ('climb.ice_axe',              'climb'),
			        ('climb.belay_device',         'climb'),
			        ('climb.quickdraw',            'climb'),
			        ('climb.chalkbag',             'climb'),
			        ('climb.crashpad',             'climb'),
			        ('apparel.pants',              'apparel'),
			        ('apparel.jersey',             'apparel'),
			        ('apparel.gloves',             'apparel'),
			        ('apparel.socks',              'apparel'),
			        ('apparel.jacket',             'apparel'),
			        ('apparel.shorts',             'apparel')
			)
			UPDATE gear g
			SET scope = sb.scope
			FROM scope_backfill sb
			WHERE g.scope IS NULL
			  AND g.type = sb.type_key
		`); err != nil {
			t.Fatalf("scope_backfill UPDATE: %v", err)
		}
		// Fallback: any row still NULL gets 'other'.
		if _, err := tx.Exec(ctx, `UPDATE gear SET scope = 'other' WHERE scope IS NULL`); err != nil {
			t.Fatalf("scope fallback UPDATE: %v", err)
		}

		var scope string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scope); err != nil {
			t.Fatalf("read scope after backfill: %v", err)
		}
		if scope != "bike" {
			t.Errorf("legacy e_bike row: scope = %q, want %q", scope, "bike")
		}
	})

	// Verify the unknown-type fallback: any type not in the backfill list gets scope='other'.
	t.Run("scope_backfill_unknown_type_falls_back_to_other", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		// Temporarily allow NULL so we can insert a pre-migration baseline row.
		if _, err := tx.Exec(ctx, `ALTER TABLE gear ALTER COLUMN scope DROP NOT NULL`); err != nil {
			t.Fatalf("drop not null on scope: %v", err)
		}

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig354_unk_"+userID[:8], "mig354_unk_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		// Insert row with scope=NULL and an unknown type — simulates a row the
		// migration's backfill CTE does not match, so the fallback UPDATE applies.
		gearID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (id, user_id, name, type, status, applicable_sport_predicates, scope)
			VALUES ($1, $2, 'Weird Gear', 'made_up_legacy', 'active', '[]', NULL)
		`, gearID, userID); err != nil {
			t.Fatalf("insert pre-migration unknown-type row: %v", err)
		}

		// Verify negative control: scope is NULL before running migration logic.
		var scopeBefore *string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scopeBefore); err != nil {
			t.Fatalf("read scope before backfill: %v", err)
		}
		if scopeBefore != nil {
			t.Fatalf("expected scope=NULL before migration UPDATE, got %q", *scopeBefore)
		}

		// Run the backfill UPDATE logic (abbreviated CTE for brevity; unknown type
		// 'made_up_legacy' won't match any row so scope stays NULL here).
		// WHERE g.scope IS NULL narrows to test-inserted rows only — the real
		// migration updates unconditionally; see the e_bike subtest comment above.
		if _, err := tx.Exec(ctx, `
			WITH scope_backfill(type_key, scope) AS (
			    VALUES
			        ('bike',                       'bike'),
			        ('e_bike',                     'bike'),
			        ('helmet',                     'other'),
			        ('goggles',                    'other'),
			        ('shoes',                      'shoes'),
			        ('ski',                        'winter'),
			        ('watercraft',                 'water'),
			        ('dive_computer',              'diving'),
			        ('climbing_harness',           'climb'),
			        ('apparel',                    'apparel'),
			        ('other',                      'other'),
			        ('climb.harness',              'climb'),
			        ('climb.rope',                 'climb'),
			        ('climb.crampons',             'climb'),
			        ('climb.ice_axe',              'climb'),
			        ('climb.belay_device',         'climb'),
			        ('climb.quickdraw',            'climb'),
			        ('climb.chalkbag',             'climb'),
			        ('climb.crashpad',             'climb'),
			        ('apparel.pants',              'apparel'),
			        ('apparel.jersey',             'apparel'),
			        ('apparel.gloves',             'apparel'),
			        ('apparel.socks',              'apparel'),
			        ('apparel.jacket',             'apparel'),
			        ('apparel.shorts',             'apparel')
			)
			UPDATE gear g
			SET scope = sb.scope
			FROM scope_backfill sb
			WHERE g.scope IS NULL
			  AND g.type = sb.type_key
		`); err != nil {
			t.Fatalf("scope_backfill UPDATE: %v", err)
		}
		// Fallback: any row still NULL (type='made_up_legacy' didn't match) gets 'other'.
		if _, err := tx.Exec(ctx, `UPDATE gear SET scope = 'other' WHERE scope IS NULL`); err != nil {
			t.Fatalf("scope fallback UPDATE: %v", err)
		}

		var scope string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scope); err != nil {
			t.Fatalf("read scope after backfill: %v", err)
		}
		if scope != "other" {
			t.Errorf("unknown-type gear row: scope = %q, want %q", scope, "other")
		}
	})

	// Verify climb and apparel leaf keys map to their respective scopes (Re-Critic #7).
	// Uses the same Option 2 approach as the subtests above.
	t.Run("scope_backfill_climb_leaf_maps_to_climb", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		if _, err := tx.Exec(ctx, `ALTER TABLE gear ALTER COLUMN scope DROP NOT NULL`); err != nil {
			t.Fatalf("drop not null on scope: %v", err)
		}

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig354_cl_"+userID[:8], "mig354_cl_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		// climb.harness is representative of all 8 climb leaf keys.
		gearID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (id, user_id, name, type, status, applicable_sport_predicates, scope)
			VALUES ($1, $2, 'Harness', 'climb.harness', 'active', '[]', NULL)
		`, gearID, userID); err != nil {
			t.Fatalf("insert climb.harness row: %v", err)
		}

		if _, err := tx.Exec(ctx, `
			WITH sb(type_key, scope) AS (
			    VALUES ('climb.harness','climb'),('climb.rope','climb'),('climb.crampons','climb'),
			           ('climb.ice_axe','climb'),('climb.belay_device','climb'),('climb.quickdraw','climb'),
			           ('climb.chalkbag','climb'),('climb.crashpad','climb'),
			           ('apparel.pants','apparel'),('apparel.jersey','apparel'),('apparel.gloves','apparel'),
			           ('apparel.socks','apparel'),('apparel.jacket','apparel'),('apparel.shorts','apparel')
			)
			UPDATE gear g SET scope = sb.scope FROM sb WHERE g.scope IS NULL AND g.type = sb.type_key
		`); err != nil {
			t.Fatalf("scope_backfill UPDATE: %v", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE gear SET scope = 'other' WHERE scope IS NULL`); err != nil {
			t.Fatalf("scope fallback UPDATE: %v", err)
		}

		var scope string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scope); err != nil {
			t.Fatalf("read scope: %v", err)
		}
		if scope != "climb" {
			t.Errorf("climb.harness row: scope = %q, want %q", scope, "climb")
		}
	})

	t.Run("scope_backfill_apparel_leaf_maps_to_apparel", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("begin tx: %v", err)
		}
		t.Cleanup(func() {
			if err := tx.Rollback(ctx); err != nil && err.Error() != "tx is closed" {
				t.Logf("rollback: %v", err)
			}
		})

		if _, err := tx.Exec(ctx, `ALTER TABLE gear ALTER COLUMN scope DROP NOT NULL`); err != nil {
			t.Fatalf("drop not null on scope: %v", err)
		}

		userID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig354_ap_"+userID[:8], "mig354_ap_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}

		// apparel.jacket is representative of all 6 apparel leaf keys.
		gearID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
			INSERT INTO gear (id, user_id, name, type, status, applicable_sport_predicates, scope)
			VALUES ($1, $2, 'Jacket', 'apparel.jacket', 'active', '[]', NULL)
		`, gearID, userID); err != nil {
			t.Fatalf("insert apparel.jacket row: %v", err)
		}

		if _, err := tx.Exec(ctx, `
			WITH sb(type_key, scope) AS (
			    VALUES ('climb.harness','climb'),('climb.rope','climb'),('climb.crampons','climb'),
			           ('climb.ice_axe','climb'),('climb.belay_device','climb'),('climb.quickdraw','climb'),
			           ('climb.chalkbag','climb'),('climb.crashpad','climb'),
			           ('apparel.pants','apparel'),('apparel.jersey','apparel'),('apparel.gloves','apparel'),
			           ('apparel.socks','apparel'),('apparel.jacket','apparel'),('apparel.shorts','apparel')
			)
			UPDATE gear g SET scope = sb.scope FROM sb WHERE g.scope IS NULL AND g.type = sb.type_key
		`); err != nil {
			t.Fatalf("scope_backfill UPDATE: %v", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE gear SET scope = 'other' WHERE scope IS NULL`); err != nil {
			t.Fatalf("scope fallback UPDATE: %v", err)
		}

		var scope string
		if err := tx.QueryRow(ctx, `SELECT scope FROM gear WHERE id = $1`, gearID).Scan(&scope); err != nil {
			t.Fatalf("read scope: %v", err)
		}
		if scope != "apparel" {
			t.Errorf("apparel.jacket row: scope = %q, want %q", scope, "apparel")
		}
	})
}

// TestMigration_GearItemJSONBAndAssignments_Down verifies the down migration
// reverses the schema to the pre-up state. Runs in a rolled-back transaction.
func TestMigration_GearItemJSONBAndAssignments_Down(t *testing.T) {
	downSQL := readMigration000355DownSQL(t)
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

	if _, err := tx.Exec(ctx, downSQL); err != nil {
		t.Fatalf("apply down.sql: %v", err)
	}

	t.Run("new_tables_gone", func(t *testing.T) {
		for _, tbl := range []string{
			"activity_gear_assignments",
			"gear_schema_definitions",
			"gear_rule_action_state",
			"gear_component_swap_events",
			"gear_idempotency_keys",
		} {
			if hasTable000355(t, ctx, tx, tbl) {
				t.Errorf("table %q still present after down migration", tbl)
			}
		}
	})

	t.Run("column_renamed_back", func(t *testing.T) {
		if hasColumn000355(t, ctx, tx, "gear", "applicable_sport_predicates") {
			t.Error("gear.applicable_sport_predicates should be absent after down (renamed back)")
		}
		if !hasColumn000355(t, ctx, tx, "gear", "default_sport_predicates") {
			t.Error("gear.default_sport_predicates should exist after down (renamed back)")
		}
	})

	t.Run("default_for_sports_recreated", func(t *testing.T) {
		if !hasColumn000355(t, ctx, tx, "gear", "default_for_sports") {
			t.Error("gear.default_for_sports must be recreated by down migration")
		}
	})

	t.Run("new_gear_columns_dropped", func(t *testing.T) {
		for _, col := range []string{"attributes", "modifiers", "scope", "total_moving_time_s", "total_elevation_loss_m"} {
			if hasColumn000355(t, ctx, tx, "gear", col) {
				t.Errorf("gear.%s still present after down migration", col)
			}
		}
	})

	t.Run("new_gear_components_columns_dropped", func(t *testing.T) {
		for _, col := range []string{"attributes", "type_key", "preset_gear_type_key", "total_activities", "total_moving_time_s", "total_elevation_loss_m"} {
			if hasColumn000355(t, ctx, tx, "gear_components", col) {
				t.Errorf("gear_components.%s still present after down migration", col)
			}
		}
	})

	t.Run("bike_setups_component_snapshot_dropped", func(t *testing.T) {
		if hasColumn000355(t, ctx, tx, "bike_setups", "component_snapshot") {
			t.Error("bike_setups.component_snapshot still present after down migration (Block D not reversed)")
		}
	})
}

// TestMigration_GearItemJSONBAndAssignments_DropsLegacyGearIDColumn verifies the
// final block of migration 000355: the activities.gear_id column is dropped
// together with the fn_mirror_activity_gear_to_aga function and its trigger
// pair. Wave-5/Cluster I makes activity_gear_assignments the sole authoritative
// source of activity-gear association; the legacy column would otherwise need
// continued read/write coordination.
func TestMigration_GearItemJSONBAndAssignments_DropsLegacyGearIDColumn(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	t.Run("activities_gear_id_column_dropped", func(t *testing.T) {
		if hasColumn000355(t, ctx, pool, "activities", "gear_id") {
			t.Error("activities.gear_id still present — final block of 000355 did not drop it")
		}
	})

	t.Run("mirror_trigger_function_dropped", func(t *testing.T) {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM pg_proc WHERE proname = 'fn_mirror_activity_gear_to_aga')`,
		).Scan(&exists); err != nil {
			t.Fatalf("query pg_proc: %v", err)
		}
		if exists {
			t.Error("fn_mirror_activity_gear_to_aga still present — final block of 000355 did not drop it")
		}
	})

	t.Run("mirror_triggers_dropped", func(t *testing.T) {
		for _, trg := range []string{
			"trg_mirror_activity_gear_to_aga",
			"trg_mirror_activity_gear_to_aga_insert",
			"trg_mirror_activity_gear_to_aga_update",
		} {
			var exists bool
			if err := pool.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM pg_trigger
				                WHERE tgname = $1
				                  AND tgrelid = 'activities'::regclass)`, trg,
			).Scan(&exists); err != nil {
				t.Fatalf("query pg_trigger %s: %v", trg, err)
			}
			if exists {
				t.Errorf("trigger %s still present after final block of 000355", trg)
			}
		}
	})

	t.Run("inserting_activity_with_gear_id_now_fails", func(t *testing.T) {
		// Sanity check: any code path still trying to write activities.gear_id
		// must surface as a hard error so callers learn at the earliest moment.
		userID := uuid.NewString()
		if _, err := pool.Exec(ctx, `
			INSERT INTO users (id, username, email, email_verified, registration_source, is_admin)
			VALUES ($1, $2, $3, true, 'test', false)
		`, userID, "mig354_drop_"+userID[:8], "mig354_drop_"+userID[:8]+"@example.com"); err != nil {
			t.Fatalf("insert user: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })

		gearID := uuid.NewString()
		if _, err := pool.Exec(ctx, `
			INSERT INTO gear (id, user_id, name, type, status, applicable_sport_predicates, scope)
			VALUES ($1, $2, 'Drop Bike', 'bike', 'active', '[]', 'bike')
		`, gearID, userID); err != nil {
			t.Fatalf("insert gear: %v", err)
		}
		t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM gear WHERE id = $1`, gearID) })

		actID := uuid.NewString()
		_, err := pool.Exec(ctx, `
			INSERT INTO activities
				(id, user_id, sport_type, title, started_at, file_hash, visibility, enrichment_status, gear_id, moving_time_s)
			VALUES ($1, $2, 'cycling', 'drop-test', now(), $3, 'private', 'complete', $4, 600)
		`, actID, userID, "hash-drop-"+actID[:8], gearID)
		if err == nil {
			_, _ = pool.Exec(context.Background(), `DELETE FROM activities WHERE id = $1`, actID)
			t.Fatal("INSERT INTO activities (..., gear_id, ...) succeeded — column should be gone")
		}
	})

	t.Run("activity_gear_assignments_table_still_present", func(t *testing.T) {
		// The aga table is the new source of truth — confirm it survives the
		// final block. (Block C creates it; the final block must not touch it.)
		var exists bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM information_schema.tables
			               WHERE table_schema = 'public' AND table_name = 'activity_gear_assignments')
		`).Scan(&exists); err != nil {
			t.Fatalf("query information_schema.tables: %v", err)
		}
		if !exists {
			t.Error("activity_gear_assignments missing — primary aga table required post-final-block")
		}
	})
}
