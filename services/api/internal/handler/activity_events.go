package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleGetActivityEvents returns all events for an activity.
// GET /api/v1/activities/{id}/events
func (h *Handler) HandleGetActivityEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	limit, offset := httputil.ParsePagination(r, 100, 500)
	events, err := h.DB.GetActivityEvents(r.Context(), activityID, limit, offset)
	if httputil.HandleDBErr(w, r, err, "events not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"events": events})
}
