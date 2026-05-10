package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration_ImportType_AddsColumnAndStatusQueued(t *testing.T) {
	upBytes, err := os.ReadFile("000999_import_type.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000999_import_type.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"ALTER TABLE import_log ADD COLUMN import_type TEXT",
		"'queued'",
		"CREATE INDEX import_log_user_id_import_type_idx",
		"ON import_log (user_id, import_type, created_at DESC)",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up.sql missing %q", needle)
		}
	}

	if strings.Contains(upSQL, "import_type IN (") {
		t.Fatalf("up.sql introduces an import_type CHECK constraint — that breaks the 'new types without migration' contract")
	}

	for _, needle := range []string{
		"DROP INDEX IF EXISTS import_log_user_id_import_type_idx",
		"DROP COLUMN IF EXISTS import_type",
		"UPDATE import_log SET status = 'pending' WHERE status = 'queued'",
	} {
		if !strings.Contains(downSQL, needle) {
			t.Fatalf("down.sql missing %q", needle)
		}
	}
}
