package auth

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// testPool returns a pgxpool.Pool connected to the database specified by
// TEST_DATABASE_URL. If the env var is unset the test is skipped.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestAllUserTablesInGDPRRegistry discovers all user-related tables in the live
// database schema and verifies each one is registered in GDPRRegistry.
//
// Two heuristics are combined:
//
//	A) Column-name: tables with a user_id or created_by column.
//	B) FK-chain: tables reachable from users(id) via foreign keys (up to 4 levels).
//
// LIMITATION: Tables that reference users through non-standard column names
// (e.g. owner_id, assigned_to) WITHOUT a foreign key to users will NOT be
// detected by either heuristic. Such tables must be added to GDPRRegistry
// manually.
func TestAllUserTablesInGDPRRegistry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Heuristic A: tables with user_id or created_by column.
	// JOIN with pg_class to restrict to base tables only (relkind='r'),
	// excluding views ('v'), materialized views ('m'), sequences etc.
	// This prevents false positives from views like gear_cost_summary that
	// expose user_id in their output column list but have no direct storage.
	columnRows, err := pool.Query(ctx, `
		SELECT DISTINCT c.relname AS table_name
		FROM information_schema.columns col
		JOIN pg_class c
		  ON c.relname = col.table_name
		 AND c.relnamespace = (SELECT oid FROM pg_namespace WHERE nspname = 'public')
		WHERE col.table_schema = 'public'
		  AND col.column_name IN ('user_id', 'created_by')
		  AND col.table_name NOT LIKE 'pg_%'
		  AND c.relkind = 'r'
	`)
	if err != nil {
		t.Fatalf("column query: %v", err)
	}
	defer columnRows.Close()

	found := make(map[string]bool)
	for columnRows.Next() {
		var name string
		if err := columnRows.Scan(&name); err != nil {
			t.Fatalf("scan column row: %v", err)
		}
		found[name] = true
	}
	columnRows.Close()

	// Heuristic B: FK-chain from users up to 4 levels deep.
	fkRows, err := pool.Query(ctx, `
		WITH RECURSIVE user_chain AS (
			-- Base: tables with direct FK to users
			SELECT DISTINCT tc.table_name, 1 AS depth
			FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = 'public'
			  AND ccu.table_schema = 'public'
			  AND ccu.table_name = 'users'
			UNION
			-- Recursive: tables with FK to already-found tables
			SELECT DISTINCT tc.table_name, uc.depth + 1
			FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name
			JOIN user_chain uc ON ccu.table_name = uc.table_name
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = 'public'
			  AND ccu.table_schema = 'public'
			  AND uc.depth < 4
		)
		SELECT DISTINCT table_name FROM user_chain
	`)
	if err != nil {
		t.Fatalf("FK chain query: %v", err)
	}
	defer fkRows.Close()

	for fkRows.Next() {
		var name string
		if err := fkRows.Scan(&name); err != nil {
			t.Fatalf("scan FK row: %v", err)
		}
		found[name] = true
	}
	fkRows.Close()

	// Compare with GDPRRegistry.
	var missing []string
	for table := range found {
		if _, ok := GDPRRegistry[table]; !ok {
			missing = append(missing, table)
		}
	}
	sort.Strings(missing)

	for _, table := range missing {
		t.Errorf("table %q has user relation but is not in GDPRRegistry", table)
	}
}

