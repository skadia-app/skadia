package dataimport

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFKMapCompleteness(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Query all FK columns from the public schema.
	rows, err := pool.Query(ctx, `
		SELECT tc.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = 'public'
		ORDER BY tc.table_name, kcu.column_name
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	// Build lookup from ExportedTableSpecs.
	exportedSet := make(map[string]bool)
	fkMap := make(map[string]map[string]bool)
	for _, spec := range ExportedTableSpecs() {
		exportedSet[spec.Name] = true
		fkSet := make(map[string]bool)
		for _, fk := range spec.FKFields {
			fkSet[fk] = true
		}
		// Deferred FKs are also covered (set NULL then updated in Phase 5b).
		for _, d := range spec.Deferred {
			fkSet[d] = true
		}
		// file_attachments.entity_id is resolved dynamically (polymorphic FK).
		if spec.Name == "file_attachments" {
			fkSet["entity_id"] = true
		}
		fkMap[spec.Name] = fkSet
	}

	// Tables that are explicitly skipped or handled outside the engine.
	skipTables := map[string]bool{
		"best_efforts": true,
		"import_log":   true,
		"users":        true,
		"user_files":   true, // handled via ShadowFS.Put()
	}

	// Columns that are handled implicitly (re-keyed via the user column path).
	skipColumns := map[string]bool{
		"user_id":    true,
		"created_by": true,
	}

	var missing []string
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			t.Fatal(err)
		}
		if !exportedSet[table] || skipTables[table] || skipColumns[col] {
			continue
		}
		fkSet := fkMap[table]
		if fkSet == nil || !fkSet[col] {
			missing = append(missing, table+"."+col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(missing) > 0 {
		t.Errorf("FK columns missing from fkFields/Deferred map (these IDs won't be remapped, causing data corruption):")
		for _, m := range missing {
			t.Errorf("  - %s", m)
		}
	}
}

// TestFileRefColumnsCompleteness verifies that every FK column pointing to
// user_files is listed in fileRefColumns. Missing entries would cause
// phantom UUID generation for files that weren't in the export.
func TestFileRefColumnsCompleteness(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Find all FK columns that reference user_files.
	rows, err := pool.Query(ctx, `
		SELECT tc.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = 'public'
			AND ccu.table_name = 'user_files'
		ORDER BY tc.table_name, kcu.column_name
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	// Build set of exported tables.
	exportedSet := make(map[string]bool)
	for _, spec := range ExportedTableSpecs() {
		exportedSet[spec.Name] = true
	}

	skipTables := map[string]bool{
		"user_files": true, // Self-reference or handled by ShadowFS
	}

	var missing []string
	for rows.Next() {
		var table, col string
		if err := rows.Scan(&table, &col); err != nil {
			t.Fatal(err)
		}
		if !exportedSet[table] || skipTables[table] {
			continue
		}
		if !isFileRefColumn(table, col) {
			missing = append(missing, table+"."+col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	if len(missing) > 0 {
		t.Errorf("FK columns referencing user_files missing from fileRefColumns (would generate phantom UUIDs):")
		for _, m := range missing {
			t.Errorf("  - %s", m)
		}
	}
}
