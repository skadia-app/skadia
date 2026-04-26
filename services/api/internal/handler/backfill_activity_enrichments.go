package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/backfill"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// AdminBackfillActivityEnrichments starts the global activity track enrichment backfill.
// The admin middleware performs the role check; this handler only requires the
// authenticated admin ID for background job ownership.
//
// POST /api/v1/admin/backfill-activity-enrichments?dry_run=true&force=true
func (h *Handler) AdminBackfillActivityEnrichments(w http.ResponseWriter, r *http.Request) {
	adminID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	force := r.URL.Query().Get("force") == "true"
	dryRun := r.URL.Query().Get("dry_run") == "true"
	opts := backfill.BackfillOptions{Force: force}

	candidates, err := backfill.DryRunActivityEnrichmentsBackfill(r.Context(), h.DB, opts)
	if err != nil {
		h.writeActivityEnrichmentsBackfillError(w, r, err)
		return
	}
	if dryRun {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status":     "dry_run",
			"candidates": candidates,
			"force":      force,
		})
		return
	}

	entityName := "Activity Track Enrichments"
	jobID, created, err := h.DB.InsertBackgroundJobGuarded(
		r.Context(),
		adminID,
		backfill.ActivityEnrichmentsJobType,
		nil,
		&entityName,
		int64(candidates),
	)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Job creation failed", err.Error(), r.URL.Path)
		return
	}
	if !created {
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	var graphReader routing.GraphReader
	if h.GraphManager != nil {
		graphReader = h.GraphManager.Reader()
	}

	shutdown.Go(jobID, func(ctx context.Context) {
		runErr := backfill.RunActivityEnrichmentsBackfill(ctx, h.DB, graphReader, backfill.BackfillOptions{
			Force:      force,
			OwnedJobID: jobID,
		})
		if runErr != nil {
			slog.Error("activity enrichments backfill failed", "job_id", jobID, "error", runErr)
			_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, true, runErr.Error())
			return
		}
		_ = h.DB.CompleteBackgroundJob(context.Background(), jobID, false, "")
	})

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"status":     "started",
		"job_id":     jobID,
		"candidates": candidates,
		"force":      force,
	})
}

func (h *Handler) writeActivityEnrichmentsBackfillError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, backfill.ErrBackfillRunning):
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
	case errors.Is(err, backfill.ErrTileBuildRunning):
		httputil.WriteJSON(w, http.StatusConflict, map[string]string{
			"status": "blocked",
			"reason": "tile_build_running",
		})
	default:
		slog.Error("activity enrichments backfill preflight failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
	}
}
