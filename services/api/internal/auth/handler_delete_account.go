package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// DeleteAccount handles POST /auth/delete-account (authenticated).
// Performs a soft-delete: sets deleted_at + is_active=false, revokes all tokens,
// and sends a cancellation email with a 30-day grace period.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)

	userID := middleware.GetUserID(r.Context())
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		Password     string `json:"password,omitempty"`
		Confirmation string `json:"confirmation,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Authentication: password-based users must provide their password,
	// OAuth-only users must type a confirmation phrase.
	// Do this before opening the transaction to avoid holding locks during slow
	// argon2id computation.
	if user.PasswordHash != nil {
		match, err := argon2id.ComparePasswordAndHash(req.Password, *user.PasswordHash)
		if err != nil || !match {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "wrong_password"})
			return
		}
	} else {
		if req.Confirmation != "ACCOUNT LÖSCHEN" {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_confirmation"})
			return
		}
	}

	tx, err := h.DB.Pool.Begin(r.Context())
	if err != nil {
		slog.Error("delete account: begin tx failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer tx.Rollback(r.Context())

	// Last-admin check: SELECT … FOR UPDATE serializes concurrent self-deletions
	// so the second request sees the first's committed result.
	if user.IsAdmin {
		var otherAdmins int
		if err := tx.QueryRow(r.Context(),
			`SELECT count(*) FROM users WHERE is_admin = true AND id != $1
			 AND is_active = true AND deleted_at IS NULL
			 FOR UPDATE`, userID).Scan(&otherAdmins); err != nil {
			slog.Error("delete account: count other admins failed", "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if otherAdmins == 0 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "last_admin"})
			return
		}
	}

	// Soft-delete: mark user as deleted, deactivate, clear pending email change.
	if _, err := tx.Exec(r.Context(),
		`UPDATE users SET deleted_at = NOW(), is_active = false, pending_email = NULL
		 WHERE id = $1`, userID); err != nil {
		slog.Error("delete account: soft-delete failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Clean up pending email-change tokens inside the same transaction.
	if _, err := tx.Exec(r.Context(),
		`DELETE FROM email_tokens WHERE user_id = $1 AND type = 'email_change'`,
		uuid.MustParse(userID)); err != nil {
		slog.Error("delete account: cleanup email_tokens failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("delete account: commit failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Revoke all sessions to force immediate logout everywhere.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Warn("delete account: revoke tokens failed", "error", err)
	}

	// Send cancellation email with a 30-day cancel link.
	if h.Mailer.IsConfigured() && user.Email != nil {
		cancelToken, err := GenerateToken()
		if err == nil {
			if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(userID),
				HashToken(cancelToken), "cancel_deletion",
				time.Now().Add(30*24*time.Hour)); err != nil {
				slog.Error("delete account: create cancel token failed", "error", err)
			} else {
				go h.Mailer.SendDeletionEmail(*user.Email, user.Username, cancelToken)
			}
		}
	}

	ip := middleware.ClientIP(r)
	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "account_deletion_requested", ip, r.UserAgent(), nil)
	slog.Info("account deletion requested", "user_id", userID)

	purgeAt := time.Now().Add(30 * 24 * time.Hour)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"status":   "deletion_scheduled",
		"purge_at": purgeAt.Format(time.RFC3339),
	})
}

// CancelDeletion handles POST /auth/cancel-deletion (public, token-based).
// Restores a soft-deleted user within the 30-day grace period.
func (h *Handler) CancelDeletion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	userID, err := h.DB.ConsumeEmailToken(r.Context(), HashToken(req.Token), "cancel_deletion")
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_or_expired_token"})
		return
	}

	if _, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET deleted_at = NULL, is_active = true WHERE id = $1`, userID); err != nil {
		slog.Error("cancel deletion: restore user failed", "error", err, "user_id", userID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	go h.DB.LogAuthEvent(h.shutdownCtx, userID.String(), "account_deletion_cancelled",
		middleware.ClientIP(r), r.UserAgent(), nil)
	slog.Info("account deletion cancelled", "user_id", userID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "deletion_cancelled"})
}
