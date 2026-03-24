package auth

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// Refresh handles POST /auth/refresh — atomic token rotation with family-based reuse detection.
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err != nil || cookie.Value == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "session_expired"})
		return
	}

	tokenHash := HashToken(cookie.Value)

	// Atomic rotation: UPDATE ... SET revoked = true WHERE ... AND NOT revoked RETURNING ...
	userID, familyID, _, err := h.DB.RotateRefreshToken(r.Context(), tokenHash)

	if err != nil {
		// Fall B or C: check if this was a reuse attempt.
		revoked, checkErr := h.DB.IsRevokedToken(r.Context(), tokenHash)
		if checkErr == nil && revoked {
			// Fall B: REUSE DETECTED — revoke entire family.
			slog.Warn("refresh token reuse detected", "token_hash_prefix", tokenHash[:8])
			var reuseFamily string
			queryErr := h.DB.Pool.QueryRow(r.Context(),
				"SELECT family_id FROM refresh_tokens WHERE token_hash = $1", tokenHash,
			).Scan(&reuseFamily)
			if queryErr == nil && reuseFamily != "" {
				_ = h.DB.RevokeTokenFamily(r.Context(), reuseFamily)
			}
		}
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	newHash := HashToken(newToken)
	ip := net.ParseIP(middleware.ClientIP(r))

	if err := h.DB.CreateRefreshToken(r.Context(), userID.String(), familyID, newHash,
		r.UserAgent(), ip, time.Now().Add(30*24*time.Hour)); err != nil {
		slog.Error("create new refresh token failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Issue new JWT.
	jwt, err := IssueJWT(userID.String(), h.secret)
	if err != nil {
		slog.Error("issue JWT on refresh failed", "error", err)
		h.clearRefreshCookie(w)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	// Set new refresh cookie.
	isHTTPS := strings.HasPrefix(h.Config.AppURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    newToken,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   30 * 24 * 3600,
	})

	writeJSON(w, http.StatusOK, map[string]string{"token": jwt})
}

// Logout handles POST /auth/logout — revoke token + clear cookie.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("refresh_token")
	if err == nil && cookie.Value != "" {
		tokenHash := HashToken(cookie.Value)
		// Best-effort revoke — don't care if it fails.
		_, _, _, _ = h.DB.RotateRefreshToken(r.Context(), tokenHash)
	}
	h.clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

// clearRefreshCookie sends a Set-Cookie that deletes the refresh_token cookie.
func (h *Handler) clearRefreshCookie(w http.ResponseWriter) {
	isHTTPS := strings.HasPrefix(h.Config.AppURL, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_token",
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   isHTTPS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
	})
}
