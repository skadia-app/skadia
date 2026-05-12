package migrations_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
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

func TestMigration_GDPRTransferActiveJobsBackfillsDuplicatesBeforeIndex(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()
	upBytes, err := os.ReadFile("000362_gdpr_transfer_active_jobs.up.sql")
	if err != nil {
		t.Fatalf("read up.sql: %v", err)
	}
	userID := uuid.NewString()
	jobOne := uuid.NewString()
	jobTwo := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "gdpr_active_"+userID[:8], "gdpr_active_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID)
	})
	if _, err := pool.Exec(ctx, `DROP INDEX IF EXISTS idx_bg_jobs_one_gdpr_transfer_active`); err != nil {
		t.Fatalf("drop active transfer index: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		VALUES
			($1, 'gdpr_export', $3, 'queued', NOW() - INTERVAL '2 minutes', NOW() - INTERVAL '2 minutes'),
			($2, 'data_import', $3, 'running', NOW() - INTERVAL '1 minute', NOW() - INTERVAL '1 minute')`,
		jobOne, jobTwo, userID); err != nil {
		t.Fatalf("insert duplicate active jobs: %v", err)
	}

	if _, err := pool.Exec(ctx, string(upBytes)); err != nil {
		t.Fatalf("apply up.sql with duplicate active jobs: %v", err)
	}

	var active int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM background_jobs
		WHERE user_id = $1
		  AND job_type IN ('gdpr_export', 'data_import')
		  AND status IN ('uploading', 'queued', 'running')`,
		userID).Scan(&active); err != nil {
		t.Fatalf("count active jobs: %v", err)
	}
	if active != 1 {
		t.Fatalf("active GDPR jobs = %d, want 1 after backfill", active)
	}
}
