package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetActivityIntervals returns detected training intervals for an activity.
// GET /api/v1/activities/{id}/intervals
func (h *Handler) GetActivityIntervals(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	intervals, err := h.DB.GetActivityIntervals(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity intervals failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get activity intervals", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, intervals)
}
