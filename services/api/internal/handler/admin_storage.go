// All handlers in this file MUST be registered inside the admin middleware group.
// They do NOT perform their own admin role check — they rely on RequireAdmin() middleware.
package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/filestore"
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

// StorageDashboard returns an overview of the unified FileStore state.
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

	httputil.WriteJSON(w, http.StatusOK, storageDashboardResponse{
		Backend:      h.Config.StorageBackend,
		BasePath:     h.Config.LibraryBasePath,
		KeepVersions: h.Config.KeepVersions,
		SyncEnabled:  h.Config.SyncEnabled,
		Users:        users,
		TypeCounts:   typeCounts,
	})
}

// StartRegistryPopulation triggers the background migration job that
// populates user_files from all legacy domain-table storage columns.
//
// POST /api/v1/admin/storage/populate-registry
func (h *Handler) StartRegistryPopulation(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(
		r.Context(),
		userID,
		"filestore_migration",
		nil, nil,
		0, // progress_total updated incrementally inside Run()
	)
	if err != nil {
		slog.Error("admin: failed to insert filestore_migration job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "could not create job", r.URL.Path)
		return
	}

	// LibraryBasePath serves as shadowFSBase for local-file path resolution.
	// s3Bucket is left empty: the migration job derives S3 paths from stored
	// keys directly; extractS3Key() handles bare keys without a bucket prefix.
	job := filestore.NewMigrateJob(h.DB.Pool, h.Config.LibraryBasePath, "")

	db := h.DB
	shutdown.Go(jobID, func(ctx context.Context) {
		if runErr := job.Run(ctx, jobID); runErr != nil {
			slog.Error("admin: filestore migration failed", "job_id", jobID, "error", runErr)
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

// StartFileMigration triggers the background job that physically moves files
// from legacy ShadowFS locations to the new Library directory structure (Phase 2).
//
// POST /api/v1/admin/storage/migrate
func (h *Handler) StartFileMigration(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.Config.LibraryBasePath == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"LIBRARY_BASE_PATH not configured", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(
		r.Context(),
		userID,
		"filestore_file_move",
		nil, nil,
		0, // progress_total set inside RunFileMove
	)
	if err != nil {
		slog.Error("admin: failed to insert filestore_file_move job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"could not create job", r.URL.Path)
		return
	}

	renderer := filestore.NewSchemaRenderer()
	job := filestore.NewMigrateJobWithLibrary(
		h.DB.Pool, h.Config.LibraryBasePath, h.Config.LibraryBasePath, "", renderer,
	)

	db := h.DB
	shutdown.Go(jobID, func(ctx context.Context) {
		if runErr := job.RunFileMove(ctx, jobID); runErr != nil {
			slog.Error("admin: file migration failed", "job_id", jobID, "error", runErr)
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

// StartReconciliation triggers a one-off filestore reconciliation job that
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

	jobID, err := h.DB.InsertBackgroundJob(
		r.Context(),
		userID,
		"filestore_reconcile",
		nil, nil,
		9, // 9 checks
	)
	if err != nil {
		slog.Error("admin: failed to insert filestore_reconcile job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"could not create job", r.URL.Path)
		return
	}

	renderer := filestore.NewSchemaRenderer()
	job := filestore.NewReconcileJob(h.DB.Pool, h.Config.LibraryBasePath, renderer, h.Config.KeepVersions)

	db := h.DB
	shutdown.Go(jobID, func(ctx context.Context) {
		_, runErr := job.Run(ctx, func(current int) {
			db.UpdateBackgroundJobProgress(ctx, jobID, int64(current)) //nolint:errcheck
		})
		if runErr != nil {
			slog.Error("admin: filestore reconciliation failed", "job_id", jobID, "error", runErr)
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

// StartFileMigrationRollback triggers a background job that reverses Phase 2
// file moves, restoring files to their original locations.
//
// POST /api/v1/admin/storage/migrate/rollback
func (h *Handler) StartFileMigrationRollback(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.Config.LibraryBasePath == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"LIBRARY_BASE_PATH not configured", r.URL.Path)
		return
	}

	jobID, err := h.DB.InsertBackgroundJob(
		r.Context(),
		userID,
		"filestore_rollback",
		nil, nil,
		0,
	)
	if err != nil {
		slog.Error("admin: failed to insert filestore_rollback job", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"could not create job", r.URL.Path)
		return
	}

	renderer := filestore.NewSchemaRenderer()
	job := filestore.NewMigrateJobWithLibrary(
		h.DB.Pool, h.Config.LibraryBasePath, h.Config.LibraryBasePath, "", renderer,
	)

	db := h.DB
	shutdown.Go(jobID, func(ctx context.Context) {
		if runErr := job.RunRollback(ctx, jobID); runErr != nil {
			slog.Error("admin: file migration rollback failed", "job_id", jobID, "error", runErr)
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
