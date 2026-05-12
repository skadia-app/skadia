package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration_GDPRTransferActiveJobsDeclaresActiveTransferIndex(t *testing.T) {
	upBytes, err := os.ReadFile("000362_gdpr_transfer_active_jobs.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	downBytes, err := os.ReadFile("000362_gdpr_transfer_active_jobs.down.sql")
	if err != nil {
		t.Fatalf("read down.sql: %v", err)
	}
	upSQL := string(upBytes)
	for _, needle := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_bg_jobs_one_gdpr_transfer_active",
		"ON background_jobs (user_id)",
		"job_type IN ('gdpr_export', 'data_import')",
		"status IN ('uploading', 'queued', 'running')",
	} {
		if !strings.Contains(upSQL, needle) {
			t.Fatalf("up.sql missing %q", needle)
		}
	}
	if !strings.Contains(string(downBytes), "DROP INDEX IF EXISTS idx_bg_jobs_one_gdpr_transfer_active") {
		t.Fatalf("down.sql does not drop active GDPR transfer index")
	}
}
