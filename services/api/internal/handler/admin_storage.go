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
