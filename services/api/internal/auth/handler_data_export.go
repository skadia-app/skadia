package auth

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// isValidExportPath validates that a file path from DB metadata points to
// a legitimate export temp file. Defense-in-depth against DB manipulation.
func isValidExportPath(path string) bool {
	cleaned := filepath.Clean(path)
	return strings.HasPrefix(cleaned, os.TempDir()) && strings.Contains(filepath.Base(cleaned), "skadia-export-")
}

var staleCleanupOnce sync.Once

const dataExportDownloadTTL = 24 * time.Hour
const dataExportMaxStartsPer24h = 3
const dataExportDownloadSessionCookie = "skadia_gdpr_download"
const dataExportDownloadSessionTTL = time.Minute

func dataExportExpiresAt(updatedAt time.Time) time.Time {
	return updatedAt.Add(dataExportDownloadTTL)
}

func dataExportDownloadExpired(now, updatedAt time.Time) bool {
	return !now.Before(dataExportExpiresAt(updatedAt))
}

func clearDataExportFilePath(ctx context.Context, pool *pgxpool.Pool, jobID string) {
	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET metadata = metadata - 'file_path' WHERE id = $1`, jobID); err != nil {
		slog.Warn("gdpr export cleanup: clear file_path failed", "job_id", jobID, "error", err)
	}
}

// startStaleExportCleanup runs the stale export job cleanup every 5 minutes.
// Safe to call multiple times — only starts once.
func startStaleExportCleanup(ctx context.Context, pool *pgxpool.Pool) {
	staleCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			// Run once immediately at startup.
			cleanupStaleExportJobs(ctx, pool)
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					cleanupStaleExportJobs(ctx, pool)
				}
			}
		}()
	})
}

// cleanupStaleExportJobs removes files and marks stale export jobs as failed.
// Called periodically by the background timer started via startStaleExportCleanup.
//
// Two cleanup classes:
//
//  1. STALE: queued/running jobs that have not made progress in >30 min are
//     marked as 'failed' (their pod must have crashed). Their tmp file (if
//     any) is deleted.
//
//  2. EXPIRED: completed/failed jobs whose tmp file is older than 25 hours
//     are deleted. The 24h-cleanup goroutine started by executeExportJobOnce
//     would normally do this, but is lost on pod-restart — without this
//     fallback the file leaks indefinitely.
func cleanupStaleExportJobs(ctx context.Context, pool *pgxpool.Pool) {
	// Class 1: stale running/queued
	staleRows, err := pool.Query(ctx,
		`SELECT metadata->>'file_path' FROM background_jobs
		 WHERE job_type = 'gdpr_export' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'
		 AND metadata->>'file_path' IS NOT NULL`)
	if err != nil {
		slog.Error("gdpr export cleanup: query stale jobs failed", "error", err)
	} else {
		for staleRows.Next() {
			var path string
			if staleRows.Scan(&path) == nil && path != "" && isValidExportPath(path) {
				os.Remove(path)
			}
		}
		staleRows.Close()
	}

	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'failed', error = 'stale: pod crash', updated_at = NOW()
		 WHERE job_type = 'gdpr_export' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'`); err != nil {
		slog.Error("gdpr export cleanup: update stale jobs failed", "error", err)
	}

	// Class 2: expired completed/failed jobs (file leaked because the
	// in-process 24h goroutine was lost on pod-restart). Delete the file and
	// clear the file_path from metadata so the next sweep doesn't re-attempt.
	expiredRows, err := pool.Query(ctx,
		`SELECT id, metadata->>'file_path' FROM background_jobs
		 WHERE job_type = 'gdpr_export' AND status IN ('completed','failed')
		 AND updated_at < NOW() - INTERVAL '25 hours'
		 AND metadata->>'file_path' IS NOT NULL`)
	if err != nil {
		slog.Error("gdpr export cleanup: query expired jobs failed", "error", err)
		return
	}
	type expired struct{ id, path string }
	var expiredJobs []expired
	for expiredRows.Next() {
		var e expired
		if err := expiredRows.Scan(&e.id, &e.path); err == nil && e.path != "" {
			expiredJobs = append(expiredJobs, e)
		}
	}
	expiredRows.Close()

	for _, e := range expiredJobs {
		if isValidExportPath(e.path) {
			os.Remove(e.path)
		}
		// Strip file_path from metadata so we don't keep retrying.
		clearDataExportFilePath(ctx, pool, e.id)
	}
}

// DataExport handles POST /auth/data-export — starts a background GDPR export job.
func (h *Handler) DataExport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	ctx := r.Context()

	user, err := h.DB.GetUserByID(ctx, userID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}
	if user.DeletedAt != nil {
		httputil.WriteJSONNoCache(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}
	if !user.IsActive {
		errCode := "account_inactive"
		if user.PendingApproval {
			errCode = "account_pending_approval"
		}
		httputil.WriteJSONNoCache(w, http.StatusForbidden, map[string]string{"error": errCode})
		return
	}

	// Ensure the background stale-job cleanup timer is running (starts once per process).
	startStaleExportCleanup(h.shutdownCtx, h.DB.Pool)

	var recentStarts int
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs
		 WHERE user_id = $1 AND job_type = 'gdpr_export'
		 AND started_at > NOW() - INTERVAL '24 hours'`,
		userID).Scan(&recentStarts); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if recentStarts >= dataExportMaxStartsPer24h {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_daily_limit"})
		return
	}

	jobID := uuid.New().String()
	expiresAt := time.Now().UTC().Add(dataExportDownloadTTL)
	reservedBytes := h.estimateGDPRExportReservationBytes(ctx, userID)
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		slog.Error("gdpr export: begin admission tx failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	defer tx.Rollback(ctx)
	if err := lockGDPRTransferAdmission(ctx, tx); err != nil {
		slog.Error("gdpr export: admission lock failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	var userRunning bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM background_jobs
			WHERE user_id = $1
			  AND job_type IN ('gdpr_export', 'data_import')
			  AND status IN ('uploading', 'queued', 'running'))`,
		userID).Scan(&userRunning); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if userRunning {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}
	var serverCount int
	if err := tx.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs WHERE job_type = 'gdpr_export' AND status IN ('queued','running')`,
	).Scan(&serverCount); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if serverCount >= 3 {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "server_busy"})
		return
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		VALUES ($1, 'gdpr_export', $2, 'queued', NOW(), NOW())
		RETURNING true`, jobID, userID).Scan(new(bool))
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}
	if err != nil {
		slog.Error("gdpr export: insert job failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if err := h.reserveGDPRArtifactCapacityTx(ctx, tx, gdprArtifactCapacityRequest{
		JobID:         jobID,
		UserID:        userID,
		Backend:       h.gdprArtifactBackendName(),
		Kind:          "export",
		ReservedBytes: reservedBytes,
		ExpiresAt:     expiresAt,
	}); err != nil {
		if isArtifactCapacityError(err) {
			httputil.WriteJSONNoCache(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "artifact_quota_exceeded"})
			return
		}
		slog.Error("gdpr export: reserve artifact capacity failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("gdpr export: commit admission tx failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	go h.runExportJob(h.shutdownCtx, jobID, userID)

	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "data_export_requested",
		middleware.ClientIP(r), r.UserAgent(), nil)

	httputil.WriteJSONNoCache(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"status": "queued",
	})
}

