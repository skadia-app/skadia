package auth

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// Register handles POST /auth/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	// Validate inputs.
	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email, username, and password required"})
		return
	}
	if len(req.Password) < 10 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password must be at least 10 characters"})
		return
	}

	// Registration mode check.
	if h.Config.RegistrationMode == "invite" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "registration disabled"})
		return
	}

	// HIBP check (mandatory for open mode).
	if h.Config.RegistrationMode == "open" {
		compromised, err := CheckHIBP(req.Password)
		if err != nil {
			slog.Warn("HIBP check failed", "error", err)
			// Don't block registration on HIBP failure.
		} else if compromised {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
			return
		}
	}

	// Check email collision.
	exists, err := h.DB.CheckEmailCollision(r.Context(), req.Email)
	if err != nil {
		slog.Error("check email collision failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if exists {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "email_exists"})
		return
	}

	// Hash password with argon2id.
	hash, err := argon2id.CreateHash(req.Password, &Argon2Params)
	if err != nil {
		slog.Error("hash password failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Create user.
	ip := net.ParseIP(middleware.ClientIP(r))
	isActive := h.Config.RegistrationMode != "approval"
	user, err := h.DB.CreateUserWithPassword(r.Context(), req.Email, req.Username, hash, "email", ip, isActive)
	if err != nil {
		slog.Error("create user failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
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
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "verification_email_sent"})
}

// VerifyEmail handles POST /auth/verify-email.
func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "verified"})
}
