package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetSensors(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	sensors, err := h.DB.GetSensors(r.Context(), activityID)
	if err != nil {
		slog.Error("get sensors failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get sensors", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, sensors)
}