// DataExportLatest handles GET /auth/data-export/latest — returns the latest export job for the current user.
func (h *Handler) DataExportLatest(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var jobID, status, startedAt string
	var updatedAt time.Time
	var errText *string
	var metadata []byte

	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id::text, status, started_at::text, COALESCE(updated_at, started_at), error, COALESCE(metadata, '{}'::jsonb)
		 FROM background_jobs
		 WHERE user_id = $1 AND job_type = 'gdpr_export'
		   AND (
		     status IN ('queued', 'running')
		     OR COALESCE(updated_at, started_at) > NOW() - INTERVAL '25 hours'
		   )
		 ORDER BY started_at DESC
		 LIMIT 1`,
		userID).Scan(&jobID, &status, &startedAt, &updatedAt, &errText, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if err != nil {
		slog.Error("gdpr export latest: query failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	metadata, unavailableReason := h.reconcileExportArtifactForStatus(r.Context(), jobID, userID, metadata)
	resp := buildDataExportStatusResponse(jobID, status, updatedAt, errText, metadata, time.Now())
	if unavailableReason == "artifact_backend_unavailable" {
		stripDataExportDownloadSessionFields(resp)
		resp["unavailable_reason"] = unavailableReason
	}
	resp["started_at"] = startedAt
	httputil.WriteJSONNoCache(w, http.StatusOK, resp)
}

// runExportJob transitions the job to running, executes the export, and marks it failed on error.
func (h *Handler) runExportJob(ctx context.Context, jobID, userID string) {
	h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'running', updated_at = NOW() WHERE id = $1`, jobID)

	if err := h.executeExportJob(ctx, jobID, userID); err != nil {
		slog.Error("gdpr export job failed", "job_id", jobID, "error", err)
		h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'failed', error = $2, updated_at = NOW() WHERE id = $1`, jobID, err.Error())
		if capacityErr := h.finalizeExportReservationAfterError(ctx, jobID, userID); capacityErr != nil {
			slog.Warn("gdpr export job: finalize artifact reservation after error failed", "job_id", jobID, "error", capacityErr)
		}
	}
}

// executeExportJob wraps executeExportJobOnce with serialization-failure retry.
func (h *Handler) executeExportJob(ctx context.Context, jobID, userID string) error {
	const maxRetries = 3
	for attempt := range maxRetries {
		err := h.executeExportJobOnce(ctx, jobID, userID)
		if err == nil {
			return nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "40001" {
			slog.Warn("gdpr export: serialization failure, retrying", "attempt", attempt+1, "job_id", jobID)
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		return err
	}
	return fmt.Errorf("gdpr export: max retries exceeded")
}

// skadiaExportVersion is the human-readable version tag embedded in
// every GDPR export manifest. Increment this string when the export format
// or export pipeline changes in a way that an importer should know about.
// Format: YYYY.MM (calendar versioning). Currently bumped manually; a future
// build-time -ldflags injection can replace this constant.
const skadiaExportVersion = "2026.04"

// executeExportJobOnce performs the export in a single REPEATABLE READ transaction.
//
// Architecture:
//   - Schema introspection runs PRE-TX via the connection pool (information_schema
//     reads do not need snapshot consistency, and a per-table data-query failure
//     inside the TX must NOT poison subsequent introspection reads — that's the
//     SQLSTATE 25P02 cascade we hit before).
//   - All data queries run inside one REPEATABLE READ ReadOnly TX so the export
//     is a consistent snapshot of the user's data.
//   - Per-table errors are collected; an export with any errors is marked as
//     `failed` (not `completed`), the partial ZIP is kept on disk for debug
//     download, and `background_jobs.metadata.export_errors` lists the failures.
func (h *Handler) executeExportJobOnce(ctx context.Context, jobID, userID string) error {
	// 1. Build the static table list (no DB).
	specs := gdprExportTables()
	healthTables := database.AllHealthTables()
	allNames := gdprAllTableNames(specs, healthTables)

	// 2a. Read schema migration version BEFORE introspection. This is the
	//     baseline we will compare against inside the TX (Finding #7 defense).
	//     On error we use 0 — a mismatch will then always trigger on any non-zero
	//     in-TX version, which is the conservative/safe direction.
	var preIntrospectMigration int
	if err := h.DB.Pool.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&preIntrospectMigration); err != nil {
		slog.Warn("gdpr export: could not read pre-introspect schema version, defaulting to 0", "error", err)
		preIntrospectMigration = 0
	}

	// 2b. Pre-TX column introspection via the pool. Failures here mean a table
	//     was renamed/dropped without the export code being updated; they are
	//     surfaced via the export_errors list, not by aborting the whole job.
	colCache, introspectErrs := introspectAllTables(ctx, h.DB.Pool, allNames)

	// 3. Open REPEATABLE READ read-only TX for a consistent data snapshot.
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 4. Read schema version from the TX snapshot so it matches the exported data.
	var schemaMigration int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schemaMigration); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	// Defense-in-depth: if a schema migration ran between step 2a and now,
	// the column cache built in step 2b may be stale — data queries inside the
	// TX could fail with "column X does not exist". Abort conservatively so the
	// caller gets a clean error rather than a silently-broken export.
	// TODO: executeExportJob's serialization-failure retry loop could be extended
	// to also retry on this migration-race error class; left out of scope for now.
	if schemaMigration != preIntrospectMigration {
		return fmt.Errorf("schema migration ran during introspection (pre=%d, in_tx=%d) — please retry", preIntrospectMigration, schemaMigration)
	}

	if h.shouldUseMultipartExport(ctx, userID) {
		return h.executeMultipartExportJobOnce(ctx, jobID, userID, tx, schemaMigration, colCache, specs, healthTables, introspectErrs)
	}

	// 5. Create artifact-backed ZIP writer.
	if h.ArtifactStore == nil {
		return fmt.Errorf("artifact_store_unconfigured")
	}
	expiresAt := time.Now().UTC().Add(dataExportDownloadTTL)
	artifactWriter, err := h.ArtifactStore.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "export",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        fmt.Sprintf("skadia-export-%s.zip", time.Now().Format("2006-01-02")),
		ContentType: "application/zip",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		return fmt.Errorf("create artifact writer: %w", err)
	}
	artifactCommitted := false
	defer func() {
		if !artifactCommitted {
			_ = artifactWriter.Abort(ctx)
		}
	}()

	zw := zip.NewWriter(artifactWriter)

	// 6. Export all tables via the TX.
	entityCounts, exportErrs := h.exportAllTables(ctx, zw, tx, userID, colCache, specs, healthTables)

	// 7. Commit TX (release snapshot). Data writes are done; remaining steps
	//    only touch the ZIP and the (immutable) ShadowFS file store.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// 8. ShadowFS files (outside TX — files are content-addressed and immutable).
	//    failedFiles captures zip-entry paths that could not be fully written
	//    (e.g. corrupt S3 blobs). The list is surfaced in the manifest so the
	//    user knows which files in the ZIP are empty/truncated.
	var failedFiles []string
	if h.ShadowFS != nil {
		fp, err := h.ShadowFS.StreamToZip(ctx, userID, zw)
		if err != nil {
			slog.Error("data export: file store stream failed", "error", err)
		}
		failedFiles = fp
	} else if basePath := h.Config.ShadowFSBasePath; basePath != "" {
		slog.Warn("data export: ShadowFS nil, falling back to legacy basePath",
			"user_id", userID, "base_path", basePath)
		streamShadowFSToZip(basePath, userID, zw)
	}

	// 9. columns.json — built from the pre-TX introspection cache, so it is
	//    consistent with the exported data even if a schema migration runs
	//    between TX commit and ZIP finalisation. No fresh DB call.
	writeZipJSON(zw, "columns.json", columnsMapToExported(colCache))

	// 10. manifest.json — MUST be the LAST file in the ZIP (so entity_counts,
	//     incomplete_tables, failed_files and export_errors reflect the final
	//     state). export_errors is included verbatim so a user inspecting the
	//     ZIP offline can see what failed without DB access.
	allErrs := append([]exportTableError{}, introspectErrs...)
	allErrs = append(allErrs, exportErrs...)
	manifest := map[string]any{
		"format_version":   1,
		"skadia_version":  skadiaExportVersion,
		"schema_migration": schemaMigration,
		"exported_at":      time.Now().Format(time.RFC3339),
		"user_id":          userID,
		"entity_counts":    entityCounts,
		"categories":       gdprCategoriesForManifest(),
	}
	if len(allErrs) > 0 {
		incompleteTables := make([]string, 0, len(allErrs))
		for _, e := range allErrs {
			incompleteTables = append(incompleteTables, e.Table)
		}
		manifest["incomplete_tables"] = incompleteTables
		manifest["export_errors"] = allErrs
	}
	if len(failedFiles) > 0 {
		manifest["failed_files"] = failedFiles
	}
	writeZipJSON(zw, "manifest.json", manifest)

	// 11. Close ZIP and commit artifact object.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	ref, err := artifactWriter.Commit(ctx)
	if err != nil {
		return fmt.Errorf("commit export artifact: %w", err)
	}
	artifactCommitted = true
	artifactPublished := false
	defer func() {
		if artifactCommitted && !artifactPublished && h.ArtifactStore != nil {
			h.cleanupUnpublishedExportArtifacts(context.Background(), userID, jobID)
		}
	}()

	// 13a. Partial-export path: errors present → mark failed, keep ZIP for debug.
	if len(allErrs) > 0 {
		errPayload, mErr := json.Marshal(allErrs)
		if mErr != nil {
			errPayload = []byte("[]")
		}
		incomplete := make([]string, 0, len(allErrs))
		for _, e := range allErrs {
			incomplete = append(incomplete, e.Table)
		}
		incompletePayload, mErr := json.Marshal(incomplete)
		if mErr != nil {
			incompletePayload = []byte("[]")
		}
		// topMsg omits the raw error string intentionally: SQL error messages can
		// leak schema internals (column names, constraint names, PostgreSQL hints).
		// Full error details are stored in metadata.export_errors[] which is only
		// visible in the admin UI, not returned to the end user.
		topMsg := fmt.Sprintf("export incomplete: %d table(s) errored, first failure on %q",
			len(allErrs), allErrs[0].Table)
		metadata, metaErr := buildSingleExportArtifactMetadata(ref, expiresAt, errPayload, incompletePayload)
		if metaErr != nil {
			return fmt.Errorf("build partial-fail metadata: %w", metaErr)
		}
		if _, err := h.DB.Pool.Exec(ctx,
			`UPDATE background_jobs SET
				status = 'failed',
				error = $3::text,
				metadata = $2::jsonb,
				updated_at = NOW()
			 WHERE id = $1::uuid`,
			jobID, string(metadata), topMsg); err != nil {
			return fmt.Errorf("store partial-fail metadata: %w", err)
		}
		artifactPublished = true
		if err := h.commitGDPRArtifactCapacity(ctx, jobID, userID, ref.SizeBytes); err != nil {
			return fmt.Errorf("commit partial artifact reservation: %w", err)
		}

		// Return an error so runExportJob's error wrapper logs the failure
		// (the metadata + status are already in place; the wrapper only
		// re-sets status='failed' which is idempotent).
		return fmt.Errorf("%s", topMsg)
	}

	// 13b. Success path: store metadata, mark completed.
	metadata, err := buildSingleExportArtifactMetadata(ref, expiresAt, nil, nil)
	if err != nil {
		return fmt.Errorf("build metadata: %w", err)
	}
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE background_jobs SET
			metadata = $2::jsonb,
			updated_at = NOW()
		 WHERE id = $1::uuid`,
		jobID, string(metadata)); err != nil {
		return fmt.Errorf("store metadata: %w", err)
	}
	artifactPublished = true
	if err := h.commitGDPRArtifactCapacity(ctx, jobID, userID, ref.SizeBytes); err != nil {
		return fmt.Errorf("commit artifact reservation: %w", err)
	}

	// Mark completed + create notification (single call, avoids double update).
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")

	return nil
}

