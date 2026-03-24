package auth

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/config"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// Handler handles all auth-related HTTP requests.
type Handler struct {
	DB     *database.DB
	Config *config.Config
	Mailer *Mailer
	RL     *middleware.RateLimiter // for per-email rate limiting (body-parsed)
	secret []byte                 // decoded JWT secret
}

// NewHandler creates an auth handler.
func NewHandler(db *database.DB, cfg *config.Config, mailer *Mailer, rl *middleware.RateLimiter) *Handler {
	secret, _ := hex.DecodeString(cfg.JWTSecret) // already validated at startup
	return &Handler{
		DB:     db,
		Config: cfg,
		Mailer: mailer,
		RL:     rl,
		secret: secret,
	}
}

// Login handles POST /auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}

	// Per-email rate limit (5/15min) — body-parsed, so done in handler not middleware.
	if !h.RL.Allow("login-email:"+req.Email, 5.0/900, 5) {
		http.Error(w, `{"error":"too many requests"}`, http.StatusTooManyRequests)
		return
	}

	user, err := h.DB.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		// User not found — timing equalization.
		_, _ = argon2id.ComparePasswordAndHash(req.Password, GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Progressive delay check.
	if user.LoginDelayUntil != nil && time.Now().Before(*user.LoginDelayUntil) {
		// Still compute hash for timing.
		_, _ = argon2id.ComparePasswordAndHash(req.Password, GetDummyHash())
		retryAfter := int(time.Until(*user.LoginDelayUntil).Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "account_delayed"})
		return
	}

	// Check password.
	if user.PasswordHash == nil {
		// User exists but registered via OAuth (no password set).
		_, _ = argon2id.ComparePasswordAndHash(req.Password, GetDummyHash())
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	match, err := argon2id.ComparePasswordAndHash(req.Password, *user.PasswordHash)
	if err != nil || !match {
		_ = h.DB.UpdateLoginFailure(r.Context(), user.ID)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	// Check is_active.
	if !user.IsActive {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account_inactive"})
		return
	}

	// Check email_verified (skip check if SMTP not configured).
	if h.Config.SMTPHost != "" && !user.EmailVerified {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "email_not_verified"})
		return
	}

	// Success — update login tracking.
	if err := h.DB.UpdateLoginSuccess(r.Context(), user.ID); err != nil {
		slog.Error("update login success failed", "error", err)
	}

	h.issueTokensAndRespond(w, r, user.ID)
}

// issueTokensAndRespond generates JWT + refresh token and writes the response.
// Shared by login, social callback, and token exchange handlers.
func (h *Handler) issueTokensAndRespond(w http.ResponseWriter, r *http.Request, userID string) {
	// Issue JWT.
	jwtToken, err := IssueJWT(userID, h.secret)
	if err != nil {
		slog.Error("issue JWT failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Generate refresh token.
	token, err := GenerateToken()
	if err != nil {
		slog.Error("generate refresh token failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	tokenHash := HashToken(token)
	familyID := GenerateFamily()
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID, familyID, tokenHash,
		r.UserAgent(), ip, time.Now().Add(30*24*time.Hour)); err != nil {
		slog.Error("create refresh token failed", "error", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Set refresh token as HttpOnly cookie.
	isHTTPS := strings.HasPrefix(h.Config.AppURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    token,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 3600, // 30 days
	})

	writeJSON(w, http.StatusOK, map[string]string{"token": jwtToken})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
