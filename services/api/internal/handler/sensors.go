package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetSensors(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	activityID := chi.URLParam(r, "id")

	activity, err := h.DB.GetActivity(r.Context(), activityID)
	if err != nil || activity == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}
	if activity.UserID != userID {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "activity not found", r.URL.Path)
		return
	}

	sensors, err := h.DB.GetSensors(r.Context(), activityID)
	if err != nil {
		slog.Error("get sensors failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get sensors", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, sensors)
}