func (h *Handler) finalizeExportReservationAfterError(ctx context.Context, jobID, userID string) error {
	var metadata []byte
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COALESCE(metadata, '{}'::jsonb) FROM background_jobs WHERE id = $1 AND user_id = $2`,
		jobID, userID).Scan(&metadata); err != nil {
		return err
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil || meta.Artifact == nil {
		return h.releaseGDPRArtifactCapacity(ctx, jobID, userID, 0)
	}
	return h.commitGDPRArtifactCapacity(ctx, jobID, userID, totalArtifactBytes(*meta.Artifact))
}

func buildSingleExportArtifactMetadata(ref gdprartifacts.ObjectRef, expiresAt time.Time, exportErrors, incompleteTables json.RawMessage) ([]byte, error) {
	artifact := gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: expiresAt,
		Objects:   []gdprartifacts.ObjectRef{ref},
	}
	raw, err := gdprartifacts.MergeArtifactJSON([]byte(`{}`), artifact)
	if err != nil {
		return nil, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	sha, _ := json.Marshal(ref.SHA256)
	size, _ := json.Marshal(ref.SizeBytes)
	obj["sha256"] = sha
	obj["size_bytes"] = size
	if len(exportErrors) > 0 {
		obj["export_errors"] = exportErrors
	}
	if len(incompleteTables) > 0 {
		obj["incomplete_tables"] = incompleteTables
	}
	return json.Marshal(obj)
}

// gdprExportTableSpec describes one table to export: ZIP path, source table
// name, and the static WHERE clause (which uses $1 for the user UUID).
type gdprExportTableSpec struct {
	zipPath string
	table   string
	where   string
}

// gdprExportTables returns the static list of non-health tables to export.
// Used both pre-TX (for column introspection) and inside the TX (for the
// actual data queries) to guarantee they operate on the same set.
func gdprExportTables() []gdprExportTableSpec {
	return []gdprExportTableSpec{
		{"data/profile.json", "users", "id = $1"},
		{"data/activities.json", "activities", "user_id = $1 ORDER BY started_at DESC"},
		{"data/gear.json", "gear", "user_id = $1"},
		{"data/tags.json", "tags", "user_id = $1"},
		{"data/segments.json", "segments", "created_by = $1"},
		{"data/segment_versions.json", "segment_versions", "segment_id IN (SELECT id FROM segments WHERE created_by = $1) ORDER BY segment_id, version_number"},
		{"data/planned_routes.json", "planned_routes", "user_id = $1"},
		{"data/activity_weather.json", "activity_weather", "activity_id IN (SELECT id FROM activities WHERE user_id = $1)"},
		{"data/workouts.json", "workouts", "user_id = $1"},
		{"data/workout_steps.json", "workout_steps", "workout_id IN (SELECT id FROM workouts WHERE user_id = $1) ORDER BY workout_id, message_index"},
		{"data/devices.json", "devices", "user_id = $1"},
		{"data/chat_sessions.json", "chat_sessions", "user_id = $1"},
		{"data/chat_messages.json", "chat_messages", "session_id IN (SELECT id FROM chat_sessions WHERE user_id = $1) ORDER BY created_at"},
		{"data/route_photos.json", "route_photos", "user_id = $1"},
		{"data/gear_documents.json", "gear_documents", "user_id = $1"},
		{"data/activity_tags.json", "activity_tags", "activity_id IN (SELECT id FROM activities WHERE user_id = $1)"},
		{"data/activity_photos.json", "activity_photos", "user_id = $1 ORDER BY activity_id, created_at"},
		{"data/user_privacy_zones.json", "user_privacy_zones", "user_id = $1"},
		{"data/custom_maps.json", "custom_maps", "user_id = $1"},
		{"data/custom_map_markers.json", "custom_map_markers", "user_id = $1"},
		{"data/automation_rules.json", "automation_rules", "user_id = $1"},
		{"data/import_log.json", "import_log", "user_id = $1 ORDER BY created_at DESC"},
		{"data/user_enrichment_settings.json", "user_enrichment_settings", "user_id = $1"},
		{"data/ml_user_settings.json", "ml_user_settings", "user_id = $1"},
		{"data/user_file_schemas.json", "user_file_schemas", "user_id = $1"},
		{"data/gear_components.json", "gear_components", "gear_id IN (SELECT id FROM gear WHERE user_id = $1)"},
		{"data/component_assignments.json", "component_assignments", "component_id IN (SELECT id FROM gear_components WHERE gear_id IN (SELECT id FROM gear WHERE user_id = $1))"},
		{"data/gear_service_events.json", "gear_service_events", "component_id IN (SELECT id FROM gear_components WHERE gear_id IN (SELECT id FROM gear WHERE user_id = $1)) ORDER BY performed_at DESC"},
		{"data/bike_setups.json", "bike_setups", "gear_id IN (SELECT id FROM gear WHERE user_id = $1)"},
		{"data/segment_favorites.json", "segment_favorites", "user_id = $1 ORDER BY created_at DESC"},
		{"data/oauth_connections.json", "oauth_connections", "user_id = $1"},
		{"data/user_files.json", "user_files", "user_id = $1 ORDER BY created_at DESC"},
		{"data/user_map_layers.json", "user_map_layers", "user_id = $1 ORDER BY sort_order, id"},
		{"data/user_readiness_preferences.json", "user_readiness_preferences", "user_id = $1"},
		{"data/file_attachments.json", "file_attachments", "user_id = $1 ORDER BY created_at DESC"},
		{"data/training_zones.json", "training_zones", "user_id = $1"},
		{"data/activity_edits.json", "activity_edits", "user_id = $1"},
		{"data/tag_zones.json", "tag_zones", "tag_id IN (SELECT id FROM tags WHERE user_id = $1)"},
		{"data/route_tags.json", "route_tags", "route_id IN (SELECT id FROM planned_routes WHERE user_id = $1)"},
		{"data/gear_service_intervals.json", "gear_service_intervals", "component_id IN (SELECT gc.id FROM gear_components gc JOIN gear g ON g.id = gc.gear_id WHERE g.user_id = $1)"},
		{"data/chain_rotation_groups.json", "chain_rotation_groups", "user_id = $1"},
		{"data/chain_rotation_members.json", "chain_rotation_members", "group_id IN (SELECT id FROM chain_rotation_groups WHERE user_id = $1)"},
		{"data/llm_user_config.json", "llm_user_config", "user_id = $1"},
		{"data/recording_groups.json", "recording_groups", "user_id = $1"},
		{"data/recording_group_members.json", "recording_group_members", "group_id IN (SELECT id FROM recording_groups WHERE user_id = $1)"},
		{"data/best_efforts.json", "best_efforts", "user_id = $1 ORDER BY achieved_at DESC"},
		{"data/best_effort_exclusions.json", "best_effort_exclusions", "user_id = $1"},
		{"data/activity_developer_apps.json", "activity_developer_apps", "activity_id IN (SELECT id FROM activities WHERE user_id = $1)"},
		{"data/routing_profiles.json", "routing_profiles", "user_id = $1"},
		{"data/activity_fusion_memberships.json", "activity_fusion_memberships", "merged_id IN (SELECT id FROM activities WHERE user_id = $1) AND source_id IN (SELECT id FROM activities WHERE user_id = $1)"},
		{"data/activity_gear_assignments.json", "activity_gear_assignments", "user_id = $1 ORDER BY created_at DESC"},
		{"data/gear_component_swap_events.json", "gear_component_swap_events", "user_id = $1 ORDER BY swap_date DESC"},
		{"data/gear_schema_definitions.json", "gear_schema_definitions", "user_id = $1"},
		// Phase 4d: rejected_by_user is user-authored; join through activities
		// to scope by user_id (activity_lift_candidates has no direct user_id column).
		{"data/activity_lift_candidates.json", "activity_lift_candidates", "activity_id IN (SELECT id FROM activities WHERE user_id = $1) ORDER BY activity_id, started_at"},
	}
}

// gdprAllTableNames returns every table that the export touches — the static
// list plus all health tables. Used to drive pre-TX column introspection.
func gdprAllTableNames(specs []gdprExportTableSpec, healthTables []string) []string {
	out := make([]string, 0, len(specs)+len(healthTables))
	for _, s := range specs {
		out = append(out, s.table)
	}
	out = append(out, healthTables...)
	return out
}

// gdprCategoriesForManifest derives the {"primary": [...], "external": [...]}
// structure from the GDPRRegistry. The lists are alphabetically sorted so the
// manifest output is stable across runs.
func gdprCategoriesForManifest() map[string][]string {
	var primary, external []string
	for table, cat := range GDPRRegistry {
		switch cat {
		case Export:
			primary = append(primary, table)
		case External:
			external = append(external, table)
		}
	}
	sort.Strings(primary)
	sort.Strings(external)
	return map[string][]string{
		"primary":  primary,
		"external": external,
	}
}

// exportAllTables runs all table exports within the given transaction, using
// pre-cached column metadata from `colCache`. Returns entity counts and a list
// of per-table export errors. A non-empty error slice means the export is
// incomplete and should be marked as failed.
func (h *Handler) exportAllTables(ctx context.Context, zw *zip.Writer, tx querier, userID string,
	colCache map[string][]columnInfo, specs []gdprExportTableSpec, healthTables []string,
) (map[string]int, []exportTableError) {
	counts := make(map[string]int)
	var errs []exportTableError

	// User data exported via profile.json (SELECT * minus deny-list).
	// account.json removed — was redundant subset.

	for _, t := range specs {
		n, err := exportTable(ctx, zw, tx, t.zipPath, t.table, t.where, colCache[t.table], userID)
		// Only record counts for fully-successful tables. A partial-row count
		// from an errored export is misleading: the importer should treat the
		// table as missing (incomplete_tables list) rather than as "X rows".
		if err != nil {
			errs = append(errs, exportTableError{Table: t.table, Error: err.Error()})
		} else if n > 0 {
			counts[t.table] = n
		}
	}

	// Health data (TimescaleDB hypertables).
	for _, table := range healthTables {
		n, err := exportTable(ctx, zw, tx, fmt.Sprintf("data/health/%s.json", table), table, "user_id = $1", colCache[table], userID)
		if err != nil {
			errs = append(errs, exportTableError{Table: table, Error: err.Error()})
		} else if n > 0 {
			counts[table] = n
		}
	}

	return counts, errs
}

// DataExportStatus handles GET /auth/data-export/{jobID} — returns export job status.
func (h *Handler) DataExportStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := chi.URLParam(r, "jobID")

	var status, startedAt string
	var updatedAt time.Time
	var errText *string
	var metadata []byte

	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT status, started_at::text, COALESCE(updated_at, started_at), error, COALESCE(metadata, '{}'::jsonb)
		 FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'`,
		jobID, userID).Scan(&status, &startedAt, &updatedAt, &errText, &metadata)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	metadata, unavailableReason := h.reconcileExportArtifactForStatus(r.Context(), jobID, userID, metadata)
	resp := buildDataExportStatusResponse(jobID, status, updatedAt, errText, metadata, time.Now())
	if unavailableReason == "artifact_backend_unavailable" {
		stripDataExportDownloadSessionFields(resp)
		resp["unavailable_reason"] = unavailableReason
	}
	resp["started_at"] = startedAt
	httputil.WriteJSONNoCache(w, http.StatusOK, resp)
}

