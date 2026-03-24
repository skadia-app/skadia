package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
)

// ForgotPassword handles POST /auth/forgot-password.
// Responds immediately (200) — background goroutine handles user lookup + email.
// This prevents timing-based user enumeration.
func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email required"})
		return
	}

	// Normalize email (I3).
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Email format validation (I4).
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}

	// Per-email rate limit: 3 requests per hour (I7).
	// Always return 200 to prevent enumeration.
	if !h.RL.Allow("forgot-email:"+req.Email, 3.0/3600, 3) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	// Respond immediately — no timing leak.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	// Background: lookup user, generate token, send email.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		user, err := h.DB.GetUserByEmail(ctx, req.Email)
		if err != nil {
			return // user not found — silently ignore
		}

		if !h.Mailer.IsConfigured() {
			slog.Warn("forgot-password: SMTP not configured, cannot send reset email")
			return
		}

		token, err := GenerateToken()
		if err != nil {
			slog.Error("forgot-password: generate token failed", "error", err)
			return
		}

		tokenHash := HashToken(token)
		if err := h.DB.CreateEmailToken(ctx, uuid.MustParse(user.ID), tokenHash, "reset",
			time.Now().Add(1*time.Hour)); err != nil {
			slog.Error("forgot-password: create email token failed", "error", err)
			return
		}

		if user.Email != nil {
			h.Mailer.SendResetEmail(*user.Email, user.Username, token) // raw token, not hash
		}
	}()
}

// ResetPassword handles POST /auth/reset-password.
func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token and new_password required"})
		return
	}
	if len(req.NewPassword) < 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 10 characters"})
		return
	}
	if len(req.NewPassword) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
		return
	}

	// HIBP check — always, not just open mode (M12).
	compromised, err := CheckHIBP(req.NewPassword)
	if err != nil {
		slog.Warn("HIBP check failed during password reset", "error", err)
	} else if compromised {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
		return
	}

	// Validate + consume token (single-use, atomic).
	tokenHash := HashToken(req.Token)
	userID, err := h.DB.ConsumeEmailToken(r.Context(), tokenHash, "reset")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	// Hash new password with Argon2id.
	hash, err := argon2id.CreateHash(req.NewPassword, &Argon2Params)
	if err != nil {
		slog.Error("hash password failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Update password (also sets password_changed_at).
	if err := h.DB.UpdatePassword(r.Context(), userID.String(), hash); err != nil {
		slog.Error("update password failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Invalidate ALL refresh tokens for this user.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID.String()); err != nil {
		slog.Error("revoke refresh tokens after password reset failed", "error", err)
	}

	// Clear login delays.
	if err := h.DB.ClearLoginDelays(r.Context(), userID.String()); err != nil {
		slog.Error("clear login delays after password reset failed", "error", err)
	}

	slog.Info("password reset", "user_id", userID.String())

	writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})
}
