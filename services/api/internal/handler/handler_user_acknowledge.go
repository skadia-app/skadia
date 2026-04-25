package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// AcknowledgeSurfaceExtension handles POST /api/v1/users/me/acknowledge/surface-extension.
func (h *Handler) AcknowledgeSurfaceExtension(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	if _, err := h.DB.Pool.Exec(
		r.Context(),
		`UPDATE users SET surface_extension_acknowledged_at = NOW() WHERE id = $1`,
		userID,
	); err != nil {
		slog.Error("acknowledge surface extension failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to acknowledge surface extension", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
