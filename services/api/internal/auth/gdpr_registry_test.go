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
//   A) Column-name: tables with a user_id or created_by column.
//   B) FK-chain: tables reachable from users(id) via foreign keys (up to 4 levels).
//
// LIMITATION: Tables that reference users through non-standard column names
// (e.g. owner_id, assigned_to) WITHOUT a foreign key to users will NOT be
// detected by either heuristic. Such tables must be added to GDPRRegistry
// manually.
func TestAllUserTablesInGDPRRegistry(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Heuristic A: tables with user_id or created_by column.
	columnRows, err := pool.Query(ctx, `
		SELECT DISTINCT table_name
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND column_name IN ('user_id', 'created_by')
		  AND table_name NOT LIKE 'pg_%'
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
// or External in GDPRRegistry is covered by the data export — either via
// ManuallyExportedTables (custom export logic) or via ExportedTables
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
		if ManuallyExportedTables[table] {
			continue
		}
		if exported[table] {
			continue
		}
		uncovered = append(uncovered, table)
	}
	sort.Strings(uncovered)

	for _, table := range uncovered {
		t.Errorf("table %q (Export/External) is not in ExportedTables and not in ManuallyExportedTables", table)
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

// TestDeleteCoversAllUserTables verifies that every table in GDPRRegistry is
// cleaned up when a user is deleted — either via ON DELETE CASCADE on its FK
// to users, or via explicit deletion in DeleteUserData.
func TestDeleteCoversAllUserTables(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()

	// Tables explicitly deleted in DeleteUserData (database/gdpr.go).
	// Keep in sync with DeleteUserData implementation.
	explicitlyDeletedInPurge := map[string]bool{
		"health_daily_summary":     true,
		"health_sleep":             true,
		"health_snapshots":         true,
		"health_hrv_night_values":  true,
		"health_monitoring_samples": true,
		"health_weight_entries":    true,
		"health_blood_pressure":    true,
		"health_fitness_age":       true,
		"health_stress_snapshots":  true,
		"health_spo2_snapshots":    true,
		"health_training_load":     true,
		"health_readiness_scores":  true,
		"user_daily_features":      true,
		"llm_user_config":          true,
		"segment_efforts":          true,
		"segment_versions":         true,
		"segments":                 true,
		"activity_edits":           true,
		"route_photos":             true,
		"planned_routes":           true,
		"heatmap_trail_matches":    true,
		"terra_incognita_cache":    true,
		"background_jobs":          true,
		"best_effort_exclusions":   true,
		"best_efforts":             true,
	}

	// Query: which tables have ON DELETE CASCADE from a FK pointing to users?
	cascadeRows, err := pool.Query(ctx, `
		SELECT DISTINCT tc.table_name
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
