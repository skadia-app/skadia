package migrations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
)

// Verifies the placeholder 000999 migration that widens idx_bg_jobs_one_running
// from (user_id, job_type) to (user_id, job_type, COALESCE(entity_id, ''))
// and from status='running' to status IN ('running','queued').
//
// Properties checked:
//  1. Same user + same job_type + DIFFERENT entity_id → both inserts succeed
//     (the old narrow index would have rejected the second one).
//  2. Same user + same job_type + SAME entity_id → second insert rejected
//     with unique_violation on idx_bg_jobs_one_running.
//  3. status='queued' is also covered by the index — a queued row blocks
//     a second running row for the same key.
//  4. NULL entity_id collapses to '' under COALESCE — two NULL inserts
//     for the same (user, job_type) collide.
//
// background_jobs.user_id is plain TEXT (no FK), so we don't need a real
// users row.
func TestMigration_WidenLiftRecomputeUnique_Coverage(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// Apply (or re-apply) the up-migration content. Idempotent via DROP IF EXISTS.
	apply := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `DROP INDEX IF EXISTS idx_bg_jobs_one_running`); err != nil {
			t.Fatalf("drop index: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			CREATE UNIQUE INDEX idx_bg_jobs_one_running
			    ON background_jobs (user_id, job_type, (COALESCE(entity_id, '')))
			    WHERE status IN ('running', 'queued')
		`); err != nil {
			t.Fatalf("create widened index: %v", err)
		}
	}
	apply()
	apply() // idempotency

	// Use a unique user_id + job_type per test run so we don't collide with
	// other parallel tests on the per-worktree DB.
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	userID := "user-widen-" + suffix
	jobType := "test_widen_" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			`DELETE FROM background_jobs WHERE user_id = $1`, userID)
	})

	insert := func(t *testing.T, entityID *string, status string) error {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO background_jobs (id, user_id, job_type, entity_id, status, progress_total)
			VALUES ($1, $2, $3, $4, $5, 1)
		`, uuid.NewString(), userID, jobType, entityID, status)
		return err
	}

	// (1) DIFFERENT entity_id → both succeed.
	a := "activity-a"
	b := "activity-b"
	if err := insert(t, &a, "running"); err != nil {
		t.Fatalf("insert (entity=a, running): %v", err)
	}
	if err := insert(t, &b, "running"); err != nil {
		t.Fatalf("insert (entity=b, running) blocked by old narrow index: %v", err)
	}

	// (2) SAME entity_id → conflict on idx_bg_jobs_one_running.
	err := insert(t, &a, "running")
	if err == nil {
		t.Fatalf("insert (entity=a, running) succeeded twice — unique guard not enforcing per-entity")
	}
	if !strings.Contains(err.Error(), "idx_bg_jobs_one_running") {
		t.Errorf("expected unique_violation on idx_bg_jobs_one_running, got: %v", err)
	}

	// (3) Queued is covered. Mark previous rows completed, queue one, then
	// running insert with same key must collide.
	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', finished_at = NOW()
		 WHERE user_id = $1 AND job_type = $2`, userID, jobType); err != nil {
		t.Fatalf("clear running rows: %v", err)
	}
	c := "activity-c"
	if err := insert(t, &c, "queued"); err != nil {
		t.Fatalf("insert (entity=c, queued): %v", err)
	}
	err = insert(t, &c, "running")
	if err == nil {
		t.Fatalf("running insert after queued collision allowed — index does not cover 'queued'")
	}
	if !strings.Contains(err.Error(), "idx_bg_jobs_one_running") {
		t.Errorf("expected unique_violation on idx_bg_jobs_one_running for queued+running collision, got: %v", err)
	}

	// (4) NULL entity_id collapses to ''.
	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'completed', finished_at = NOW()
		 WHERE user_id = $1 AND job_type = $2`, userID, jobType); err != nil {
		t.Fatalf("clear running/queued: %v", err)
	}
	if err := insert(t, nil, "running"); err != nil {
		t.Fatalf("insert (entity=NULL, running): %v", err)
	}
	err = insert(t, nil, "running")
	if err == nil {
		t.Fatalf("two NULL-entity_id running inserts allowed — COALESCE collapse missing")
	}
}
