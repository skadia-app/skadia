package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// ChangePassword handles POST /auth/change-password (authenticated).
// Requires valid JWT. The user provides current password + new password.
func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	if req.CurrentPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "current_and_new_password_required"})
		return
	}
	if len(req.NewPassword) < 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
		return
	}
	if len(req.NewPassword) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
		return
	}

	userID := middleware.GetUserID(r.Context())

	// Load user to verify current password.
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if user.PasswordHash == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_password_set"})
		return
	}

	// Verify current password.
	match, err := argon2id.ComparePasswordAndHash(req.CurrentPassword, *user.PasswordHash)
	if err != nil || !match {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "wrong_password"})
		return
	}

	// HIBP check.
	compromised, err := CheckHIBP(req.NewPassword)
	if err != nil {
		slog.Warn("HIBP check failed during password change", "error", err)
	} else if compromised {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
		return
	}

	// Hash new password.
	hash, err := argon2id.CreateHash(req.NewPassword, &Argon2Params)
	if err != nil {
		slog.Error("hash password failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Update password.
	if err := h.DB.UpdatePassword(r.Context(), userID, hash); err != nil {
		slog.Error("update password failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Revoke ALL refresh tokens (force re-login everywhere).
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Error("revoke tokens after password change failed", "error", err)
	}

	slog.Info("password changed", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}
