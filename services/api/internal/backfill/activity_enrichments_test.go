package backfill

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/google/uuid"
)

func activityEnrichmentsTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}

	db, err := database.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func insertBackfillTestUser(t *testing.T, db *database.DB) string {
	t.Helper()

	userID := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO users (id, username, email, email_verified, registration_source)
		VALUES ($1, $2, $3, true, 'test')
	`, userID, "backfill_"+userID[:8], "backfill_"+userID[:8]+"@example.com")
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})
	return userID
}

func insertBackfillTestActivity(t *testing.T, db *database.DB, userID string, startedAt time.Time) string {
	t.Helper()

	activityID := uuid.NewString()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO activities (id, user_id, title, sport_type, started_at, created_at)
		VALUES ($1, $2, $3, 'cycling', $4, $4)
	`, activityID, userID, "backfill test activity", startedAt)
	if err != nil {
		t.Fatalf("insert activity: %v", err)
	}
	return activityID
}

func TestListActivityEnrichmentBackfillCandidates_SurfaceRowsDoNotCountAsEnriched(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)

	surfaceOnlyID := insertBackfillTestActivity(t, db, userID, time.Now().Add(-2*time.Hour))
	enrichedID := insertBackfillTestActivity(t, db, userID, time.Now().Add(-time.Hour))

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO activity_enrichments (activity_id, category, type, percentage, length_m)
		VALUES ($1, 'surface', 'asphalt', 100, 50)
	`, surfaceOnlyID)
	if err != nil {
		t.Fatalf("insert surface enrichment: %v", err)
	}

	_, err = db.Pool.Exec(ctx, `
		INSERT INTO activity_enrichments (activity_id, category, type, percentage)
		VALUES ($1, 'forest_pct', '', 42)
	`, enrichedID)
	if err != nil {
		t.Fatalf("insert enrichment-category row: %v", err)
	}

	candidates, err := listActivityEnrichmentBackfillCandidates(ctx, db, false)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}

	if !candidateContains(candidates, surfaceOnlyID) {
		t.Fatalf("surface-only activity %s should still be a candidate", surfaceOnlyID)
	}
	if candidateContains(candidates, enrichedID) {
		t.Fatalf("activity %s with forest_pct should not be a candidate without force", enrichedID)
	}

	forced, err := listActivityEnrichmentBackfillCandidates(ctx, db, true)
	if err != nil {
		t.Fatalf("list forced candidates: %v", err)
	}
	if !candidateContains(forced, enrichedID) {
		t.Fatalf("force=true should include enriched activity %s", enrichedID)
	}
}

func TestRunActivityEnrichmentsBackfill_DryRunDoesNotCreateSettings(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	insertBackfillTestActivity(t, db, userID, time.Now())

	if err := RunActivityEnrichmentsBackfill(ctx, db, nil, BackfillOptions{DryRun: true}); err != nil {
		t.Fatalf("dry-run backfill: %v", err)
	}

	var settingsRows int
	if err := db.Pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM user_enrichment_settings WHERE user_id = $1", userID,
	).Scan(&settingsRows); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if settingsRows != 0 {
		t.Fatalf("dry-run should not create user_enrichment_settings, got %d rows", settingsRows)
	}
}

func TestRunActivityEnrichmentsBackfill_RefusesWhenTileBuildRunning(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	jobID := uuid.NewString()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status)
		VALUES ($1, $2, 'tile_build', 'running')
	`, jobID, userID)
	if err != nil {
		t.Fatalf("insert running tile_build: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	err = RunActivityEnrichmentsBackfill(ctx, db, nil, BackfillOptions{DryRun: true})
	if !errors.Is(err, ErrTileBuildRunning) {
		t.Fatalf("expected ErrTileBuildRunning, got %v", err)
	}
}

func TestRunActivityEnrichmentsBackfill_RefusesWhenBackfillRunning(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	jobID := uuid.NewString()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status)
		VALUES ($1, $2, $3, 'running')
	`, jobID, userID, activityEnrichmentsJobType)
	if err != nil {
		t.Fatalf("insert running backfill: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	err = RunActivityEnrichmentsBackfill(ctx, db, nil, BackfillOptions{DryRun: true})
	if !errors.Is(err, ErrBackfillRunning) {
		t.Fatalf("expected ErrBackfillRunning, got %v", err)
	}
}

func TestRunActivityEnrichmentsBackfill_AllowsOwnedBackgroundJob(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	jobID := uuid.NewString()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status)
		VALUES ($1, $2, $3, 'running')
	`, jobID, userID, activityEnrichmentsJobType)
	if err != nil {
		t.Fatalf("insert owned running backfill: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	err = RunActivityEnrichmentsBackfill(ctx, db, nil, BackfillOptions{DryRun: true, OwnedJobID: jobID})
	if err != nil {
		t.Fatalf("owned running job should not self-block: %v", err)
	}

	otherUserID := insertBackfillTestUser(t, db)
	otherJobID := uuid.NewString()
	_, err = db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status)
		VALUES ($1, $2, $3, 'running')
	`, otherJobID, otherUserID, activityEnrichmentsJobType)
	if err != nil {
		t.Fatalf("insert other running backfill: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", otherJobID)
	})

	err = RunActivityEnrichmentsBackfill(ctx, db, nil, BackfillOptions{DryRun: true, OwnedJobID: jobID})
	if !errors.Is(err, ErrBackfillRunning) {
		t.Fatalf("expected ErrBackfillRunning for another running job, got %v", err)
	}
}

// TestWriteBackfillResult_MetadataShape verifies that writeBackfillResult stores
// the expected JSON structure in the job metadata column.
func TestWriteBackfillResult_MetadataShape(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	jobID := uuid.NewString()

	_, err := db.Pool.Exec(ctx, `
		INSERT INTO background_jobs (id, user_id, job_type, status, progress_total)
		VALUES ($1, $2, $3, 'running', 10)
	`, jobID, userID, activityEnrichmentsJobType)
	if err != nil {
		t.Fatalf("insert job: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), "DELETE FROM background_jobs WHERE id = $1", jobID)
	})

	failedIDs := []string{"aaa", "bbb"}
	if err := writeBackfillResult(ctx, db, jobID, 8, 2, 10, failedIDs, false); err != nil {
		t.Fatalf("writeBackfillResult: %v", err)
	}

	var meta string
	if err := db.Pool.QueryRow(ctx,
		"SELECT metadata::text FROM background_jobs WHERE id = $1", jobID,
	).Scan(&meta); err != nil {
		t.Fatalf("read metadata: %v", err)
	}

	for _, want := range []string{
		`"processed":8`,
		`"failed":2`,
		`"total":10`,
		`"failed_ids":["aaa","bbb"]`,
	} {
		if !containsStr(meta, want) {
			t.Fatalf("metadata missing %q, got: %s", want, meta)
		}
	}
}

// TestWriteBackfillResult_AbortedReturnsError verifies that writeBackfillResult
// returns a non-nil error when the backfill is aborted due to high failure rate.
func TestWriteBackfillResult_AbortedReturnsError(t *testing.T) {
	// No DB needed — use empty jobID to skip DB write.
	err := writeBackfillResult(context.Background(), nil, "", 3, 7, 10, nil, true)
	if err == nil {
		t.Fatal("expected error for aborted=true, got nil")
	}
}

// TestEnrichOneCandidate_NoSettings verifies that enrichOneCandidate treats
// nil settings as a no-op (not an error) to avoid counting disabled-users
// as failures.
func TestEnrichOneCandidate_NoSettings(t *testing.T) {
	db := activityEnrichmentsTestDB(t)
	ctx := context.Background()
	userID := insertBackfillTestUser(t, db)
	actID := insertBackfillTestActivity(t, db, userID, time.Now())

	// No user_enrichment_settings row → GetEnrichmentSettings returns nil → should be no-op.
	err := enrichOneCandidate(ctx, db, nil, activityEnrichmentBackfillCandidate{
		ActivityID: actID,
		UserID:     userID,
	})
	if err != nil {
		t.Fatalf("enrichOneCandidate with nil settings should succeed silently, got: %v", err)
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

func candidateContains(candidates []activityEnrichmentBackfillCandidate, activityID string) bool {
	for _, c := range candidates {
		if c.ActivityID == activityID {
			return true
		}
	}
	return false
}
