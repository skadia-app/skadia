package auth

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// Refresh handles POST /auth/refresh (web, cookie-based).
//
// Concurrent refresh behavior: If two tabs refresh simultaneously, the first
// succeeds and the second triggers reuse detection, revoking the entire family.
// This is the correct security behavior — it prevents stolen-token replay.
// The frontend deduplicates refresh calls via refreshInFlight (lib/auth.ts).
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

	// Ensure user is still active (deactivated users must not keep refreshing).
	user, err := h.DB.GetUserByID(r.Context(), userID.String())
	if err != nil || !user.IsActive {
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "account_inactive"})
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
	jwt, err := IssueJWT(user.ID, user.IsAdmin, user.EmailVerified, user.MustChangePassword, h.secret)
	if err != nil {
		slog.Error("issue JWT on refresh failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Set new refresh cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    newToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   RefreshCookieMaxAge,
	})

	writeJSON(w, http.StatusOK, map[string]string{"token": jwt})
}

// Logout handles POST /auth/logout — revoke entire token family + clear cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshCookieName)
	if err == nil && cookie.Value != "" {
		tokenHash := HashToken(cookie.Value)
		// Revoke the token AND its entire family.
		_, familyID, _, rotateErr := h.DB.RotateRefreshToken(r.Context(), tokenHash)
		if rotateErr == nil && familyID != "" {
			_ = h.DB.RevokeTokenFamily(r.Context(), familyID)
		} else {
			// Fallback: if token was already revoked, still try to revoke its family.
			if fallbackFamily, fbErr := h.DB.GetTokenFamily(r.Context(), tokenHash); fbErr == nil && fallbackFamily != "" {
				_ = h.DB.RevokeTokenFamily(r.Context(), fallbackFamily)
			}
		}
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
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   h.isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
