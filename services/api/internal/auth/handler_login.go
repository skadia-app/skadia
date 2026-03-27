package auth

import (
	"context"
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
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

const maxAuthBodySize = 4096 // 4 KB auth payload limit

// limitBody restricts the request body to maxAuthBodySize.
func limitBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthBodySize)
}

// Handler handles all auth-related HTTP requests.
type Handler struct {
	DB          *database.DB
	Config      *config.Config
	Mailer      *Mailer
	RL          *middleware.RateLimiter // for per-email rate limiting (body-parsed)
	ShadowFS    shadowfs.ShadowFS       // unified storage (nil = legacy ShadowFS only)
	secret      []byte                  // decoded JWT secret
	isHTTPS     bool                    // derived from Config.AppURL at init
	shutdownCtx context.Context         // server lifecycle context for background goroutines
}

// NewHandler creates an auth handler. shutdownCtx should be the server's
// lifecycle context so background goroutines (e.g. ForgotPassword email)
// are cancelled on graceful shutdown.
func NewHandler(db *database.DB, cfg *config.Config, mailer *Mailer, rl *middleware.RateLimiter, shutdownCtx context.Context) *Handler {
	return &Handler{
		DB:          db,
		Config:      cfg,
		Mailer:      mailer,
		RL:          rl,
		secret:      cfg.JWTSecretBytes,
		isHTTPS:     strings.HasPrefix(cfg.AppURL, "https://"),
		shutdownCtx: shutdownCtx,
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

	h.issueTokensAndRespond(w, r, user)
}

// authenticateUser validates email/password credentials with timing equalization.
// Every early-return path calls argon2id.ComparePasswordAndHash(dummy) to ensure
// constant response time regardless of which guard triggered (user-not-found,
// inactive, delayed, no-password). This prevents user enumeration via timing.
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
		go h.DB.LogAuthEvent(h.shutdownCtx, user.ID, "login_failure", middleware.ClientIP(r), r.UserAgent(), nil)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_credentials"})
		return nil
	}

	// 7. Success — clear delays.
	if err := h.DB.UpdateLoginSuccess(r.Context(), user.ID); err != nil {
		slog.Error("update login success failed", "error", err)
	}

	slog.Info("auth.login_success", "user_id", user.ID, "method", "email")
	go h.DB.LogAuthEvent(h.shutdownCtx, user.ID, "login_success", middleware.ClientIP(r), r.UserAgent(), map[string]string{"method": "email"})

	return user
}

// issueTokensAndRespond generates JWT + refresh token and writes the response.
// Shared by login, social callback, and token exchange handlers.
func (h *Handler) issueTokensAndRespond(w http.ResponseWriter, r *http.Request, user *model.User) {
	// Issue JWT.
	jwtToken, err := IssueJWT(user.ID, user.IsAdmin, user.EmailVerified, user.MustChangePassword, h.secret)
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
	familyID, err := GenerateFamily()
	if err != nil {
		slog.Error("generate family ID failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), user.ID, familyID, tokenHash,
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

	resp := map[string]any{"token": jwtToken}
	if user.MustChangePassword {
		resp["must_change_password"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

// writeJSON writes a JSON response with the given status code.
// All auth responses include Cache-Control: no-store to prevent caching
// of tokens, credentials, and security-sensitive data.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("writeJSON encode failed", "error", err)
	}
}
