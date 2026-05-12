package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/database/testdb"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestGDPRArtifactCapacityRejectsPerUserActiveBytes(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{GDPRArtifactBackend: "local", GDPRArtifactUserActiveBytes: 1000},
		ArtifactStore: store,
		shutdownCtx:   canceledContext(),
	}

	jobOne := uuid.NewString()
	insertCapacityTestJob(t, pool, jobOne, userID, "data_import", "failed")
	if err := h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         jobOne,
		UserID:        userID,
		Backend:       "local",
		Kind:          "import",
		ReservedBytes: 800,
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("reserve first job: %v", err)
	}

	jobTwo := uuid.NewString()
	insertCapacityTestJob(t, pool, jobTwo, userID, "data_import", "failed")
	err = h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         jobTwo,
		UserID:        userID,
		Backend:       "local",
		Kind:          "import",
		ReservedBytes: 300,
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if !errors.Is(err, gdprartifacts.ErrCapacityExceeded) {
		t.Fatalf("second reservation error = %v, want ErrCapacityExceeded", err)
	}
}

func TestGDPRArtifactCapacityCommitAndRelease(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{GDPRArtifactBackend: "local", GDPRArtifactUserActiveBytes: 1000},
		ArtifactStore: store,
		shutdownCtx:   canceledContext(),
	}
	jobID := uuid.NewString()
	insertCapacityTestJob(t, pool, jobID, userID, "gdpr_export", "failed")
	if err := h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         jobID,
		UserID:        userID,
		Backend:       "local",
		Kind:          "export",
		ReservedBytes: 800,
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := h.commitGDPRArtifactCapacity(ctx, jobID, userID, 512); err != nil {
		t.Fatalf("commit capacity: %v", err)
	}
	if err := h.releaseGDPRArtifactCapacity(ctx, jobID, userID, 512); err != nil {
		t.Fatalf("release capacity: %v", err)
	}

	var reserved, committed int64
	var released bool
	if err := pool.QueryRow(ctx, `
		SELECT reserved_bytes, committed_bytes, released_at IS NOT NULL
		FROM gdpr_artifact_reservations
		WHERE job_id = $1`, jobID).Scan(&reserved, &committed, &released); err != nil {
		t.Fatalf("query reservation: %v", err)
	}
	if reserved != 800 || committed != 512 || !released {
		t.Fatalf("reservation = reserved=%d committed=%d released=%v", reserved, committed, released)
	}
}

func TestGDPRArtifactCapacityRejectsS3ReservationBudget(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	h := &Handler{
		DB: &database.DB{Pool: pool},
		Config: &config.Config{
			GDPRArtifactBackend:         "s3",
			GDPRArtifactUserActiveBytes: 10_000,
			GDPRArtifactS3QuotaBytes:    1000,
		},
		shutdownCtx: canceledContext(),
	}

	jobOne := uuid.NewString()
	insertCapacityTestJob(t, pool, jobOne, userID, "gdpr_export", "failed")
	if err := h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         jobOne,
		UserID:        userID,
		Backend:       "s3",
		Kind:          "export",
		ReservedBytes: 800,
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("reserve first S3 job: %v", err)
	}

	jobTwo := uuid.NewString()
	insertCapacityTestJob(t, pool, jobTwo, userID, "gdpr_export", "failed")
	err := h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         jobTwo,
		UserID:        userID,
		Backend:       "s3",
		Kind:          "export",
		ReservedBytes: 300,
		ExpiresAt:     time.Now().Add(time.Hour),
	})
	if !errors.Is(err, gdprartifacts.ErrCapacityExceeded) {
		t.Fatalf("second S3 reservation error = %v, want ErrCapacityExceeded", err)
	}
}

