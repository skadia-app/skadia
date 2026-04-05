package auth

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/skadia-app/skadia/services/api/internal/database"
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
func cleanupStaleExportJobs(ctx context.Context, pool *pgxpool.Pool) {
	staleRows, err := pool.Query(ctx,
		`SELECT metadata->>'file_path' FROM background_jobs
		 WHERE job_type = 'gdpr_export' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'
		 AND metadata->>'file_path' IS NOT NULL`)
	if err != nil {
		slog.Error("gdpr export cleanup: query stale jobs failed", "error", err)
	} else {
		defer staleRows.Close()
		for staleRows.Next() {
			var path string
			if staleRows.Scan(&path) == nil && path != "" && isValidExportPath(path) {
				os.Remove(path)
			}
		}
	}

	if _, err := pool.Exec(ctx,
		`UPDATE background_jobs SET status = 'failed', error = 'stale: pod crash', updated_at = NOW()
		 WHERE job_type = 'gdpr_export' AND status IN ('queued','running')
		 AND updated_at < NOW() - INTERVAL '30 minutes'`); err != nil {
		slog.Error("gdpr export cleanup: update stale jobs failed", "error", err)
	}
}

// DataExport handles POST /auth/data-export — starts a background GDPR export job.
func (h *Handler) DataExport(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	ctx := r.Context()

	user, err := h.DB.GetUserByID(ctx, userID)
	if err != nil || !user.IsActive || user.DeletedAt != nil {
		httputil.WriteJSONNoCache(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}

	// Ensure the background stale-job cleanup timer is running (starts once per process).
	startStaleExportCleanup(h.shutdownCtx, h.DB.Pool)

	// Per-user: no concurrent export.
	var userRunning bool
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE user_id = $1 AND job_type = 'gdpr_export' AND status IN ('queued','running'))`,
		userID).Scan(&userRunning); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if userRunning {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}

	// Server-wide: max 3 concurrent exports.
	var serverCount int
	if err := h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM background_jobs WHERE job_type = 'gdpr_export' AND status IN ('queued','running')`,
	).Scan(&serverCount); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
		return
	}
	if serverCount >= 3 {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "server_busy"})
		return
	}

	// Atomic INSERT — prevents race condition between EXISTS check and INSERT.
	// Also enforces the server-wide limit of 3 concurrent exports atomically (S5 TOCTOU fix).
	jobID := uuid.New().String()
	err = h.DB.Pool.QueryRow(ctx, `
		INSERT INTO background_jobs (id, job_type, user_id, status, started_at, updated_at)
		SELECT $1, 'gdpr_export', $2, 'queued', NOW(), NOW()
		WHERE NOT EXISTS (
			SELECT 1 FROM background_jobs
			WHERE user_id = $2 AND job_type = 'gdpr_export'
			AND status IN ('queued', 'running')
		)
		AND (SELECT COUNT(*) FROM background_jobs
		     WHERE job_type = 'gdpr_export'
		     AND status IN ('queued', 'running')) < 3
		RETURNING true`, jobID, userID).Scan(new(bool))
	if errors.Is(err, pgx.ErrNoRows) {
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]string{"error": "export_already_running"})
		return
	}
	if err != nil {
		slog.Error("gdpr export: insert job failed", "error", err)
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

// runExportJob transitions the job to running, executes the export, and marks it failed on error.
func (h *Handler) runExportJob(ctx context.Context, jobID, userID string) {
	h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'running', updated_at = NOW() WHERE id = $1`, jobID)

	if err := h.executeExportJob(ctx, jobID, userID); err != nil {
		slog.Error("gdpr export job failed", "job_id", jobID, "error", err)
		h.DB.Pool.Exec(ctx, `UPDATE background_jobs SET status = 'failed', error = $2, updated_at = NOW() WHERE id = $1`, jobID, err.Error())
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

// executeExportJobOnce performs the export in a single REPEATABLE READ transaction.
func (h *Handler) executeExportJobOnce(ctx context.Context, jobID, userID string) error {
	// 1. Open REPEATABLE READ read-only TX for consistent snapshot.
	tx, err := h.DB.Pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// 2. Read schema version from TX snapshot.
	var schemaMigration int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&schemaMigration); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}

	// 3. Create temporary ZIP file.
	tmpFile, err := os.CreateTemp("", "skadia-export-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()
	tmpPath := tmpFile.Name()
	// Clean up on failure (success path moves ownership to background_jobs.metadata).
	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmpFile)

	// 4. Export all tables via TX.
	entityCounts := h.exportAllTables(ctx, zw, tx, userID)

	// 5. Commit TX (release snapshot).
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	// 6. ShadowFS files (outside TX — files are immutable).
	if h.ShadowFS != nil {
		if err := h.ShadowFS.StreamToZip(ctx, userID, zw); err != nil {
			slog.Error("data export: file store stream failed", "error", err)
		}
	} else if basePath := h.Config.ShadowFSBasePath; basePath != "" {
		streamShadowFSToZip(basePath, userID, zw)
	}

	// 7. columns.json (needs fresh connection since TX is committed).
	exportedColumns := make(map[string][]string)
	for table := range entityCounts {
		cols := exportColumns(ctx, h.DB.Pool, table)
		if len(cols) > 0 {
			exportedColumns[table] = cols
		}
	}
	writeZipJSON(zw, "columns.json", exportedColumns)

	// 8. manifest.json as LAST file.
	manifest := map[string]any{
		"format_version":   1,
		"schema_migration": schemaMigration,
		"exported_at":      time.Now().Format(time.RFC3339),
		"user_id":          userID,
		"entity_counts":    entityCounts,
	}
	writeZipJSON(zw, "manifest.json", manifest)

	// 9. Close ZIP + flush file to disk before re-opening for SHA-256.
	if err := zw.Close(); err != nil {
		return fmt.Errorf("close zip: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// 10. Compute SHA-256.
	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("reopen for hash: %w", err)
	}
	defer f.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	sizeBytes := fi.Size()
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// 11. Store file metadata, then mark completed (with notification).
	if _, err := h.DB.Pool.Exec(ctx,
		`UPDATE background_jobs SET
			metadata = jsonb_build_object('sha256', $2, 'size_bytes', $3::bigint, 'file_path', $4),
			updated_at = NOW()
		 WHERE id = $1`,
		jobID, checksum, sizeBytes, tmpPath); err != nil {
		return fmt.Errorf("store metadata: %w", err)
	}

	cleanupNeeded = false // tmpFile is now owned by the job record.

	// Mark completed + create notification (single call, avoids double update).
	h.DB.CompleteBackgroundJob(ctx, jobID, false, "")

	// Schedule cleanup after 24h.
	go func(path string) {
		time.Sleep(24 * time.Hour)
		if isValidExportPath(path) {
			os.Remove(path)
		}
	}(tmpPath)

	return nil
}

// exportAllTables runs all table exports within the given transaction, returning entity counts.
func (h *Handler) exportAllTables(ctx context.Context, zw *zip.Writer, tx querier, userID string) map[string]int {
	counts := make(map[string]int)

	// User data exported via profile.json (SELECT * minus deny-list).
	// account.json removed — was redundant subset.

	// All table exports via TX.
	type tableExport struct {
		zipPath string
		table   string
		where   string
	}

	tables := []tableExport{
		{"data/profile.json", "users", "id = $1"},
		{"data/activities.json", "activities", "user_id = $1 ORDER BY started_at DESC"},
		{"data/gear.json", "gear", "user_id = $1"},
		{"data/tags.json", "tags", "user_id = $1"},
		{"data/segments.json", "segments", "created_by = $1"},
		{"data/planned_routes.json", "planned_routes", "user_id = $1"},
		{"data/activity_weather.json", "activity_weather", "activity_id IN (SELECT id FROM activities WHERE user_id = $1)"},
		{"data/workouts.json", "workouts", "user_id = $1"},
		{"data/workout_steps.json", "workout_steps", "workout_id IN (SELECT id FROM workouts WHERE user_id = $1) ORDER BY workout_id, step_order"},
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
	}

	for _, t := range tables {
		n := exportTable(ctx, zw, tx, t.zipPath, t.table, t.where, userID)
		if n > 0 {
			counts[t.table] = n
		}
	}

	// Health data (TimescaleDB hypertables).
	for _, table := range database.AllHealthTables() {
		n := exportTable(ctx, zw, tx, fmt.Sprintf("data/health/%s.json", table), table, "user_id = $1", userID)
		if n > 0 {
			counts[table] = n
		}
	}

	return counts
}

// DataExportStatus handles GET /auth/data-export/{jobID} — returns export job status.
func (h *Handler) DataExportStatus(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	jobID := chi.URLParam(r, "jobID")

	var status, startedAt, updatedAt string
	var errText *string
	var metadata []byte

	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT status, started_at::text, updated_at::text, error, metadata
		 FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'`,
		jobID, userID).Scan(&status, &startedAt, &updatedAt, &errText, &metadata)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	resp := map[string]any{
		"job_id":     jobID,
		"status":     status,
		"started_at": startedAt,
		"updated_at": updatedAt,
	}

	if status == "completed" && len(metadata) > 0 {
		var meta map[string]any
		if json.Unmarshal(metadata, &meta) == nil {
			if v, ok := meta["sha256"]; ok {
				resp["sha256"] = v
			}
			if v, ok := meta["size_bytes"]; ok {
				resp["size_bytes"] = v
			}
			resp["download_url"] = fmt.Sprintf("/api/v1/auth/data-export/%s/download", jobID)
		}
	}

	if status == "failed" && errText != nil {
		resp["error"] = "export_failed" // Don't leak internal error details to client
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, resp)
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
	var metadata []byte
	err := h.DB.Pool.QueryRow(ctx,
		`SELECT status, metadata FROM background_jobs
		 WHERE id = $1 AND user_id = $2 AND job_type = 'gdpr_export'`,
		jobID, userID).Scan(&status, &metadata)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	if status != "completed" {
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
	const maxLegacyZipBytes int64 = 10 << 30 // 10 GB — matches UnifiedShadowFS.StreamToZip limit

	userDir := filepath.Join(basePath, userID)
	if _, err := os.Stat(userDir); os.IsNotExist(err) {
		return
	}

	var totalBytes int64
	err := filepath.Walk(userDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		// Enforce 10 GB limit (same as the ShadowFS/S3 path above).
		totalBytes += info.Size()
		if totalBytes > maxLegacyZipBytes {
			slog.Warn("data export: legacy ShadowFS zip exceeds 10 GB limit",
				"user_id", userID, "total_bytes", totalBytes)
			return fmt.Errorf("zip export exceeds 10 GB limit")
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
