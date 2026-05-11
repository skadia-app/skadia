package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// RecalculateLifts handles POST /api/v1/lifts/recalculate-all.
//
// Clears the rejected_by_user flag on every lift candidate that belongs to
// the authenticated user so that candidates hidden by a previous soft-reject
// become visible again on the next enrichment pass.
//
// This endpoint only resets the soft-reject flag; it does not trigger
// re-enrichment or recompute aggregate statistics. Lift aggregates are
// refreshed on the next regular enrichment pass for each affected activity.
//
// Response on success:
//
//	200 {"cleared_count": N, "affected_activities": M}
//
// Returns 204 when no rows were in rejected state.
func (h *Handler) RecalculateLifts(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	cleared, affected, err := h.DB.ResetSoftRejectedLiftCandidates(r.Context(), userID)
	if err != nil {
		slog.Error("recalculate lifts: reset soft-rejected candidates failed",
			"user_id", userID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error",
			"failed to reset lift candidates", r.URL.Path)
		return
	}

	if cleared == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	type response struct {
		ClearedCount       int `json:"cleared_count"`
		AffectedActivities int `json:"affected_activities"`
	}
	httputil.WriteJSON(w, http.StatusOK, response{
		ClearedCount:       cleared,
		AffectedActivities: affected,
	})
}
