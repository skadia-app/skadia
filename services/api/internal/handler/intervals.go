package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetActivityIntervals returns detected training intervals for an activity.
// GET /api/v1/activities/{id}/intervals
func (h *Handler) GetActivityIntervals(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activityID := chi.URLParam(r, "id")
	if activityID == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing activity ID", r.URL.Path)
		return
	}

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil {
		slog.Error("get activity intervals: get activity failed", "id", activityID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activity", r.URL.Path)
		return
	}
	if activity == nil || activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
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
