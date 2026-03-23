package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// ImportLiftsFromOSM creates lift segments from osm_lifts.
// POST /api/v1/segments/import-lifts
func (h *Handler) ImportLiftsFromOSM(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	count, err := h.DB.ImportLiftSegments(r.Context(), userID)
	if err != nil {
		slog.Error("import lifts failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Import Failed", err.Error(), r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"imported": count})
}