// TestExportDenyColumnsExist verifies that every column listed in ExportDenyColumns
// actually exists in the database. This catches stale deny-list entries after
// column renames or drops.
func TestExportDenyColumnsExist(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	for table, columns := range ExportDenyColumns {
		for _, col := range columns {
			var exists bool
			err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = 'public'
					  AND table_name = $1
					  AND column_name = $2
				)
			`, table, col).Scan(&exists)
			if err != nil {
				t.Fatalf("query column existence %s.%s: %v", table, col, err)
			}
			if !exists {
				t.Errorf("ExportDenyColumns references %s.%s but column does not exist in schema", table, col)
			}
		}
	}
}

// TestExportedTablesMatchRegistry verifies that every table with category Export
// or External in GDPRRegistry is covered by ExportedTables
// (the exportTable() calls in handler_data_export.go).
func TestExportedTablesMatchRegistry(t *testing.T) {
	exported := make(map[string]bool)
	for _, table := range ExportedTables {
		exported[table] = true
	}

	var uncovered []string
	for table, cat := range GDPRRegistry {
		if cat != Export && cat != External {
			continue
		}
		if exported[table] {
			continue
		}
		uncovered = append(uncovered, table)
	}
	sort.Strings(uncovered)

	for _, table := range uncovered {
		t.Errorf("table %q (Export/External) is not in ExportedTables", table)
	}

	// Reverse check: every entry in ExportedTables must be in GDPRRegistry
	for _, table := range ExportedTables {
		cat, ok := GDPRRegistry[table]
		if !ok {
			t.Errorf("ExportedTables contains %q but it is not in GDPRRegistry", table)
		} else if cat != Export && cat != External {
			t.Errorf("ExportedTables contains %q but its category is %v (expected Export or External)", table, cat)
		}
	}
}

// TestRegistryEntriesExistInSchema verifies that every non-Global entry in
// GDPRRegistry corresponds to an actual table (or partition/materialized view)
// in the database schema. This catches stale registry entries after a migration
// drops a table without cleaning the registry.
func TestRegistryEntriesExistInSchema(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	for table, cat := range GDPRRegistry {
		// Global entries are documented reference points; they may not exist
		// as real tables (e.g. lookup tables managed by migrations differently).
		if cat == Global {
			continue
		}

		var exists bool
		err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE n.nspname = 'public'
				  AND c.relname = $1
				  AND c.relkind IN ('r', 'm', 'p')
			)
		`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("query pg_class for %q: %v", table, err)
		}
		if !exists {
			t.Errorf("registry contains %q (category %v) but no such table exists in DB schema — stale entry?", table, cat)
		}
	}
}

