package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleGetActivitySegmentLaps returns segment lap data for an activity.
// GET /api/v1/activities/{id}/segment-laps
func (h *Handler) HandleGetActivitySegmentLaps(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	segmentLaps, err := h.DB.GetActivitySegmentLaps(r.Context(), activityID)
	if httputil.HandleDBErr(w, r, err, "segment laps not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"segment_laps": segmentLaps})
}
