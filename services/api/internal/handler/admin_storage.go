package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/filestore"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

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