func TestSingleZipImportRejectsActiveArtifactBudgetBeforeArtifactWrite(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	local, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	store := &recordingArtifactStore{LocalStore: local}
	h := &Handler{
		DB:            &database.DB{Pool: pool},
		Config:        &config.Config{GDPRArtifactBackend: "local", GDPRArtifactUserActiveBytes: 1000},
		ArtifactStore: store,
		shutdownCtx:   canceledContext(),
	}

	preexistingJob := uuid.NewString()
	insertCapacityTestJob(t, pool, preexistingJob, userID, "gdpr_export", "failed")
	if err := h.reserveGDPRArtifactCapacity(ctx, gdprArtifactCapacityRequest{
		JobID:         preexistingJob,
		UserID:        userID,
		Backend:       "local",
		Kind:          "export",
		ReservedBytes: 900,
		ExpiresAt:     time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("reserve preexisting job: %v", err)
	}

	req := newDataImportUploadRequest(t, userID, minimalImportZip(t))
	w := httptest.NewRecorder()
	h.DataImport(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("DataImport status = %d body=%s", w.Code, w.Body.String())
	}
	if store.createWriterCalls != 0 {
		t.Fatalf("artifact writer calls = %d, want 0 before quota admission", store.createWriterCalls)
	}
}

func TestDataExportAdmissionRejectsEstimatedArtifactBudget(t *testing.T) {
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	h := &Handler{
		DB: &database.DB{Pool: pool},
		Config: &config.Config{
			GDPRArtifactBackend:               "local",
			GDPRArtifactUserActiveBytes:       1024,
			GDPRExportMultipartThresholdBytes: 1 << 30,
		},
		ArtifactStore: store,
		ShadowFS:      &testExportShadowFS{files: []testExportFile{{zipPath: "activity/a.fit", data: []byte("abc")}}},
		shutdownCtx:   canceledContext(),
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/data-export", nil).
		WithContext(context.WithValue(context.Background(), middleware.UserIDKey, userID))
	w := httptest.NewRecorder()
	h.DataExport(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("DataExport status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestStaleExportCleanupDeletesArtifactPrefixBeforeReleasingReservation(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	jobID := uuid.NewString()
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "export.zip",
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte("orphaned export")); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("commit artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		VALUES ($1, 'gdpr_export', $2, 'queued', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '31 minutes', '{}'::jsonb)`,
		jobID, userID); err != nil {
		t.Fatalf("insert stale export job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO gdpr_artifact_reservations
			(job_id, user_id, backend, kind, reserved_bytes, expires_at)
		VALUES ($1, $2, 'local', 'export', 4096, NOW() + INTERVAL '1 hour')`,
		jobID, userID); err != nil {
		t.Fatalf("insert export reservation: %v", err)
	}

	h := &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store}
	h.reconcileExpiredArtifacts(ctx)

	var status string
	var released bool
	if err := pool.QueryRow(ctx, `
		SELECT j.status, r.released_at IS NOT NULL
		FROM background_jobs j
		JOIN gdpr_artifact_reservations r ON r.job_id = j.id
		WHERE j.id = $1`, jobID).Scan(&status, &released); err != nil {
		t.Fatalf("query stale cleanup result: %v", err)
	}
	if status != "failed" || !released {
		t.Fatalf("cleanup result status=%q released=%v, want failed/released", status, released)
	}
	if _, err := store.Open(ctx, ref, nil); err == nil {
		t.Fatal("stale export artifact prefix is still readable after cleanup")
	}
}

func TestStaleSingleImportCleanupDeletesArtifactPrefixAndReleasesReservation(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	clearActiveDataImportJobsForTest(t, pool)
	userID := createArtifactExportUser(t, pool)
	jobID := uuid.NewString()
	store, err := gdprartifacts.NewLocalStore(t.TempDir()+"/exports", t.TempDir()+"/imports")
	if err != nil {
		t.Fatalf("NewLocalStore: %v", err)
	}
	writer, err := store.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "import",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "metadata.zip",
		ContentType: "application/zip",
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateWriter: %v", err)
	}
	if _, err := writer.Write([]byte("orphaned import")); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		t.Fatalf("commit artifact: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		VALUES ($1, 'data_import', $2, 'uploading', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '31 minutes', '{}'::jsonb)`,
		jobID, userID); err != nil {
		t.Fatalf("insert stale import job: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO gdpr_artifact_reservations
			(job_id, user_id, backend, kind, reserved_bytes, expires_at)
		VALUES ($1, $2, 'local', 'import', 4096, NOW() + INTERVAL '1 hour')`,
		jobID, userID); err != nil {
		t.Fatalf("insert import reservation: %v", err)
	}

	h := &Handler{DB: &database.DB{Pool: pool}, ArtifactStore: store}
	h.cleanupStaleImportJobs(ctx)

	var status string
	var released bool
	if err := pool.QueryRow(ctx, `
		SELECT j.status, r.released_at IS NOT NULL
		FROM background_jobs j
		JOIN gdpr_artifact_reservations r ON r.job_id = j.id
		WHERE j.id = $1`, jobID).Scan(&status, &released); err != nil {
		t.Fatalf("query stale import cleanup result: %v", err)
	}
	if status != "failed" || !released {
		t.Fatalf("cleanup result status=%q released=%v, want failed/released", status, released)
	}
	if _, err := store.Open(ctx, ref, nil); err == nil {
		t.Fatal("stale import artifact prefix is still readable after cleanup")
	}
}

func insertCapacityTestJob(t *testing.T, pool *pgxpool.Pool, jobID, userID, jobType, status string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		jobID, jobType, userID, status); err != nil {
		t.Fatalf("insert background job: %v", err)
	}
}
