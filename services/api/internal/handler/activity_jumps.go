package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleGetActivityJumps returns all jump records for an activity.
// GET /api/v1/activities/{id}/jumps
func (h *Handler) HandleGetActivityJumps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	limit, offset := httputil.ParsePagination(r, 50, 200)
	jumps, err := h.DB.GetActivityJumps(r.Context(), activityID, limit, offset)
	if httputil.HandleDBErr(w, r, err, "jumps not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"jumps": jumps})
}
