package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// AdminRestoreUser handles POST /admin/users/{id}/restore.
// Re-activates a soft-deleted user (admin only). Can be used during the 30-day
// grace period or even after (as long as PurgeDeletedUsers hasn't run yet).
func (h *Handler) AdminRestoreUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	// Query WITHOUT soft-delete filter — we need to see deleted users.
	var deletedAt *time.Time
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT deleted_at FROM users WHERE id = $1`, targetID).Scan(&deletedAt)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
		return
	}
	if deletedAt == nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "user_not_deleted"})
		return
	}

	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET deleted_at = NULL, is_active = true WHERE id = $1`, targetID)
	if err != nil {
		slog.Error("admin restore user failed", "error", err, "target_user_id", targetID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Clean up cancel_deletion tokens — they're no longer needed.
	_, _ = h.DB.Pool.Exec(r.Context(),
		`DELETE FROM email_tokens WHERE user_id = $1 AND type = 'cancel_deletion'`,
		uuid.MustParse(targetID))

	adminID := middleware.GetUserID(r.Context())
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_restore_user",
		middleware.ClientIP(r), r.UserAgent(),
		map[string]string{"target_user_id": targetID})
	slog.Info("admin restored user", "target_user_id", targetID, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "user_restored"})
}
