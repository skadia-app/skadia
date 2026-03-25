package auth

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// AdminCreateUserRequest is the request body for POST /admin/users.
type AdminCreateUserRequest struct {
	Email    string  `json:"email"`
	Username string  `json:"username"`
	Password *string `json:"password,omitempty"`
}

// AdminCreateUser handles POST /admin/users — create a user with a temporary
// password (must_change_password=true) or invite via email (no password, sends
// a reset-password link).
func (h *Handler) AdminCreateUser(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req AdminCreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Normalize.
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	// Validate email format.
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}

	// Validate username (shared rules).
	if errKey := ValidateUsername(req.Username); errKey != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errKey})
		return
	}

	// Email collision — 200+status, not 409 (project convention).
	exists, err := h.DB.CheckEmailCollision(r.Context(), req.Email)
	if err != nil {
		slog.Error("admin create user: check email collision failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if exists {
		writeJSON(w, http.StatusOK, map[string]string{"status": "email_exists"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	ip := middleware.ClientIP(r)

	if req.Password != nil {
		// Direct creation with temporary password.
		if len(*req.Password) < 10 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
			return
		}
		if len(*req.Password) > 1000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
			return
		}

		// HIBP check.
		compromised, hibpErr := CheckHIBP(*req.Password)
		if hibpErr != nil {
			slog.Warn("admin create user: HIBP check failed", "error", hibpErr)
			// Don't block on HIBP failure.
		} else if compromised {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
			return
		}

		hash, hashErr := argon2id.CreateHash(*req.Password, GetArgon2Params())
		if hashErr != nil {
			slog.Error("admin create user: hash password failed", "error", hashErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}

		user, createErr := h.DB.CreateUserWithPassword(r.Context(),
			req.Email, req.Username, hash, "admin", net.ParseIP(ip), true)
		if createErr != nil {
			slog.Error("admin create user: create with password failed", "error", createErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "admin_create_failed"})
			return
		}

		// Set must_change_password and email_verified (admin-created accounts are pre-verified).
		if _, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET must_change_password = true, email_verified = true WHERE id = $1`,
			user.ID); err != nil {
			slog.Error("admin create user: set must_change_password failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}

		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_create_user", ip, r.UserAgent(),
			map[string]string{"target_email": req.Email, "method": "password"})
		slog.Info("admin created user with password", "admin_id", adminID, "target_email", req.Email)
		writeJSON(w, http.StatusCreated, map[string]any{"user": user, "must_change_password": true})
	} else {
		// Invite via email — requires SMTP.
		if !h.Mailer.IsConfigured() {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error":   "smtp_required_for_invite",
				"message": "Ohne SMTP muss ein temporaeres Passwort gesetzt werden",
			})
			return
		}

		user, createErr := h.DB.CreateUserWithoutPassword(r.Context(),
			req.Email, req.Username, "admin_invite", net.ParseIP(ip), true)
		if createErr != nil {
			slog.Error("admin create user: create without password failed", "error", createErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "admin_create_failed"})
			return
		}

		token, tokenErr := GenerateToken()
		if tokenErr != nil {
			slog.Error("admin create user: generate invite token failed", "error", tokenErr)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}

		if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(user.ID),
			HashToken(token), "reset", time.Now().Add(24*time.Hour)); err != nil {
			slog.Error("admin create user: create email token failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}

		go h.Mailer.SendInviteEmail(req.Email, req.Username, token)

		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_create_user", ip, r.UserAgent(),
			map[string]string{"target_email": req.Email, "method": "invite"})
		slog.Info("admin invited user via email", "admin_id", adminID, "target_email", req.Email)
		writeJSON(w, http.StatusCreated, map[string]any{"user": user, "invite_sent": true})
	}
}
