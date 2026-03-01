package handler

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) GetStreams(w http.ResponseWriter, r *http.Request) {
	activityID := chi.URLParam(r, "id")

	streams, err := h.DB.GetStreams(r.Context(), activityID)
	if err != nil {
		slog.Error("get streams failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to get streams", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, streams)
}