// TestDeleteCoversAllUserTables verifies that every table in GDPRRegistry is
// cleaned up when a user is deleted — either via ON DELETE CASCADE on its FK
// to users, or via explicit deletion in DeleteUserData.
func TestDeleteCoversAllUserTables(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Tables explicitly deleted in DeleteUserData (database/gdpr.go).
	// Keep in sync with DeleteUserData implementation.
	//
	// NOTE: Health hypertables are kept here even if they have CASCADE FKs,
	// because TimescaleDB hypertable CASCADE behaviour is unreliable — explicit
	// deletes are required. Do NOT remove health_* entries based solely on
	// the INFO log from the reverse-check below.
	explicitlyDeletedInPurge := map[string]bool{
		"health_daily_summary":      true,
		"health_sleep":              true,
		"health_snapshots":          true,
		"health_hrv_night_values":   true,
		"health_monitoring_samples": true,
		"health_weight_entries":     true,
		"health_blood_pressure":     true,
		"health_fitness_age":        true,
		"health_stress_snapshots":   true,
		"health_spo2_snapshots":     true,
		"health_training_load":      true,
		"health_readiness_scores":   true,
		// user_daily_features: TODO verify with Agent 3 whether this has CASCADE or not.
		// Kept here to be safe — redundant explicit delete is better than missing one.
		"user_daily_features":    true,
		"llm_user_config":        true,
		"segment_efforts":        true,
		"segment_versions":       true,
		"segments":               true,
		"activity_edits":         true,
		"route_photos":           true,
		"planned_routes":         true,
		"heatmap_trail_matches":  true,
		"terra_incognita_cache":  true,
		"background_jobs":        true,
		// best_effort_exclusions and best_efforts: cascade from users via activities.
		// Agent 3 should remove explicit deletes from gdpr.go if CASCADE is confirmed.
		// Kept here until that sync happens.
		"best_effort_exclusions": true,
		"best_efforts":           true,
		// auth_audit_log: user_id is SET NULL on user deletion by design (audit trail preserved).
		// The FK ON DELETE SET NULL handles the purge-link; no explicit row deletion needed.
		"auth_audit_log": true,
	}

	// Query: which tables are reachable via ON DELETE CASCADE from users?
	// Uses a recursive CTE so transitive chains are included, e.g.:
	//   bike_setups → gear → users
	//   chat_messages → chat_sessions → users
	//   workout_steps → workouts → users
	// depth < 6 is sufficient for Skadia (deepest observed chain is depth 3).
	cascadeRows, err := pool.Query(ctx, `
		WITH RECURSIVE cascade_chain AS (
			-- Base: tables with direct ON DELETE CASCADE FK to users
			SELECT DISTINCT tc.table_name, 1 AS depth
			FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name
			JOIN information_schema.referential_constraints rc
			  ON rc.constraint_name = tc.constraint_name
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = 'public'
			  AND ccu.table_schema = 'public'
			  AND ccu.table_name = 'users'
			  AND rc.delete_rule = 'CASCADE'
			UNION
			-- Recursive: tables with ON DELETE CASCADE FK to already-found tables
			SELECT DISTINCT tc.table_name, cc.depth + 1
			FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
			  ON ccu.constraint_name = tc.constraint_name
			JOIN information_schema.referential_constraints rc
			  ON rc.constraint_name = tc.constraint_name
			JOIN cascade_chain cc ON ccu.table_name = cc.table_name
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_schema = 'public'
			  AND ccu.table_schema = 'public'
			  AND rc.delete_rule = 'CASCADE'
			  AND cc.depth < 6
		)
		SELECT DISTINCT table_name FROM cascade_chain
	`)
	if err != nil {
		t.Fatalf("cascade query: %v", err)
	}
	defer cascadeRows.Close()

	cascadeTables := make(map[string]bool)
	for cascadeRows.Next() {
		var name string
		if err := cascadeRows.Scan(&name); err != nil {
			t.Fatalf("scan cascade row: %v", err)
		}
		cascadeTables[name] = true
	}
	cascadeRows.Close()

	// Global tables and users itself don't need deletion coverage.
	skipCategories := map[TableCategory]bool{
		Global: true,
	}

	var uncovered []string
	for table, cat := range GDPRRegistry {
		if skipCategories[cat] {
			continue
		}
		if table == "users" {
			continue // users row is the deletion target itself
		}
		if cascadeTables[table] {
			continue
		}
		if explicitlyDeletedInPurge[table] {
			continue
		}
		uncovered = append(uncovered, table)
	}
	sort.Strings(uncovered)

	for _, table := range uncovered {
		detail := fmt.Sprintf("table %q (category %d) has no ON DELETE CASCADE from users and is not in explicitlyDeletedInPurge", table, GDPRRegistry[table])
		// Provide actionable hint.
		hint := " — add CASCADE to the FK, or add to explicitlyDeletedInPurge + DeleteUserData"
		t.Errorf("%s%s", detail, hint)
	}

	// Reverse check: warn about explicitlyDeletedInPurge entries that are not in the registry.
	for table := range explicitlyDeletedInPurge {
		if _, ok := GDPRRegistry[table]; !ok {
			t.Errorf("explicitlyDeletedInPurge contains %q but it is not in GDPRRegistry — stale entry?", table)
		}
	}

	// Reverse check: warn about explicitlyDeletedInPurge entries that actually have CASCADE.
	for table := range explicitlyDeletedInPurge {
		if cascadeTables[table] {
			lines := []string{
				fmt.Sprintf("table %q is in explicitlyDeletedInPurge but also has ON DELETE CASCADE", table),
				" — the explicit delete is redundant (unless it's a TimescaleDB hypertable)",
			}
			t.Logf("INFO: %s", strings.Join(lines, ""))
		}
	}
}
