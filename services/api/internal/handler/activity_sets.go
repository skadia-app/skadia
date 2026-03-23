package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleGetActivitySets returns all exercise sets for an activity.
// GET /api/v1/activities/{id}/sets
func (h *Handler) HandleGetActivitySets(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	sets, err := h.DB.GetActivitySets(r.Context(), activityID)
	if httputil.HandleDBErr(w, r, err, "sets not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"sets": sets})
}
