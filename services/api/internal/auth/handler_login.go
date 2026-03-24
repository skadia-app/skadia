package auth

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const maxAuthBodySize = 4096 // 4 KB auth payload limit

// limitBody restricts the request body to maxAuthBodySize.
func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)
}

// Handler handles all auth-related HTTP requests.
type Handler struct {
	DB      *database.DB
	Config  *config.Config
	Mailer  *Mailer
	RL      *middleware.RateLimiter // for per-email rate limiting (body-parsed)
	secret  []byte                 // decoded JWT secret
	isHTTPS bool                   // derived from Config.AppURL at init
}

// NewHandler creates an auth handler.
func NewHandler(db *database.DB, cfg *config.Config, mailer *Mailer, rl *middleware.RateLimiter) *Handler {
	secret, _ := hex.DecodeString(cfg.JWTSecret) // already validated at startup
	return &Handler{
		DB:      db,
		Config:  cfg,
		Mailer:  mailer,
		RL:      rl,
		secret:  secret,
		isHTTPS: strings.HasPrefix(cfg.AppURL, "https://"),
	}
}

// Login handles POST /auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	limitBody(w, r)

	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email_and_password_required"})
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_email"})
		return
	}

	user := h.authenticateUser(w, r, req.Email, req.Password)
	if user == nil {
		return
	}

	h.issueTokensAndRespond(w, r, user.ID)
}

// authenticateUser validates email/password credentials with timing equalization.
// Returns the authenticated user or writes an error response and returns nil.
func (h *Handler) authenticateUser(w http.ResponseWriter, r *http.Request, email, password string) *model.User {
	// Normalize email.
	email = strings.ToLower(strings.TrimSpace(email))

	// Password max length guard (prevent argon2id DoS).
	if len(password) > 1000 {
		_, _ = argon2id.ComparePasswordAndHash(password[:100], GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 1. Per-email rate limit (5/15min).
	if !h.RL.Allow("login-email:"+email, 5.0/900, 5) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too_many_requests"})
		return nil
	}

	// 2. Lookup user.
	user, err := h.DB.GetUserByEmail(r.Context(), email)
	if err != nil {
		// User not found — timing equalization.
		_, _ = argon2id.ComparePasswordAndHash(password, GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 3. Check is_active BEFORE password verify (I21 fix — no info leak with timing equalization).
	if !user.IsActive {
		_, _ = argon2id.ComparePasswordAndHash(password, GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 4. Progressive delay check.
	if user.LoginDelayUntil != nil && time.Now().Before(*user.LoginDelayUntil) {
		_, _ = argon2id.ComparePasswordAndHash(password, GetDummyHash())
		retryAfter := int(time.Until(*user.LoginDelayUntil).Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "account_delayed"})
		return nil
	}

	// 5. No password set (OAuth-only user).
	if user.PasswordHash == nil {
		_, _ = argon2id.ComparePasswordAndHash(password, GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 6. Verify password.
	match, err := argon2id.ComparePasswordAndHash(password, *user.PasswordHash)
	if err != nil || !match {
		_ = h.DB.UpdateLoginFailure(r.Context(), user.ID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 7. Email verified check (skip if no SMTP).
	if h.Config.SMTPHost != "" && !user.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "email_not_verified"})
		return nil
	}

	// 8. Success — clear delays.
	if err := h.DB.UpdateLoginSuccess(r.Context(), user.ID); err != nil {
		slog.Error("update login success failed", "error", err)
	}

	slog.Info("login success", "user_id", user.ID, "method", "email")

	return user
}

// issueTokensAndRespond generates JWT + refresh token and writes the response.
// Shared by login, social callback, and token exchange handlers.
func (h *Handler) issueTokensAndRespond(w http.ResponseWriter, r *http.Request, userID string) {
	// Issue JWT.
	jwtToken, err := IssueJWT(userID, h.secret)
	if err != nil {
		slog.Error("issue JWT failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Generate refresh token.
	token, err := GenerateToken()
	if err != nil {
		slog.Error("generate refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	tokenHash := HashToken(token)
	familyID := GenerateFamily()
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID, familyID, tokenHash,
		r.UserAgent(), ip, time.Now().Add(RefreshTokenExpiry)); err != nil {
		slog.Error("create refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Set refresh token as HttpOnly cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   RefreshCookieMaxAge,
	})

	writeJSON(w, http.StatusOK, map[string]string{"token": jwtToken})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "error", err)
	}
}
