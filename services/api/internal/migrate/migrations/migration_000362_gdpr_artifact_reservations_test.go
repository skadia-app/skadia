package migrations_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestMigration_GDPRArtifactReservations_TableAndIndexes(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	var exists bool
	if err := pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_name = 'gdpr_artifact_reservations'
		)`).Scan(&exists); err != nil {
		t.Fatalf("query table: %v", err)
	}
	if !exists {
		t.Fatal("gdpr_artifact_reservations table is missing")
	}

	for _, index := range []string{
		"idx_gdpr_artifact_reservations_user_active",
		"idx_gdpr_artifact_reservations_backend_active",
		"idx_gdpr_artifact_reservations_expiry_active",
	} {
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_indexes
				WHERE tablename = 'gdpr_artifact_reservations'
				  AND indexname = $1
			)`, index).Scan(&exists); err != nil {
			t.Fatalf("query index %s: %v", index, err)
		}
		if !exists {
			t.Fatalf("index %s is missing", index)
		}
	}
}

func TestMigration_GDPRArtifactReservations_UserCascade(t *testing.T) {
	pool := migrationTestPool(t)
	ctx := context.Background()

	userID := uuid.NewString()
	jobID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')`,
		userID, "gdpr_res_"+userID[:8], "gdpr_res_"+userID[:8]+"@example.com"); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status, started_at, updated_at)
		VALUES ($1, $2, 'data_import', 'uploading', NOW(), NOW())`,
		jobID, userID); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO gdpr_artifact_reservations
			(job_id, user_id, backend, kind, reserved_bytes, expires_at)
		VALUES ($1, $2, 'local', 'import', 1024, NOW() + INTERVAL '1 hour')`,
		jobID, userID); err != nil {
		t.Fatalf("insert reservation: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gdpr_artifact_reservations WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if count != 0 {
		t.Fatalf("reservation count after user delete = %d, want 0", count)
	}
}