func buildDataExportStatusResponse(
	jobID, status string,
	updatedAt time.Time,
	errText *string,
	metadata []byte,
	now time.Time,
) map[string]any {
	resp := map[string]any{
		"job_id":     jobID,
		"status":     status,
		"updated_at": updatedAt.Format(time.RFC3339),
	}

	// Surface metadata for both completed AND failed (partial-export) jobs.
	// A failed job may still have a downloadable ZIP for debugging — the
	// `partial: true` flag tells the frontend to warn the user.
	if (status == "completed" || status == "failed") && len(metadata) > 0 {
		if jobMeta, err := gdprartifacts.DecodeJobMetadata(metadata); err == nil && jobMeta.Artifact != nil {
			var rawMeta map[string]json.RawMessage
			_ = json.Unmarshal(metadata, &rawMeta)
			artifactMissing := len(rawMeta["artifact_missing_at"]) > 0

			artifact := jobMeta.Artifact
			resp["artifact_format"] = string(artifact.Format)
			resp["expires_at"] = artifact.ExpiresAt.Format(time.RFC3339)
			if len(artifact.Objects) > 0 {
				resp["sha256"] = artifact.Objects[0].SHA256
				resp["size_bytes"] = artifact.Objects[0].SizeBytes
			}
			if now.Before(artifact.ExpiresAt) && !artifactMissing {
				switch artifact.Format {
				case gdprartifacts.ArtifactFormatSingleZIP:
					resp["download_session_url"] = fmt.Sprintf("/api/v1/auth/data-export/%s/download-session", jobID)
				case gdprartifacts.ArtifactFormatMultipart:
					resp["manifest_download_session_url"] = fmt.Sprintf("/api/v1/auth/data-export/%s/download-session", jobID)
					parts := make([]map[string]any, 0, len(artifact.Objects))
					for _, ref := range artifact.Objects {
						parts = append(parts, map[string]any{
							"name":                 ref.Name,
							"role":                 ref.Role,
							"download_session_url": fmt.Sprintf("/api/v1/auth/data-export/%s/download-session", jobID),
							"sha256":               ref.SHA256,
							"size_bytes":           ref.SizeBytes,
							"web_import_supported": ref.Role != gdprartifacts.RoleFilesZIP || ref.SizeBytes <= 4<<30,
						})
					}
					resp["parts"] = parts
				}
				if status == "failed" {
					resp["partial"] = true
				}
			} else if artifactMissing {
				resp["unavailable_reason"] = "artifact_missing"
			}
			if raw := rawMeta["incomplete_tables"]; len(raw) > 0 {
				var incompleteTables []string
				if json.Unmarshal(raw, &incompleteTables) == nil {
					resp["incomplete_tables"] = incompleteTables
				}
			}
			return resp
		}

		var meta map[string]any
		if json.Unmarshal(metadata, &meta) == nil {
			if v, ok := meta["sha256"]; ok {
				resp["sha256"] = v
			}
			if v, ok := meta["size_bytes"]; ok {
				resp["size_bytes"] = v
			}
			if filePath, ok := meta["file_path"].(string); ok && filePath != "" {
				expiresAt := dataExportExpiresAt(updatedAt)
				resp["expires_at"] = expiresAt.Format(time.RFC3339)
				if !dataExportDownloadExpired(now, updatedAt) {
					resp["download_url"] = fmt.Sprintf("/api/v1/auth/data-export/%s/download", jobID)
					if status == "failed" {
						resp["partial"] = true
					}
				}
			}
			if v, ok := meta["incomplete_tables"]; ok {
				resp["incomplete_tables"] = v
			}
		}
	}

	if status == "failed" && errText != nil {
		// Don't leak internal SQL/error details, but distinguish between
		// stale-pod failures and real export failures so the frontend can
		// pick the right user-facing message.
		resp["error"] = "export_failed"
	}

	return resp
}

