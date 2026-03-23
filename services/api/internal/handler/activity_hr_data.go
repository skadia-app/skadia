package handler

import (
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// HandleGetActivityHrData returns high-resolution HR data for an activity.
// GET /api/v1/activities/{id}/hr-data
func (h *Handler) HandleGetActivityHrData(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}
	hrData, err := h.DB.GetActivityHrData(r.Context(), activityID)
	if httputil.HandleDBErr(w, r, err, "hr data not found") {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"hr_data": hrData})
}
