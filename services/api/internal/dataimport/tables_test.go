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
