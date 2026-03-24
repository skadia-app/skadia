package auth

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// Refresh handles POST /auth/refresh — atomic token rotation with family-based reuse detection.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshCookieName)
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session_expired"})
		return
	}

	tokenHash := HashToken(cookie.Value)

	// Atomic rotation: UPDATE ... SET revoked = true WHERE ... AND NOT revoked RETURNING ...
	userID, familyID, _, err := h.DB.RotateRefreshToken(r.Context(), tokenHash)
	if err != nil {
		h.handleRefreshReuse(r.Context(), tokenHash)
		// Fall B + C: same response (no info leak).
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session_expired"})
		return
	}

	// Fall A: Token was valid, now revoked. Issue new token in same family.
	newToken, err := GenerateToken()
	if err != nil {
		slog.Error("generate refresh token failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	newHash := HashToken(newToken)
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID.String(), familyID, newHash,
		r.UserAgent(), ip, time.Now().Add(RefreshTokenExpiry)); err != nil {
		slog.Error("create new refresh token failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Issue new JWT.
	jwt, err := IssueJWT(userID.String(), h.secret)
	if err != nil {
		slog.Error("issue JWT on refresh failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Set new refresh cookie.
	isHTTPS := strings.HasPrefix(h.Config.AppURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    newToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   RefreshCookieMaxAge,
	})

	writeJSON(w, http.StatusOK, map[string]string{"token": jwt})
}

// Logout handles POST /auth/logout — revoke token + clear cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshCookieName)
	if err == nil && cookie.Value != "" {
		tokenHash := HashToken(cookie.Value)
		// Best-effort revoke — don't care if it fails.
		_, _, _, _ = h.DB.RotateRefreshToken(r.Context(), tokenHash)
	}
	h.clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// handleRefreshReuse checks if a failed rotation was a reuse attempt and revokes the family.
func (h *Handler) handleRefreshReuse(ctx context.Context, tokenHash string) {
	familyID, err := h.DB.GetTokenFamily(ctx, tokenHash)
	if err != nil {
		return // token not found at all — not reuse
	}
	// Token exists but was already revoked → reuse detected.
	slog.Warn("refresh token reuse detected", "token_hash_prefix", tokenHash[:min(8, len(tokenHash))])
	_ = h.DB.RevokeTokenFamily(ctx, familyID)
}

// clearRefreshCookie sends a Set-Cookie that deletes the refresh cookie.
func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	isHTTPS := strings.HasPrefix(h.Config.AppURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
