package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetActivityHealthContext returns health context (sleep, HRV, resting HR)
// for the night/morning before the given activity.
// GET /api/v1/activities/{id}/health-context
func (h *Handler) GetActivityHealthContext(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	activity, _, ok := h.requireActivity(w, r, userID)
	if !ok {
		return
	}

	hc, err := h.DB.GetHealthContextForActivity(r.Context(), userID, activity.StartedAt)
	if err != nil {
		slog.Error("get health context failed", "activity_id", activity.ID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get health context", r.URL.Path)
		return
	}

	if hc == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, hc)
}
