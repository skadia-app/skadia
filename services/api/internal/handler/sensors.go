package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) GetSensors(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	_, activityID, ok := h.requireActivity(w, r, userID)
	if !ok {
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