func stripDataExportDownloadSessionFields(resp map[string]any) {
	delete(resp, "download_session_url")
	delete(resp, "manifest_download_session_url")
	if parts, ok := resp["parts"].([]map[string]any); ok {
		for _, part := range parts {
			delete(part, "download_session_url")
		}
	}
}

// DataExportDownloadSession handles POST /auth/data-export/{jobID}/download-session.
func (h *Handler) DataExportDownloadSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := chi.URLParam(r, "jobID")
	ctx := r.Context()
	if h.ArtifactStore == nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "artifact_store_unconfigured"})
		return
	}

	var req struct {
		Single bool   `json:"single"`
		Part   string `json:"part"`
		All    bool   `json:"all"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	var status string
	var metadata []byte
	err := h.DB.Pool.QueryRow(ctx,
		`SELECT status, COALESCE(metadata, '{}'::jsonb)
		 FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'`,
		jobID, userID).Scan(&status, &metadata)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if status != "completed" && status != "failed" {
		httputil.WriteJSONNoCache(w, http.StatusConflict, map[string]string{"error": "not_completed"})
		return
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil || meta.Artifact == nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if !time.Now().Before(meta.Artifact.ExpiresAt) {
		httputil.WriteJSONNoCache(w, http.StatusGone, map[string]string{"error": "file_expired"})
		return
	}

	allowedRefs, err := selectDownloadSessionObjects(*meta.Artifact, req.Single, req.Part, req.All)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_part"})
		return
	}
	for _, ref := range allowedRefs {
		stat, err := h.ArtifactStore.Stat(ctx, ref)
		if err != nil || stat.State != gdprartifacts.ObjectPresent {
			httputil.WriteJSONNoCache(w, http.StatusGone, map[string]string{"error": "artifact_unavailable"})
			return
		}
	}

	sessionID := uuid.NewString()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "session_failed"})
		return
	}
	rawSecret := base64.RawURLEncoding.EncodeToString(secret)
	expiresAt := time.Now().UTC().Add(dataExportDownloadSessionTTL)
	session := gdprartifacts.DownloadSessionRecord{
		SessionID:  sessionID,
		SecretHash: gdprartifacts.HashDownloadSessionSecret([]byte(rawSecret)),
		UserID:     userID,
		JobID:      jobID,
		ExpiresAt:  expiresAt,
		CreatedAt:  time.Now().UTC(),
	}
	for _, ref := range allowedRefs {
		session.AllowedObjects = append(session.AllowedObjects, gdprartifacts.DownloadSessionObject{
			Name: ref.Name, Role: ref.Role, SHA256: ref.SHA256, SizeBytes: ref.SizeBytes,
		})
	}
	updated, err := gdprartifacts.AddDownloadSessionJSON(metadata, session)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "session_failed"})
		return
	}
	if _, err := h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET metadata=$2::jsonb, updated_at=updated_at WHERE id=$1`, jobID, string(updated)); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "session_failed"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     dataExportDownloadSessionCookie,
		Value:    rawSecret,
		Path:     "/api/v1/downloads/gdpr/data-export/",
		Expires:  expiresAt,
		MaxAge:   int(dataExportDownloadSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isRequestHTTPS(r),
	})
	first := allowedRefs[0]
	downloadURL := dataExportDownloadURL(jobID, first.Name, sessionID)
	type downloadSessionObjectResponse struct {
		Name      string                   `json:"name"`
		Role      gdprartifacts.ObjectRole `json:"role"`
		URL       string                   `json:"url"`
		SHA256    string                   `json:"sha256"`
		SizeBytes int64                    `json:"size_bytes"`
	}
	objects := make([]downloadSessionObjectResponse, 0, len(allowedRefs))
	for _, ref := range allowedRefs {
		objects = append(objects, downloadSessionObjectResponse{
			Name: ref.Name, Role: ref.Role, URL: dataExportDownloadURL(jobID, ref.Name, sessionID), SHA256: ref.SHA256, SizeBytes: ref.SizeBytes,
		})
	}
	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "data_export_download_session_issued",
		middleware.ClientIP(r), r.UserAgent(), map[string]string{"job_id": jobID})
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"url":        downloadURL,
		"expires_at": expiresAt.Format(time.RFC3339),
		"objects":    objects,
	})
}

func (h *Handler) DataExportDownloadManifest(w http.ResponseWriter, r *http.Request) {
	h.streamDataExportArtifact(w, r, "manifest.json")
}

func (h *Handler) DataExportDownloadPart(w http.ResponseWriter, r *http.Request) {
	h.streamDataExportArtifact(w, r, chi.URLParam(r, "partName"))
}

func (h *Handler) streamDataExportArtifact(w http.ResponseWriter, r *http.Request, objectName string) {
	jobID := chi.URLParam(r, "jobID")
	sessionID := r.URL.Query().Get("session_id")
	cookie, err := r.Cookie(dataExportDownloadSessionCookie)
	if err != nil || sessionID == "" || objectName == "" {
		httputil.WriteJSONNoCache(w, http.StatusUnauthorized, map[string]string{"error": "download_session_invalid"})
		return
	}
	ref, clearCookie, status, errorCode := h.consumeDataExportDownloadSession(r.Context(), jobID, sessionID, cookie.Value, objectName)
	if status != http.StatusOK {
		httputil.WriteJSONNoCache(w, status, map[string]string{"error": errorCode})
		return
	}
	rangeSpec, statusCode, err := parseRangeHeader(r.Header.Get("Range"))
	if err != nil {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", ref.SizeBytes))
		httputil.WriteJSONNoCache(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid_range"})
		return
	}
	contentRange := ""
	if rangeSpec != nil {
		start := int64(0)
		if rangeSpec.Start != nil {
			start = *rangeSpec.Start
		}
		if start >= ref.SizeBytes {
			w.Header().Set("Accept-Ranges", "bytes")
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", ref.SizeBytes))
			httputil.WriteJSONNoCache(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"error": "invalid_range"})
			return
		}
		end := ref.SizeBytes - 1
		if rangeSpec.End != nil && *rangeSpec.End < end {
			end = *rangeSpec.End
		}
		rangeSpec.End = &end
		contentRange = fmt.Sprintf("bytes %d-%d/%d", start, end, ref.SizeBytes)
	}
	reader, err := h.ArtifactStore.Open(r.Context(), ref, rangeSpec)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusGone, map[string]string{"error": "artifact_unavailable"})
		return
	}
	defer reader.Close()

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", defaultDownloadContentType(ref.ContentType))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeDownloadFilename(ref.Name)))
	w.Header().Set("X-Checksum-SHA256", ref.SHA256)
	if contentRange != "" {
		w.Header().Set("Content-Range", contentRange)
	}
	if clearCookie {
		http.SetCookie(w, &http.Cookie{
			Name:     dataExportDownloadSessionCookie,
			Value:    "",
			Path:     "/api/v1/downloads/gdpr/data-export/",
			Expires:  time.Unix(0, 0),
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   isRequestHTTPS(r),
		})
	}
	if reader.Size() >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(reader.Size(), 10))
	}
	if statusCode != 0 {
		w.WriteHeader(statusCode)
	}
	if _, err := io.Copy(w, reader); err != nil {
		slog.Warn("gdpr export download stream failed", "job_id", jobID, "object", objectName, "error", err)
		return
	}
	go h.DB.LogAuthEvent(h.shutdownCtx, ref.UserID, "data_export_download_started",
		middleware.ClientIP(r), r.UserAgent(), map[string]string{"job_id": jobID, "part_name": objectName})
}

func (h *Handler) consumeDataExportDownloadSession(ctx context.Context, jobID, sessionID, rawSecret, objectName string) (gdprartifacts.ObjectRef, bool, int, string) {
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusInternalServerError, "download_session_invalid"
	}
	defer tx.Rollback(ctx)

	var metadata []byte
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(metadata, '{}'::jsonb)
		 FROM background_jobs
		 WHERE id = $1 AND job_type = 'gdpr_export'
		 FOR UPDATE`,
		jobID).Scan(&metadata); err != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusNotFound, "not_found"
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil || meta.Artifact == nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusUnauthorized, "download_session_invalid"
	}
	now := time.Now().UTC()
	if !now.Before(meta.Artifact.ExpiresAt) {
		return gdprartifacts.ObjectRef{}, false, http.StatusGone, "file_expired"
	}
	session, ok := meta.DownloadSessions[sessionID]
	if !ok || session.JobID != jobID || now.After(session.ExpiresAt) {
		return gdprartifacts.ObjectRef{}, false, http.StatusUnauthorized, "download_session_invalid"
	}
	if subtle.ConstantTimeCompare([]byte(session.SecretHash), []byte(gdprartifacts.HashDownloadSessionSecret([]byte(rawSecret)))) != 1 {
		return gdprartifacts.ObjectRef{}, false, http.StatusUnauthorized, "download_session_invalid"
	}
	var allowed *gdprartifacts.DownloadSessionObject
	for i := range session.AllowedObjects {
		if session.AllowedObjects[i].Name == objectName {
			allowed = &session.AllowedObjects[i]
			break
		}
	}
	if allowed == nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusForbidden, "download_session_invalid"
	}
	if allowed.UsedAt != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusUnauthorized, "download_session_invalid"
	}
	ref, ok := findArtifactObject(*meta.Artifact, objectName)
	if !ok {
		return gdprartifacts.ObjectRef{}, false, http.StatusGone, "artifact_unavailable"
	}
	if ref.Role != allowed.Role || ref.SHA256 != allowed.SHA256 || ref.SizeBytes != allowed.SizeBytes {
		return gdprartifacts.ObjectRef{}, false, http.StatusUnauthorized, "download_session_invalid"
	}
	updated, err := gdprartifacts.MarkDownloadSessionObjectUsedJSON(metadata, sessionID, objectName, now)
	if err != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusInternalServerError, "download_session_invalid"
	}
	if _, err := tx.Exec(ctx, `UPDATE background_jobs SET metadata=$2::jsonb, updated_at=updated_at WHERE id=$1`, jobID, string(updated)); err != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusInternalServerError, "download_session_invalid"
	}
	if err := tx.Commit(ctx); err != nil {
		return gdprartifacts.ObjectRef{}, false, http.StatusInternalServerError, "download_session_invalid"
	}
	clearCookie := true
	for _, object := range session.AllowedObjects {
		if object.Name != objectName && object.UsedAt == nil {
			clearCookie = false
			break
		}
	}
	return ref, clearCookie, http.StatusOK, ""
}

