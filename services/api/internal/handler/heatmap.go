package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

func (h *Handler) BackfillHeatmap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	count, err := h.DB.BackfillHeatmap(r.Context(), userID)
	if err != nil {
		slog.Error("backfill heatmap failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to backfill heatmap", r.URL.Path)
		return
	}

	slog.Info("heatmap backfill complete", "user_id", userID, "processed", count)

	writeJSON(w, http.StatusOK, map[string]int{"processed": count})
}
