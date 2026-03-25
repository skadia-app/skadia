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

// Register handles POST /auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Registration mode check — before validation to avoid external calls (HIBP) on rejection.
	if h.Config.RegistrationMode == "invite" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "registration_disabled"})
		return
	}

	// Normalize email (I3).
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	// Validate inputs.
	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, username, and password required"})
		return
	}

	// Email format validation (I4).
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}

	// Username validation (M10) — shared rules with settings.go.
	if errKey := ValidateUsername(req.Username); errKey != "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": errKey})
		return
	}

	if len(req.Password) < 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
		return
	}
	if len(req.Password) > 1000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
		return
	}

	// HIBP check — always, not just open mode (M12).
	compromised, err := CheckHIBP(req.Password)
	if err != nil {
		slog.Warn("HIBP check failed", "error", err)
		// Don't block registration on HIBP failure.
	} else if compromised {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
		return
	}

	// Check email collision — return 201 regardless to prevent enumeration (M7).
	exists, err := h.DB.CheckEmailCollision(r.Context(), req.Email)
	if err != nil {
		slog.Error("check email collision failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if exists {
		// Silent — don't reveal email existence (M7).
		writeJSON(w, http.StatusCreated, map[string]string{"status": "verification_email_sent"})
		return
	}

	// Hash password with argon2id.
	hash, err := argon2id.CreateHash(req.Password, GetArgon2Params())
	if err != nil {
		slog.Error("hash password failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Create user.
	ip := net.ParseIP(middleware.ClientIP(r))
	isActive := h.Config.RegistrationMode != "approval"
	user, err := h.DB.CreateUserWithPassword(r.Context(), req.Email, req.Username, hash, "email", ip, isActive)
	if err != nil {
		slog.Error("create user failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	slog.Info("auth.register", "user_id", user.ID, "mode", h.Config.RegistrationMode)
	go h.DB.LogAuthEvent(h.shutdownCtx, user.ID, "register", middleware.ClientIP(r), r.UserAgent(), map[string]string{"mode": h.Config.RegistrationMode})

	// In approval mode, notify admins about the pending registration.
	if h.Config.RegistrationMode == "approval" {
		if h.Mailer.IsConfigured() {
			go h.notifyAdminsNewRegistration(user)
		}
	}

	// Generate verification token.
	token, err := GenerateToken()
	if err != nil {
		slog.Error("generate verification token failed", "error", err)
		// User was created but token generation failed — not fatal.
		writeJSON(w, http.StatusCreated, map[string]string{"status": "verification_email_sent"})
		return
	}

	tokenHash := HashToken(token)
	if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(user.ID), tokenHash, "verification",
		time.Now().Add(24*time.Hour)); err != nil {
		slog.Error("create verification token failed", "error", err)
	}

	// Send verification email (async) — send the RAW token, not the hash.
	if h.Mailer.IsConfigured() {
		h.Mailer.SendVerificationEmail(req.Email, req.Username, token)
		writeJSON(w, http.StatusCreated, map[string]string{"status": "verification_email_sent"})
	} else {
		writeJSON(w, http.StatusCreated, map[string]string{"status": "created"})
	}
}

// ResendVerification handles POST /auth/resend-verification.
// Requires authentication (user must already be logged in).
func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	user, err := h.DB.GetUserByID(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if user.EmailVerified {
		writeJSON(w, http.StatusOK, map[string]string{"status": "already_verified"})
		return
	}
	if !h.Mailer.IsConfigured() {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "smtp_not_configured"})
		return
	}
	token, err := GenerateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(userID), HashToken(token), "verification",
		time.Now().Add(24*time.Hour)); err != nil {
		slog.Error("create email token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if user.Email == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no_email_configured"})
		return
	}
	go h.Mailer.SendVerificationEmail(*user.Email, user.Username, token)
	writeJSON(w, http.StatusOK, map[string]string{"status": "verification_email_sent"})
}

// VerifyEmail handles POST /auth/verify-email.
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	tokenHash := HashToken(req.Token)
	userID, err := h.DB.ConsumeEmailToken(r.Context(), tokenHash, "verification")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired token"})
		return
	}

	if err := h.DB.SetEmailVerified(r.Context(), userID.String()); err != nil {
		slog.Error("set email verified failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}