func selectDownloadSessionObjects(artifact gdprartifacts.Artifact, single bool, part string, all bool) ([]gdprartifacts.ObjectRef, error) {
	if all {
		return artifact.Objects, nil
	}
	if single {
		for _, ref := range artifact.Objects {
			if ref.Role == gdprartifacts.RoleSingleZIP {
				return []gdprartifacts.ObjectRef{ref}, nil
			}
		}
		return nil, fmt.Errorf("single artifact missing")
	}
	if part != "" {
		ref, ok := findArtifactObject(artifact, part)
		if !ok {
			return nil, fmt.Errorf("part not found")
		}
		return []gdprartifacts.ObjectRef{ref}, nil
	}
	return nil, fmt.Errorf("object selector required")
}

func findArtifactObject(artifact gdprartifacts.Artifact, name string) (gdprartifacts.ObjectRef, bool) {
	for _, ref := range artifact.Objects {
		if ref.Name == name {
			return ref, true
		}
	}
	return gdprartifacts.ObjectRef{}, false
}

func dataExportDownloadURL(jobID, objectName, sessionID string) string {
	return fmt.Sprintf("/api/v1/downloads/gdpr/data-export/%s/parts/%s?session_id=%s",
		url.PathEscape(jobID), url.PathEscape(objectName), url.QueryEscape(sessionID))
}

