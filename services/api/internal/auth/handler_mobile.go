package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// MobileLogin handles POST /auth/mobile/login.
// Same validation, timing equalization and progressive delays as Login,
// but returns tokens in the response body (no cookie).
func (h *Handler) MobileLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email and password required"})
		return
	}

	// Per-email rate limit (5/15min).
	if !h.RL.Allow("login-email:"+req.Email, 5.0/900, 5) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
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
		_, _ = argon2id.ComparePasswordAndHash(req.Password, GetDummyHash())
		retryAfter := int(time.Until(*user.LoginDelayUntil).Seconds()) + 1
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "account_delayed"})
		return
	}

	// Check password.
	if user.PasswordHash == nil {
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

	h.issueMobileTokens(w, r, user.ID)
}

// issueMobileTokens generates JWT + refresh token and writes both in the response body.
// Used by mobile login and PKCE exchange handlers.
func (h *Handler) issueMobileTokens(w http.ResponseWriter, r *http.Request, userID string) {
	jwt, err := IssueJWT(userID, h.secret)
	if err != nil {
		slog.Error("issue JWT failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	token, err := GenerateToken()
	if err != nil {
		slog.Error("generate refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	tokenHash := HashToken(token)
	familyID := GenerateFamily()
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID, familyID, tokenHash,
		r.UserAgent(), ip, time.Now().Add(30*24*time.Hour)); err != nil {
		slog.Error("create refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":         jwt,
		"refresh_token": token,
	})
}

// MobileRefresh handles POST /auth/mobile/refresh — atomic token rotation with body-based tokens.
func (h *Handler) MobileRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token required"})
		return
	}

	tokenHash := HashToken(req.RefreshToken)

	userID, familyID, _, err := h.DB.RotateRefreshToken(r.Context(), tokenHash)
	if err != nil {
		// Check for reuse attempt.
		revoked, checkErr := h.DB.IsRevokedToken(r.Context(), tokenHash)
		if checkErr == nil && revoked {
			slog.Warn("mobile refresh token reuse detected", "token_hash_prefix", tokenHash[:8])
			var reuseFamily string
			_ = h.DB.Pool.QueryRow(r.Context(),
				"SELECT family_id FROM refresh_tokens WHERE token_hash = $1", tokenHash,
			).Scan(&reuseFamily)
			if reuseFamily != "" {
				_ = h.DB.RevokeTokenFamily(r.Context(), reuseFamily)
			}
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session_expired"})
		return
	}

	// Issue new tokens in body.
	newToken, err := GenerateToken()
	if err != nil {
		slog.Error("generate refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	newHash := HashToken(newToken)
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID.String(), familyID, newHash,
		r.UserAgent(), ip, time.Now().Add(30*24*time.Hour)); err != nil {
		slog.Error("create new refresh token failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	jwt, err := IssueJWT(userID.String(), h.secret)
	if err != nil {
		slog.Error("issue JWT on mobile refresh failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":         jwt,
		"refresh_token": newToken,
	})
}

// MobileExchange handles POST /auth/mobile/exchange — PKCE auth code exchange.
// Consumes a short-lived auth code with S256 code_verifier validation
// and returns tokens in the body.
func (h *Handler) MobileExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.CodeVerifier == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code and code_verifier required"})
		return
	}

	codeHash := HashToken(req.Code)
	userID, codeChallenge, _, err := h.DB.ConsumeAuthCode(r.Context(), codeHash)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or expired code"})
		return
	}

	// Verify PKCE: S256(code_verifier) must match stored code_challenge.
	if codeChallenge == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code not issued for PKCE"})
		return
	}
	computedChallenge := computeS256Challenge(req.CodeVerifier)
	if computedChallenge != codeChallenge {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid code_verifier"})
		return
	}

	h.issueMobileTokens(w, r, userID.String())
}

// computeS256Challenge computes the S256 PKCE code challenge.
// S256: BASE64URL(SHA256(code_verifier))
func computeS256Challenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// MobileLogout handles POST /auth/mobile/logout — best-effort token revocation.
func (h *Handler) MobileLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token required"})
		return
	}

	tokenHash := HashToken(req.RefreshToken)
	_, _, _, _ = h.DB.RotateRefreshToken(r.Context(), tokenHash) // best-effort revoke

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}
