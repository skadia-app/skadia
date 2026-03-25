package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// ChangeEmail handles POST /auth/change-email (authenticated).
// Initiates the email change flow by sending a verification link to the new address.
// Timing-equalized: DB operations happen in both paths to prevent email enumeration.
func (h *Handler) ChangeEmail(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	userID := middleware.GetUserID(r.Context())
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req struct {
		NewEmail string `json:"new_email"`
		Password string `json:"password,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	req.NewEmail = strings.ToLower(strings.TrimSpace(req.NewEmail))

	// Password check only if user has a password (not OAuth-only).
	if user.PasswordHash != nil {
		if req.Password == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_required"})
			return
		}
		match, _ := argon2id.ComparePasswordAndHash(req.Password, *user.PasswordHash)
		if !match {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "wrong_password"})
			return
		}
	}

	if _, err := mail.ParseAddress(req.NewEmail); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}

	// Timing-equalization: do DB ops in BOTH paths to prevent enumeration.
	exists, _ := h.DB.CheckEmailCollision(r.Context(), req.NewEmail)

	_, err = h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET pending_email = $1 WHERE id = $2`, req.NewEmail, userID)
	if err != nil {
		slog.Error("change-email: set pending_email failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	token, err := GenerateToken()
	if err != nil {
		slog.Error("change-email: generate token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(userID),
		HashToken(token), "email_change", time.Now().Add(24*time.Hour)); err != nil {
		slog.Error("change-email: create email token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if !exists {
		// Send verification to the NEW email address.
		go h.Mailer.SendChangeEmailVerification(req.NewEmail, user.Username, token)
	}
	// Silent drop if email collision — timing equalized, token expires in 24h.

	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "email_change_requested",
		middleware.ClientIP(r), r.UserAgent(), nil)

	writeJSON(w, http.StatusOK, map[string]string{"status": "verification_email_sent"})
}

// ConfirmEmailChange handles POST /auth/confirm-email-change (public, token-based).
// Consumes the email_change token and updates the user's email address.
func (h *Handler) ConfirmEmailChange(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Validate + consume token (single-use, atomic).
	userID, err := h.DB.ConsumeEmailToken(r.Context(), HashToken(req.Token), "email_change")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_or_expired_token"})
		return
	}

	var oldEmail, newEmail string
	err = h.DB.Pool.QueryRow(r.Context(),
		`WITH old AS (SELECT email FROM users WHERE id = $1)
		 UPDATE users SET email = pending_email, pending_email = NULL, email_verified = true
		 WHERE id = $1
		 RETURNING (SELECT email FROM old) AS old_email, email AS new_email`,
		userID).Scan(&oldEmail, &newEmail)
	if err != nil {
		slog.Error("confirm-email-change: update email failed", "user_id", userID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	go h.DB.LogAuthEvent(h.shutdownCtx, userID.String(), "email_changed",
		middleware.ClientIP(r), r.UserAgent(),
		map[string]string{"old_email": oldEmail, "new_email": newEmail})

	writeJSON(w, http.StatusOK, map[string]string{"status": "email_changed"})
}
