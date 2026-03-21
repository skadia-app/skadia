package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

var validReEnrichModules = []string{
	"ascents", "stops", "intervals",
	"peaks", "settlements", "forests", "water", "borders", "surfaces",
}

// ReEnrichActivities re-runs selected enrichment modules for the user's activities
// without downloading or re-parsing original files from S3.
//
// POST /api/v1/activities/re-enrich
func (h *Handler) ReEnrichActivities(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Modules []string `json:"modules"`
		IDs     []string `json:"ids,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Modules) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "modules required", r.URL.Path)
		return
	}

	// Validate modules
	for _, m := range req.Modules {
		if !slices.Contains(validReEnrichModules, m) {
			writeError(w, http.StatusBadRequest, "Bad Request",
				fmt.Sprintf("invalid module: %s", m), r.URL.Path)
			return
		}
	}

	// Check for already-running re-enrich job
	running, err := h.DB.HasRunningJob(r.Context(), userID, "activity_re_enrich")
	if err != nil {
		slog.Warn("re-enrich: check running job failed", "error", err)
	}
	if running {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
		return
	}

	// List activity IDs (user-scoped)
	var filter *database.ReprocessFilter
	if len(req.IDs) > 0 {
		filter = &database.ReprocessFilter{IDs: req.IDs}
	}
	ids, err := h.DB.ListActivityIDsFiltered(r.Context(), userID, filter)
	if err != nil {
		slog.Error("re-enrich: list ids failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"status": "no_activities", "total": 0})
		return
	}

	// Create background job
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"activity_re_enrich", nil, nil, int64(len(ids)))
	if err != nil {
		slog.Error("re-enrich: create job failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
		return
	}

	// Dispatch async
	shutdown.Go(jobID, func(ctx context.Context) {
		var gr routing.GraphReader
		if h.GraphManager != nil {
			gr = h.GraphManager.Reader()
		}
		enrichment.ReEnrichBatch(ctx, h.DB, h.Events, userID, req.Modules, ids, jobID, gr)
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":  jobID,
		"status":  "running",
		"total":   len(ids),
		"modules": req.Modules,
	})
}