func parseRangeHeader(header string) (*gdprartifacts.RangeSpec, int, error) {
	if header == "" {
		return nil, 0, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return nil, 0, fmt.Errorf("unsupported range")
	}
	bounds := strings.TrimPrefix(header, "bytes=")
	startText, endText, ok := strings.Cut(bounds, "-")
	if !ok || startText == "" {
		return nil, 0, fmt.Errorf("invalid range")
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil || start < 0 {
		return nil, 0, fmt.Errorf("invalid range start")
	}
	spec := &gdprartifacts.RangeSpec{Start: &start}
	if endText != "" {
		end, err := strconv.ParseInt(endText, 10, 64)
		if err != nil || end < start {
			return nil, 0, fmt.Errorf("invalid range end")
		}
		spec.End = &end
	}
	return spec, http.StatusPartialContent, nil
}

func defaultDownloadContentType(contentType string) string {
	if contentType == "" {
		return "application/octet-stream"
	}
	return contentType
}

func sanitizeDownloadFilename(name string) string {
	name = strings.ReplaceAll(name, `"`, "")
	name = filepath.Base(name)
	if name == "." || name == "/" || name == "" {
		return "download.bin"
	}
	return name
}

func isRequestHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// DataExportDownload handles GET /auth/data-export/{jobID}/download — streams the export ZIP.
func (h *Handler) DataExportDownload(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := chi.URLParam(r, "jobID")
	ctx := r.Context()

	// Rate-limit: max 5 downloads per hour.
	var dlCount int
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM auth_audit_log
		 WHERE user_id = $1::uuid AND event_type = 'data_export_download' AND created_at > NOW() - INTERVAL '1 hour'`,
		userID).Scan(&dlCount); err != nil {
		slog.Error("gdpr export download: rate limit query failed", "error", err)
	}
	if dlCount >= 5 {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "download_rate_limit"})
		return
	}

	var status string
	var updatedAt time.Time
	var metadata []byte
	err := h.DB.Pool.QueryRow(ctx,
		`SELECT status, COALESCE(updated_at, started_at), COALESCE(metadata, '{}'::jsonb)
		 FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'`,
		jobID, userID).Scan(&status, &updatedAt, &metadata)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	// Allow downloading partial-export ZIPs from failed jobs, too — they are
	// useful for debugging "why is my export incomplete?". The DataExportStatus
	// response sets `partial: true` for these so the UI can warn the user.
	if status != "completed" && status != "failed" {
		httputil.WriteJSONNoCache(w, http.StatusConflict, map[string]string{"error": "not_completed"})
		return
	}

	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "invalid_metadata"})
		return
	}

	filePath, _ := meta["file_path"].(string)
	if filePath == "" || !isValidExportPath(filePath) {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	if dataExportDownloadExpired(time.Now(), updatedAt) {
		os.Remove(filePath)
		clearDataExportFilePath(ctx, h.DB.Pool, jobID)
		httputil.WriteJSONNoCache(w, http.StatusGone, map[string]string{"error": "file_expired"})
		return
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		httputil.WriteJSONNoCache(w, http.StatusGone, map[string]string{"error": "file_expired"})
		return
	}

	checksum, _ := meta["sha256"].(string)

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="skadia-export-%s.zip"`, time.Now().Format("2006-01-02")))
	if checksum != "" {
		w.Header().Set("X-Checksum-SHA256", checksum)
	}

	http.ServeFile(w, r, filePath)

	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "data_export_download",
		middleware.ClientIP(r), r.UserAgent(), map[string]string{"job_id": jobID})
}

// createZipEntry creates a zip entry with the current timestamp (avoids 1979 default).
func createZipEntry(zw *zip.Writer, name string) (io.Writer, error) {
	return zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
}

// writeZipJSON writes a single JSON value to the zip archive under the given name.
func writeZipJSON(zw *zip.Writer, name string, data any) {
	w, err := createZipEntry(zw, name)
	if err != nil {
		slog.Error("data export: create zip entry failed", "name", name, "error", err)
		return
	}
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("data export: encode json failed", "name", name, "error", err)
	}
}

// streamShadowFSToZip walks the user's ShadowFS directory and copies each file into the zip.
func streamShadowFSToZip(basePath, userID string, zw *zip.Writer) {
	const maxLegacyZipBytes int64 = 4 << 30 // 4 GiB — matches the web GDPR import limit

	userDir := filepath.Join(basePath, userID)
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return
	}

	var totalBytes int64
	err := filepath.Walk(userDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Enforce the same 4 GiB payload limit as the ShadowFS/S3 path above.
		totalBytes += info.Size()
		if totalBytes > maxLegacyZipBytes {
			slog.Warn("data export: legacy ShadowFS zip exceeds 4 GiB limit",
				"user_id", userID, "total_bytes", totalBytes)
			return fmt.Errorf("zip export exceeds 4 GiB limit")
		}

		rel, relErr := filepath.Rel(userDir, path)
		if relErr != nil {
			return nil
		}
		w, createErr := createZipEntry(zw, "files/"+rel)
		if createErr != nil {
			slog.Error("data export: create zip entry for shadowfs failed", "path", rel, "error", createErr)
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			slog.Error("data export: open shadowfs file failed", "path", path, "error", openErr)
			return nil
		}
		defer f.Close()
		if _, cpErr := io.Copy(w, f); cpErr != nil {
			slog.Error("data export: copy shadowfs file failed", "path", rel, "error", cpErr)
		}
		return nil
	})
	if err != nil {
		slog.Error("data export: walk shadowfs failed", "user_id", userID, "error", err)
	}
}
