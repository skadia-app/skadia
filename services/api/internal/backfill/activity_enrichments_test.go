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

func candidateContains(candidates []activityEnrichmentBackfillCandidate, activityID string) bool {
	for _, c := range candidates {
		if c.ActivityID == activityID {
			return true
		}
	}
	return false
}
