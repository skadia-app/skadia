package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/dataimport"
	"github.com/skadia-app/skadia/services/api/internal/gdprartifacts"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// isValidImportPath validates that a file path from DB metadata points to
// a legitimate import temp file. Defense-in-depth against DB manipulation.
func isValidImportPath(path string) bool {
	cleaned := filepath.Clean(path)
	return strings.HasPrefix(cleaned, os.TempDir()) && strings.Contains(filepath.Base(cleaned), "skadia-import-")
}

var staleImportCleanupOnce sync.Once

// startStaleImportCleanup runs the stale import job cleanup every 5 minutes.
// Safe to call multiple times — only starts once.
func (h *Handler) startStaleImportCleanup() {
	staleImportCleanupOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			// Run once immediately at startup.
			h.cleanupStaleImportJobs(h.shutdownCtx)
			for {
				select {
				case <-h.shutdownCtx.Done():
					return
				case <-ticker.C:
					h.cleanupStaleImportJobs(h.shutdownCtx)
				}
			}
		}()
	})
}

// cleanupStaleImportJobs removes files and marks stale import jobs as failed.
// Called periodically by the background timer started via startStaleImportCleanup.
//
// Two cleanup classes:
//
//  1. STALE: queued/running jobs that have not made progress in >30 min are
//     marked as 'failed' (their pod must have crashed). Their tmp file (if
//     any) is deleted.
//
//  2. EXPIRED: completed/failed jobs whose tmp file is older than 25 hours
//     are deleted. The in-process cleanup goroutine started by runImportJob
//     would normally do this, but is lost on pod-restart — without this
//     fallback the file leaks indefinitely.
func (h *Handler) cleanupStaleImportJobs(ctx context.Context) {
	pool := h.DB.Pool

	// Class 1: stale running/queued
	staleRows, err := pool.Query(ctx,
		`SELECT metadata->>'file_path' FROM background_jobs
		 WHERE job_type = 'data_import' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'
		 AND metadata->>'file_path' IS NOT NULL`)
	if err != nil {
		slog.Error("data import cleanup: query stale jobs failed", "error", err)
	} else {
		for staleRows.Next() {
			var path string
			if staleRows.Scan(&path) == nil && path != "" && isValidImportPath(path) {
				os.Remove(path)
			}
		}
		staleRows.Close()
	}

	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'failed', error = 'stale: pod crash', updated_at = NOW()
		 WHERE job_type = 'data_import' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'`); err != nil {
		slog.Error("data import cleanup: update stale jobs failed", "error", err)
	}

	// Class 2: expired completed/failed jobs (file leaked because the
	// in-process cleanup goroutine was lost on pod-restart).
	expiredRows, err := pool.Query(ctx,
		`SELECT id, metadata->>'file_path' FROM background_jobs
		 WHERE job_type = 'data_import' AND status IN ('completed','failed')
		 AND updated_at < NOW() - INTERVAL '25 hours'
		 AND metadata->>'file_path' IS NOT NULL`)
	if err != nil {
		slog.Error("data import cleanup: query expired jobs failed", "error", err)
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
		if isValidImportPath(e.path) {
			os.Remove(e.path)
		}
		// Strip file_path from metadata so we don't keep retrying.
		if _, err := pool.Exec(ctx,
			`UPDATE background_jobs SET metadata = metadata - 'file_path' WHERE id = $1`, e.id); err != nil {
			slog.Warn("data import cleanup: clear file_path failed", "job_id", e.id, "error", err)
		}
	}
}

