package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ChangePassword handles POST /auth/change-password (authenticated).
// Requires valid JWT. The user provides current password + new password.
// OAuth-Only users (no password set + must_change_password) can omit current_password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// NewPassword is always required.
	if req.NewPassword == "" {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "new_password_required"})
		return
	}
	if len(req.NewPassword) < 10 {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
		return
	}
	if len(req.NewPassword) > 1000 {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
		return
	}

	userID := middleware.GetUserID(r.Context())

	// Load user to determine whether current_password is required.
	//
	// current_password is only optional for OAuth-only users who have never set a
	// password (PasswordHash == nil && MustChangePassword). For every other case
	// (normal users, admin-reset users with an existing password) it's required.
	//
	// The JWT only exposes must_change_password, not whether a password hash
	// exists, so we must consult the DB. If the DB is not wired (unit tests with
	// the testHandler stub), we fall through to the safe default
	// (requireCurrentPassword = true) — this matches the default contract and
	// prevents SIGSEGV when the test doesn't exercise the DB path. Any DB write
	// further down will still bail out with 503 because h.DB is nil.
	var user *model.User
	if h.DB != nil && h.DB.Pool != nil {
		loaded, err := h.DB.GetUserByID(r.Context(), userID)
		if err != nil {
			httputil.WriteJSONNoCache(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		user = loaded
	}

	// Default: current_password is required. Only loosen when we positively know
	// the user is OAuth-only with no password set.
	requireCurrentPassword := true
	if user != nil && user.PasswordHash == nil && user.MustChangePassword {
		requireCurrentPassword = false
	}

	if requireCurrentPassword {
		if req.CurrentPassword == "" {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "current_and_new_password_required"})
			return
		}
		if len(req.CurrentPassword) > 1000 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
			return
		}
		// Hash comparison requires the loaded user. If DB was nil (test stub),
		// there is nothing to compare against — fail closed with 503 instead of
		// accepting any password.
		if user == nil {
			httputil.WriteJSONNoCache(w, http.StatusServiceUnavailable, map[string]string{"error": "service_unavailable"})
			return
		}
		if user.PasswordHash == nil {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "no_password_set"})
			return
		}
		// Verify current password.
		match, err := argon2id.ComparePasswordAndHash(req.CurrentPassword, *user.PasswordHash)
		if err != nil || !match {
			httputil.WriteJSONNoCache(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
			return
		}
	}

	// HIBP check.
	compromised, err := CheckHIBP(req.NewPassword)
	if err != nil {
		slog.Warn("HIBP check failed during password change", "error", err)
	} else if compromised {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
		return
	}

	// Hash new password.
	hash, err := argon2id.CreateHash(req.NewPassword, GetArgon2Params())
	if err != nil {
		slog.Error("hash password failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Update password.
	if err := h.DB.UpdatePassword(r.Context(), userID, hash); err != nil {
		slog.Error("update password failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Clear must_change_password and pending_email flags.
	if err := h.DB.ClearMustChangePassword(r.Context(), userID); err != nil {
		slog.Error("clear must_change_password after password change failed", "error", err)
	}
	if err := h.DB.ClearPendingEmail(r.Context(), userID); err != nil {
		slog.Error("clear pending_email after password change failed", "error", err)
	}

	// Revoke ALL refresh tokens (force re-login everywhere).
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Error("revoke tokens after password change failed", "error", err)
	}

	slog.Info("auth.password_change", "user_id", userID)
	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "password_change", middleware.ClientIP(r), r.UserAgent(), nil)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "password_changed"})
}
