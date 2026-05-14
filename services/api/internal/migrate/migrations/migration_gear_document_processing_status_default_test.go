package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration_GearDocumentProcessingStatusDefault_SetsIdleAndBackfillsImages(t *testing.T) {
	upBytes, err := os.ReadFile("000368_gear_document_processing_status_default.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000368_gear_document_processing_status_default.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	upSQL := string(upBytes)
	downSQL := string(downBytes)

	for _, needle := range []string{
		"ALTER COLUMN processing_status SET DEFAULT 'idle'",
		"WHERE processing_status IS NULL",
		"WHERE processing_status NOT IN ('idle', 'pending', 'processed', 'failed')",
		"WHERE processing_status = 'pending'",
		"content_type LIKE 'image/%'",
		"gear_documents_processing_status_check",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up.sql missing %q", needle)
		}
	}
	if !strings.Contains(downSQL, "ALTER COLUMN processing_status SET DEFAULT 'pending'") {
		t.Fatal("down.sql must restore the previous default")
	}
	if !strings.Contains(downSQL, "DROP CONSTRAINT IF EXISTS gear_documents_processing_status_check") {
		t.Fatal("down.sql must drop the processing_status check constraint")
	}
}