// DataImport handles POST /auth/data-import — uploads a GDPR export ZIP and starts an import job.
func (h *Handler) DataImport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	ctx := r.Context()

	// Ensure the background stale-job cleanup timer is running (starts once per process).
	h.startStaleImportCleanup()

	// Limit request body to 4 GB.
	r.Body = http.MaxBytesReader(w, r.Body, 4<<30)

	file, _, err := r.FormFile("file")
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "file_required"})
		return
	}
	defer file.Close()

	if h.ArtifactStore == nil {
		slog.Error("data import: artifact store not configured")
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	jobID := uuid.New().String()
	expiresAt := time.Now().UTC().Add(24 * time.Hour)
	writer, err := h.ArtifactStore.CreateWriter(ctx, gdprartifacts.ObjectSpec{
		Kind:        "import",
		UserID:      userID,
		JobID:       jobID,
		Role:        gdprartifacts.RoleSingleZIP,
		Name:        "import.zip",
		ContentType: "application/zip",
		ExpiresAt:   expiresAt,
	})
	if err != nil {
		slog.Error("data import: create artifact writer failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			if err := writer.Abort(context.Background()); err != nil {
				slog.Warn("data import: abort artifact writer failed", "job_id", jobID, "error", err)
			}
		}
	}()
	if _, err := io.Copy(writer, file); err != nil {
		slog.Error("data import: write artifact failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	ref, err := writer.Commit(ctx)
	if err != nil {
		slog.Error("data import: commit artifact failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	committed = true

	artifact := gdprartifacts.Artifact{
		Version:   1,
		Format:    gdprartifacts.ArtifactFormatSingleZIP,
		ExpiresAt: expiresAt,
		Objects:   []gdprartifacts.ObjectRef{ref},
	}
	metadataJSON, err := buildImportUploadMetadata(artifact, ref)
	if err != nil {
		slog.Error("data import: encode artifact metadata failed", "error", err)
		_ = h.ArtifactStore.DeletePrefix(context.Background(), gdprartifacts.ArtifactPrefix{Kind: "import", UserID: userID, JobID: jobID})
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	// Atomic INSERT — prevents race condition between conflict-check and INSERT.
	// Also enforces per-user mutual exclusion (no concurrent data_export or data_import)
	// and server-wide concurrency limit of 2 simultaneous imports.
	err = h.DB.Pool.QueryRow(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at, metadata)
		SELECT $1, 'data_import', $2, 'queued', NOW(), NOW(), $3::jsonb
		WHERE NOT EXISTS (
			SELECT 1 FROM background_jobs
			WHERE user_id = $2
			  AND job_type IN ('data_export', 'data_import')
			  AND status IN ('queued', 'running')
		)
		AND (SELECT COUNT(*) FROM background_jobs
		     WHERE job_type = 'data_import'
		     AND status IN ('queued', 'running')) < 2
		RETURNING true`, jobID, userID, metadataJSON).Scan(new(bool))
	if errors.Is(err, pgx.ErrNoRows) {
		// Either a concurrent export/import for this user, or server-wide limit hit.
		_ = h.ArtifactStore.DeletePrefix(context.Background(), gdprartifacts.ArtifactPrefix{Kind: "import", UserID: userID, JobID: jobID})
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "import_already_running"})
		return
	}
	if err != nil {
		slog.Error("data import: insert job failed", "error", err)
		_ = h.ArtifactStore.DeletePrefix(context.Background(), gdprartifacts.ArtifactPrefix{Kind: "import", UserID: userID, JobID: jobID})
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}

	go h.runImportJob(jobID, userID, "")

	httputil.WriteJSONNoCache(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"status": "queued",
	})
}

func buildImportUploadMetadata(artifact gdprartifacts.Artifact, ref gdprartifacts.ObjectRef) ([]byte, error) {
	raw, err := gdprartifacts.MergeArtifactJSON(nil, artifact)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	obj["import_upload"] = map[string]any{
		"state":       "committed",
		"size_bytes":  ref.SizeBytes,
		"sha256":      ref.SHA256,
		"received_at": time.Now().UTC(),
	}
	return json.Marshal(obj)
}

// runImportJob transitions the job to running, executes the import, and updates status on completion.
func (h *Handler) runImportJob(jobID, userID, zipPath string) {
	ctx := h.shutdownCtx

	// Transition to running.
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'running', updated_at = NOW() WHERE id = $1`, jobID); err != nil {
		// Log but don't abort — import is more important than status tracking.
		slog.Warn("data import job: failed to set status=running", "job_id", jobID, "error", err)
	}

	// Build engine config from handler config.
	engineCfg := dataimport.EngineConfig{
		StorageBackend:                 h.Config.StorageBackend,
		LibraryBasePath:                h.Config.LibraryBasePath,
		MaxStorageBytes:                h.Config.MaxUserStorageBytes,
		SkipBackgroundJobConflictCheck: true,
	}
	engine := &dataimport.Engine{
		DB:            h.DB.Pool,
		SettingsStore: h.Settings,
		Config:        engineCfg,
	}
	if h.ShadowFS != nil {
		engine.ShadowFS = h.ShadowFS
	}

	src, artifact, legacyPath, err := h.openImportJobSource(ctx, jobID, zipPath)
	if err != nil {
		slog.Error("data import job: open source failed", "job_id", jobID, "error", err)
		if _, dbErr := h.DB.Pool.Exec(ctx,
			`UPDATE background_jobs SET status = 'failed', error = $2, updated_at = NOW(), finished_at = NOW() WHERE id = $1`,
			jobID, fmt.Sprintf("open source: %s", err.Error())); dbErr != nil {
			// Log but don't abort — import is more important than status tracking.
			slog.Warn("data import job: failed to set status=failed", "job_id", jobID, "error", dbErr)
		}
		return
	}
	defer src.Close()
	if artifact != nil && h.ArtifactStore != nil {
		defer func() {
			if err := h.ArtifactStore.DeletePrefix(context.Background(), gdprartifacts.ArtifactPrefix{Kind: "import", UserID: userID, JobID: jobID}); err != nil {
				slog.Warn("data import job: delete staged artifact failed", "job_id", jobID, "error", err)
			}
		}()
	} else if legacyPath != "" {
		defer os.Remove(legacyPath)
	}

	// Throttle for progress updates: max 1 DB write per second.
	var lastProgress time.Time
	var phasesTotal int
	onProgress := func(ev dataimport.ProgressEvent) {
		now := time.Now()
		if now.Sub(lastProgress) < time.Second {
			return
		}
		lastProgress = now
		if ev.PhasesTotal > 0 {
			phasesTotal = ev.PhasesTotal
		}
		metaJSON, _ := json.Marshal(map[string]any{
			"phase":             ev.Phase,
			"phase_name":        ev.PhaseName,
			"phases_total":      phasesTotal,
			"table":             ev.Table,
			"entities_imported": ev.EntitiesImported,
			"entities_skipped":  ev.EntitiesSkipped,
			"entities_failed":   ev.EntitiesFailed,
			"files_total":       ev.FilesTotal,
			"files_imported":    ev.FilesImported,
			"bytes_imported":    ev.BytesImported,
			"message":           ev.Message,
		})
		// Phase-based progress: Phase 0–10, PhasesTotal = 11.
		var phaseName *string
		if ev.PhaseName != "" {
			phaseName = &ev.PhaseName
		} else if ev.Table != "" {
			phaseName = &ev.Table
		}
		h.DB.Pool.Exec(ctx,
			`UPDATE background_jobs
			 SET progress_current = $2, progress_total = $3,
			     phase = $4,
			     metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{import_progress}', $5::jsonb, true),
			     updated_at = NOW()
			 WHERE id = $1`,
			jobID, ev.Phase, phasesTotal, phaseName, metaJSON)
	}

	result, err := engine.RunImport(ctx, dataimport.ImportInput{
		Source:       src,
		TargetUserID: userID,
		OnProgress:   onProgress,
	})
	if err != nil {
		slog.Error("data import job failed", "job_id", jobID, "error", err)
		if _, dbErr := h.DB.Pool.Exec(ctx,
			`UPDATE background_jobs SET status = 'failed', error = $2, updated_at = NOW(), finished_at = NOW() WHERE id = $1`,
			jobID, err.Error()); dbErr != nil {
			// Log but don't abort — import is more important than status tracking.
			slog.Warn("data import job: failed to set status=failed", "job_id", jobID, "error", dbErr)
		}
		return
	}

	// Store result summary in metadata.
	resultMeta, _ := json.Marshal(map[string]any{
		"phases_completed":  result.PhasesCompleted,
		"entities_imported": result.EntitiesImported,
		"entities_skipped":  result.EntitiesSkipped,
		"entities_failed":   result.EntitiesFailed,
		"warnings":          result.Warnings,
	})
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE background_jobs
		 SET status = 'completed', finished_at = NOW(), updated_at = NOW(),
		     metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{import_result}', $2::jsonb, true)
		 WHERE id = $1`,
		jobID, resultMeta); err != nil {
		// Log but don't abort — import is more important than status tracking.
		slog.Warn("data import job: failed to set status=completed", "job_id", jobID, "error", err)
	}

	slog.Info("data import job completed",
		"job_id", jobID,
		"user_id", userID,
		"entities_imported", result.EntitiesImported,
		"entities_skipped", result.EntitiesSkipped,
		"entities_failed", result.EntitiesFailed,
	)
}

func (h *Handler) openImportJobSource(ctx context.Context, jobID, fallbackPath string) (dataimport.Source, *gdprartifacts.Artifact, string, error) {
	var metadata []byte
	if err := h.DB.Pool.QueryRow(ctx, `SELECT metadata FROM background_jobs WHERE id = $1`, jobID).Scan(&metadata); err != nil {
		return nil, nil, "", fmt.Errorf("load job metadata: %w", err)
	}
	meta, err := gdprartifacts.DecodeJobMetadata(metadata)
	if err != nil {
		return nil, nil, "", fmt.Errorf("decode job metadata: %w", err)
	}
	if meta.Artifact != nil {
		if h.ArtifactStore == nil {
			return nil, nil, "", fmt.Errorf("artifact store not configured")
		}
		if meta.Artifact.Format != gdprartifacts.ArtifactFormatSingleZIP {
			return nil, nil, "", fmt.Errorf("unsupported import artifact format %q", meta.Artifact.Format)
		}
		if len(meta.Artifact.Objects) != 1 {
			return nil, nil, "", fmt.Errorf("single import artifact has %d objects", len(meta.Artifact.Objects))
		}
		src, err := dataimport.NewArtifactZipSource(ctx, h.ArtifactStore, meta.Artifact.Objects[0])
		if err != nil {
			return nil, nil, "", err
		}
		return src, meta.Artifact, "", nil
	}
	path := meta.LegacyFilePath
	if path == "" {
		path = fallbackPath
	}
	if path == "" {
		return nil, nil, "", fmt.Errorf("import source missing")
	}
	src, err := dataimport.OpenSource(path)
	if err != nil {
		return nil, nil, "", err
	}
	return src, nil, path, nil
}

// DataImportStatus handles GET /auth/data-import/{jobID} — returns import job status.
func (h *Handler) DataImportStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := chi.URLParam(r, "jobID")

	var status string
	var errText *string
	var metadata []byte
	var progressCurrent, progressTotal int

	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT status, error, metadata, progress_current, progress_total
		 FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'data_import'`,
		jobID, userID).Scan(&status, &errText, &metadata, &progressCurrent, &progressTotal)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	resp := map[string]any{
		"job_id":           jobID,
		"status":           status,
		"progress_current": progressCurrent,
		"progress_total":   progressTotal,
	}

	if len(metadata) > 0 {
		var meta map[string]any
		if json.Unmarshal(metadata, &meta) == nil {
			if progress, ok := meta["import_progress"].(map[string]any); ok {
				for k, v := range progress {
					resp[k] = v
				}
			}
			if result, ok := meta["import_result"].(map[string]any); ok {
				for k, v := range result {
					resp[k] = v
				}
			}
			for _, key := range []string{
				"phase", "phase_name", "phases_total",
				"table", "entities_imported", "entities_skipped", "entities_failed",
				"files_total", "files_imported", "bytes_imported",
				"phases_completed", "warnings", "message",
			} {
				if v, ok := meta[key]; ok {
					resp[key] = v
				}
			}
		}
	}

	if status == "failed" && errText != nil {
		resp["error"] = "import_failed"
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, resp)
}
