// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// storageDashboardUserRow holds per-user storage statistics.
type storageDashboardUserRow struct {
	UserID           string `json:"user_id"`
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
			COALESCE(u.library_dir_name, u.id::text) AS library_dir,
			COALESCE(SUM(uf.size_bytes) FILTER (WHERE uf.status = 'active'),  0) AS disk_usage_bytes,
			COUNT(uf.id)                              FILTER (WHERE uf.status = 'active')   AS file_count,
			COALESCE(SUM(uf.size_bytes) FILTER (WHERE uf.status = 'archived'), 0) AS archive_size_bytes
		FROM users u
		LEFT JOIN user_files uf ON uf.user_id = u.id
		GROUP BY u.id, u.library_dir_name
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
		if err := userRows.Scan(&row.UserID, &row.LibraryDir, &row.DiskUsageBytes, &row.FileCount, &row.ArchiveSizeBytes); err != nil {
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

	httputil.WriteJSON(w, http.StatusOK, storageDashboardResponse{
		Backend:      h.Config.StorageBackend,
		BasePath:     basePath,
		KeepVersions: h.Config.KeepVersions,
		SyncEnabled:  h.Config.SyncEnabled,
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

