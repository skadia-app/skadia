// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/imageutil"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
	"github.com/skadia-app/skadia/services/api/internal/sidecar"
)

// storageDashboardUserRow holds per-user storage statistics.
type storageDashboardUserRow struct {
	UserID           string `json:"user_id"`
	Username         string `json:"username"`
	Email            string `json:"email"`
	LibraryDir       string `json:"library_dir"`
	DiskUsageBytes   int64  `json:"disk_usage_bytes"`
	FileCount        int64  `json:"file_count"`
	ArchiveSizeBytes int64  `json:"archive_size_bytes"`
}

// storageDashboardResponse is the JSON shape for GET /api/v1/admin/storage.
type storageDashboardResponse struct {
	Backend      string                    `json:"backend"`
	BasePath     string                    `json:"base_path"`
	KeepVersions int                       `json:"keep_versions"`
	SyncEnabled  bool                      `json:"sync_enabled"`
	Users        []storageDashboardUserRow `json:"users"`
	TypeCounts   map[string]int64          `json:"type_counts"`
}

// StorageDashboard returns an overview of the unified ShadowFS state.
//
// GET /api/v1/admin/storage
func (h *Handler) StorageDashboard(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	ctx := r.Context()

	// ── per-user statistics ────────────────────────────────────────────────
	userRows, err := h.DB.Pool.Query(ctx, `
		SELECT
			u.id::text,
			u.username,
			COALESCE(u.email, '') AS email,
			COALESCE(u.library_dir_name, u.id::text) AS library_dir,
			COALESCE(SUM(uf.size_bytes) FILTER (WHERE uf.status = 'active'),  0) AS disk_usage_bytes,
			COUNT(uf.id)                              FILTER (WHERE uf.status = 'active')   AS file_count,
			COALESCE(SUM(uf.size_bytes) FILTER (WHERE uf.status = 'archived'), 0) AS archive_size_bytes
		FROM users u
		LEFT JOIN user_files uf ON uf.user_id = u.id
		GROUP BY u.id, u.username, u.email, u.library_dir_name
		ORDER BY u.id
	`)
	if err != nil {
		slog.Error("admin/storage: query user stats", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not query user stats", r.URL.Path)
		return
	}
	defer userRows.Close()

	users := make([]storageDashboardUserRow, 0)
	for userRows.Next() {
		var row storageDashboardUserRow
		if err := userRows.Scan(&row.UserID, &row.Username, &row.Email, &row.LibraryDir, &row.DiskUsageBytes, &row.FileCount, &row.ArchiveSizeBytes); err != nil {
			slog.Error("admin/storage: scan user row", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not scan user row", r.URL.Path)
			return
		}
		users = append(users, row)
	}
	if err := userRows.Err(); err != nil {
		slog.Error("admin/storage: iterate user rows", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not iterate user rows", r.URL.Path)
		return
	}

	// ── per-type counts (active files only) ───────────────────────────────
	typeRows, err := h.DB.Pool.Query(ctx, `
		SELECT file_type, COUNT(*) AS cnt
		FROM user_files
		WHERE status = 'active'
		GROUP BY file_type
		ORDER BY file_type
	`)
	if err != nil {
		slog.Error("admin/storage: query type counts", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not query type counts", r.URL.Path)
		return
	}
	defer typeRows.Close()

	typeCounts := make(map[string]int64)
	for typeRows.Next() {
		var ft string
		var cnt int64
		if err := typeRows.Scan(&ft, &cnt); err != nil {
			slog.Error("admin/storage: scan type row", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not scan type row", r.URL.Path)
			return
		}
		typeCounts[ft] = cnt
	}
	if err := typeRows.Err(); err != nil {
		slog.Error("admin/storage: iterate type rows", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not iterate type rows", r.URL.Path)
		return
	}

	// Use a symbolic label instead of the real container path (I9: avoid leaking internal paths).
	basePath := "local"
	if h.Config.StorageBackend == "s3" {
		basePath = "s3"
	}

	keepVersions, _ := strconv.Atoi(h.Settings.Get("library.keep_versions").Value)
	syncEnabled := h.Settings.Get("library.sync_enabled").Value == "true"

	httputil.WriteJSON(w, http.StatusOK, storageDashboardResponse{
		Backend:      h.Config.StorageBackend,
		BasePath:     basePath,
		KeepVersions: keepVersions,
		SyncEnabled:  syncEnabled,
		Users:        users,
		TypeCounts:   typeCounts,
	})
}

// shadowfsMigrationJobTypes lists all job types that operate on the user_files
// registry or the library filesystem. These are mutually exclusive: running two
// simultaneously would produce conflicting journal entries and race on file moves.
var shadowfsMigrationJobTypes = []string{
	"shadowfs_reconcile",
}

// hasRunningShadowfsJob returns true if any of the shadowfs migration/reconcile
// job types is currently running. Used for cross-job mutual exclusion (M23).
func (h *Handler) hasRunningShadowfsJob(ctx context.Context) (bool, error) {
	for _, jt := range shadowfsMigrationJobTypes {
		running, err := h.DB.HasRunningJobGlobal(ctx, jt)
		if err != nil {
			return false, err
		}
		if running {
			return true, nil
		}
	}
	return false, nil
}

// StartReconciliation triggers a one-off shadowfs reconciliation job that
// checks consistency between the filesystem and the user_files registry.
//
// POST /api/v1/admin/storage/reconcile
func (h *Handler) StartReconciliation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.Config.LibraryBasePath == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"LIBRARY_BASE_PATH not configured", r.URL.Path)
		return
	}

	// L33: Reconcile only makes sense for local storage — it walks the local filesystem.
	if h.Config.StorageBackend != "local" {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{
			"status": "skipped",
			"reason": "reconciliation only applies to storage_backend=local",
		})
		return
	}

	// M23: prevent concurrent shadowfs jobs of any type.
	if running, err := h.hasRunningShadowfsJob(r.Context()); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not check running jobs", r.URL.Path)
		return
	} else if running {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	jobID, inserted, err := h.DB.InsertBackgroundJobGuarded(
		r.Context(),
		userID,
		"shadowfs_reconcile",
		nil, nil,
		9, // 9 checks
	)
	if err != nil {
		slog.Error("admin: failed to insert shadowfs_reconcile job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"could not create job", r.URL.Path)
		return
	}
	if !inserted {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	renderer := shadowfs.NewSchemaRenderer()
	job := shadowfs.NewReconcileJob(h.DB.Pool, h.Config.LocalBE, h.Config.S3BE, h.Config.LibraryBasePath, h.Config.BucketFn, renderer, h.Config.KeepVersions)

	db := h.DB
	shutdown.Go(jobID, func(ctx context.Context) {
		_, runErr := job.Run(ctx, func(current int) {
			db.UpdateBackgroundJobProgress(ctx, jobID, int64(current)) //nolint:errcheck
		})
		if runErr != nil {
			slog.Error("admin: shadowfs reconciliation failed", "job_id", jobID, "error", runErr)
			db.CompleteBackgroundJob(ctx, jobID, true, runErr.Error()) //nolint:errcheck
			return
		}
		db.CompleteBackgroundJob(ctx, jobID, false, "") //nolint:errcheck
	})

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"status": "started",
		"job_id": jobID,
	})
}

// BackfillThumbnails generates missing thumbnails for all activity photos whose
// original media file is stored in ShadowFS but that have no 'thumb' derivative.
// It runs globally across all users (admin-only endpoint).
//
// POST /api/v1/admin/storage/backfill-thumbnails
func (h *Handler) BackfillThumbnails(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "skipped", "reason": "ShadowFS not configured"})
		return
	}

	// Check whether a thumbnail_backfill job is already running (global, any user).
	var alreadyRunning bool
	if err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE job_type = 'thumbnail_backfill' AND status IN ('pending', 'running'))`,
	).Scan(&alreadyRunning); err != nil {
		slog.Error("thumbnail backfill: check running job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not check running jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// Collect photos without thumbnails across all users (admin-global scope).
	missing, err := h.DB.PhotosWithoutThumb(r.Context())
	if err != nil {
		slog.Error("thumbnail backfill: list missing thumbs failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not list photos without thumbnails", r.URL.Path)
		return
	}

	if len(missing) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": 0})
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "thumbnail_backfill", nil, nil, int64(len(missing)))
	if err != nil {
		slog.Error("thumbnail backfill: insert job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create job", r.URL.Path)
		return
	}

	fs := h.ShadowFS
	db := h.DB

	shutdown.Go(jobID, func(ctx context.Context) {
		sem := make(chan struct{}, 4)
		var mu sync.Mutex
		processed := int64(0)
		anyFailed := false

		var wg sync.WaitGroup
		for _, row := range missing {
			wg.Add(1)
			sem <- struct{}{}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				// Read original media.
				rc, getErr := fs.Get(ctx, row.UserID, row.FileID)
				if getErr != nil {
					slog.Warn("thumbnail backfill: ShadowFS.Get failed", "file_id", row.FileID, "error", getErr)
					mu.Lock()
					anyFailed = true
					processed++
					mu.Unlock()
					_ = db.UpdateBackgroundJobProgress(ctx, jobID, processed) //nolint:errcheck
					return
				}
				data, readErr := io.ReadAll(rc)
				rc.Close()
				if readErr != nil {
					slog.Warn("thumbnail backfill: read failed", "file_id", row.FileID, "error", readErr)
					mu.Lock()
					anyFailed = true
					processed++
					mu.Unlock()
					_ = db.UpdateBackgroundJobProgress(ctx, jobID, processed) //nolint:errcheck
					return
				}

				// Generate thumbnail.
				thumbData, resizeErr := imageutil.Resize(data, 200, 200, 80)
				if resizeErr != nil || thumbData == nil {
					slog.Warn("thumbnail backfill: resize failed", "file_id", row.FileID, "error", resizeErr)
					mu.Lock()
					anyFailed = true
					processed++
					mu.Unlock()
					_ = db.UpdateBackgroundJobProgress(ctx, jobID, processed) //nolint:errcheck
					return
				}

				// Derive a stable file_hash for the thumbnail (original hash + suffix).
				thumbHash := fmt.Sprintf("%s_thumb", row.FileID)
				thumbName := row.FileID + "_thumb.jpg"

				_, putErr := fs.Put(ctx, shadowfs.FileEntry{
					UserID:      row.UserID,
					FileType:    "thumb",
					ContentType: "image/jpeg",
					FileName:    thumbName,
					SizeBytes:   int64(len(thumbData)),
					FileHash:    thumbHash,
					ParentType:  new("photo"),
					ParentID:    new(row.ParentID),
				}, bytes.NewReader(thumbData))
				if putErr != nil {
					slog.Warn("thumbnail backfill: ShadowFS.Put thumb failed", "parent_id", row.ParentID, "error", putErr)
					mu.Lock()
					anyFailed = true
					mu.Unlock()
				}

				mu.Lock()
				processed++
				mu.Unlock()
				_ = db.UpdateBackgroundJobProgress(ctx, jobID, processed) //nolint:errcheck
			}()
		}
		wg.Wait()

		_ = db.CompleteBackgroundJob(ctx, jobID, anyFailed, "")
		slog.Info("thumbnail backfill: completed", "user_id", userID, "job_id", jobID, "processed", processed, "any_failed", anyFailed)
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id": jobID,
		"queued": len(missing),
	})
}

// BackfillMetaSidecars generates missing .meta companions for:
//   - Phase 1: activity files that don't have a sidecar yet (has_sidecar=false)
//   - Phase 2: photo files with manually assigned locations (has_sidecar=false)
//
// Runs as a background job with 4 concurrent workers.
//
// POST /api/v1/admin/storage/backfill-meta
func (h *Handler) BackfillMetaSidecars(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if h.ShadowFS == nil {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "skipped", "reason": "ShadowFS not configured"})
		return
	}

	// Check for already-running job.
	var alreadyRunning bool
	if err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM background_jobs WHERE job_type = 'meta_backfill' AND status IN ('pending', 'running'))`,
	).Scan(&alreadyRunning); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not check running jobs", r.URL.Path)
		return
	}
	if alreadyRunning {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// ── Phase 1: activity files missing companions ─────────────────────────

	type backfillRow struct {
		FileID     string
		UserID     string
		ActivityID string
	}

	rows, err := h.DB.Pool.Query(r.Context(), `
		SELECT uf.id, uf.user_id, a.id AS activity_id
		FROM user_files uf
		JOIN activities a ON a.file_id = uf.id
		WHERE uf.file_type = 'activity' AND uf.status = 'active'
		  AND uf.has_sidecar = false
		ORDER BY a.started_at`)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not query activities", r.URL.Path)
		return
	}
	defer rows.Close()

	var missing []backfillRow
	for rows.Next() {
		var row backfillRow
		if err := rows.Scan(&row.FileID, &row.UserID, &row.ActivityID); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "scan failed", r.URL.Path)
			return
		}
		missing = append(missing, row)
	}
	if err := rows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "query error", r.URL.Path)
		return
	}

	// ── Phase 2: photo files with manual locations missing companions ───────

	type photoBackfillRow struct {
		FileID         string
		UserID         string
		Lat            float64
		Lng            float64
		TakenAt        *time.Time
		LocationSource string
	}

	photoRows, err := h.DB.Pool.Query(r.Context(), `
		SELECT uf.id AS file_id, uf.user_id, ap.lat, ap.lng, ap.taken_at, ap.location_source
		FROM activity_photos ap
		JOIN user_files uf ON uf.id = ap.file_id
		WHERE ap.lat IS NOT NULL
		  AND ap.location_source = 'manual'
		  AND uf.has_sidecar = false
		  AND uf.status = 'active'`)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not query photos", r.URL.Path)
		return
	}
	defer photoRows.Close()

	var missingPhotos []photoBackfillRow
	for photoRows.Next() {
		var row photoBackfillRow
		if err := photoRows.Scan(&row.FileID, &row.UserID, &row.Lat, &row.Lng, &row.TakenAt, &row.LocationSource); err != nil {
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "scan photo row failed", r.URL.Path)
			return
		}
		missingPhotos = append(missingPhotos, row)
	}
	if err := photoRows.Err(); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "photo query error", r.URL.Path)
		return
	}

	total := len(missing) + len(missingPhotos)
	if total == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "processed": 0})
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID, "meta_backfill", nil, nil, int64(total))
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create job", r.URL.Path)
		return
	}

	fs := h.ShadowFS
	db := h.DB

	type photoMetaSidecar struct {
		Lat            float64    `json:"lat"`
		Lng            float64    `json:"lng"`
		TakenAt        *time.Time `json:"taken_at,omitempty"`
		LocationSource string     `json:"location_source"`
		UpdatedAt      time.Time  `json:"updated_at"`
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		sem := make(chan struct{}, 4)
		var mu sync.Mutex
		var processed int64
		anyFailed := false
		var errMsgs []string

		var wg sync.WaitGroup

		// Phase 1: activity companions.
	outer:
		for _, row := range missing {
			wg.Add(1)
			select {
			case <-ctx.Done():
				wg.Done()
				break outer
			case sem <- struct{}{}:
			}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				act, getErr := db.GetActivity(ctx, row.ActivityID)
				if getErr != nil || act == nil {
					mu.Lock()
					anyFailed = true
					if len(errMsgs) < 5 {
						msg := row.ActivityID + ": activity not found"
						if getErr != nil {
							msg = getErr.Error()
						}
						errMsgs = append(errMsgs, msg)
					}
					mu.Unlock()
				} else {
					sidecar.WriteMetaCompanion(ctx, fs, db, row.UserID, row.ActivityID, row.FileID, nil, act)
				}

				cur := atomic.AddInt64(&processed, 1)
				_ = db.UpdateBackgroundJobProgress(ctx, jobID, cur)
			}()
		}
		wg.Wait()

		// Phase 2: photo location companions.
		wg = sync.WaitGroup{}
	outerPhoto:
		for _, row := range missingPhotos {
			wg.Add(1)
			select {
			case <-ctx.Done():
				wg.Done()
				break outerPhoto
			case sem <- struct{}{}:
			}
			go func() {
				defer wg.Done()
				defer func() { <-sem }()

				meta := photoMetaSidecar{
					Lat:            row.Lat,
					Lng:            row.Lng,
					TakenAt:        row.TakenAt,
					LocationSource: row.LocationSource,
					UpdatedAt:      time.Now().UTC(),
				}
				jsonBytes, marshalErr := json.Marshal(meta)
				if marshalErr != nil {
					slog.Warn("meta backfill: photo marshal failed", "file_id", row.FileID, "error", marshalErr)
					mu.Lock()
					anyFailed = true
					if len(errMsgs) < 5 {
						errMsgs = append(errMsgs, fmt.Sprintf("photo %s: %v", row.FileID, marshalErr))
					}
					mu.Unlock()
				} else if putErr := fs.PutCompanion(ctx, row.UserID, row.FileID, ".meta", jsonBytes); putErr != nil {
					slog.Warn("meta backfill: photo PutCompanion failed", "file_id", row.FileID, "error", putErr)
					mu.Lock()
					anyFailed = true
					if len(errMsgs) < 5 {
						errMsgs = append(errMsgs, fmt.Sprintf("photo %s: %v", row.FileID, putErr))
					}
					mu.Unlock()
				}

				cur := atomic.AddInt64(&processed, 1)
				_ = db.UpdateBackgroundJobProgress(ctx, jobID, cur)
			}()
		}
		wg.Wait()

		_ = db.CompleteBackgroundJob(ctx, jobID, anyFailed, strings.Join(errMsgs, "; "))
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":            jobID,
		"queued":            total,
		"queued_activities": len(missing),
		"queued_photos":     len(missingPhotos),
	})
}
