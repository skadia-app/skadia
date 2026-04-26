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
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

var validReEnrichModules = []string{
	"ascents", "stops", "intervals",
	"summits", "settlements", "forests", "water", "borders", "surfaces",
}

// geoSubmodules lists the individual geo module names that the legacy "geo"
// aggregate key expands to. Kept in sync with recompute.go scope handling.
var geoSubmodules = []string{
	"summits", "settlements", "forests", "water", "borders", "surfaces",
}

// ReEnrichActivities re-runs selected enrichment modules for the user's activities
// without downloading or re-parsing original files from S3.
//
// POST /api/v1/activities/re-enrich
func (h *Handler) ReEnrichActivities(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Modules []string `json:"modules"`
		IDs     []string `json:"ids,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Modules) == 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "modules required", r.URL.Path)
		return
	}

	// Expand the legacy "geo" aggregate key to individual submodule names so that
	// re_enrich.go and recompute.go accept the same module vocabulary.
	expanded := make([]string, 0, len(req.Modules))
	for _, m := range req.Modules {
		if m == "geo" {
			expanded = append(expanded, geoSubmodules...)
		} else {
			expanded = append(expanded, m)
		}
	}
	// Deduplicate (a request may mix "geo" and explicit names).
	seen := make(map[string]struct{}, len(expanded))
	deduped := expanded[:0]
	for _, m := range expanded {
		if _, ok := seen[m]; !ok {
			seen[m] = struct{}{}
			deduped = append(deduped, m)
		}
	}
	req.Modules = deduped

	// Validate modules
	for _, m := range req.Modules {
		if !slices.Contains(validReEnrichModules, m) {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
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
		httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "already_running"})
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
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list activities", r.URL.Path)
		return
	}
	if len(ids) == 0 {
		httputil.WriteJSON(w, http.StatusOK, map[string]any{"status": "no_activities", "total": 0})
		return
	}

	// Create background job
	jobID, err := h.DB.InsertBackgroundJob(r.Context(), userID,
		"activity_re_enrich", nil, nil, int64(len(ids)))
	if err != nil {
		slog.Error("re-enrich: create job failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create job", r.URL.Path)
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

	httputil.WriteJSON(w, http.StatusAccepted, map[string]any{
		"job_id":  jobID,
		"status":  "running",
		"total":   len(ids),
		"modules": req.Modules,
	})
}
