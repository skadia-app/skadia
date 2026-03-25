package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// sessionEntry is the JSON response shape for user session listings.
type sessionEntry struct {
	FamilyID  string    `json:"family_id"`
	UserAgent *string   `json:"user_agent,omitempty"`
	IPAddress string    `json:"ip_address"`
	IsCurrent bool      `json:"is_current"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ListMySessions handles GET /auth/sessions (authenticated, per-user).
// Returns only active (non-revoked, non-expired) sessions for the current user.
func (h *Handler) ListMySessions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	// Get current family_id from refresh token cookie
	var currentFamilyID string
	if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
		hashedToken := HashToken(cookie.Value)
		h.DB.Pool.QueryRow(r.Context(),
			`SELECT family_id::text FROM refresh_tokens WHERE token_hash = $1 AND revoked = false`,
			hashedToken).Scan(&currentFamilyID)
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT family_id, user_agent, ip_address::text, created_at, expires_at
		 FROM refresh_tokens WHERE user_id = $1 AND NOT revoked AND expires_at > NOW()
		 ORDER BY created_at DESC`, userID)
	if err != nil {
		slog.Error("list my sessions failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	sessions := make([]sessionEntry, 0)
	for rows.Next() {
		var s sessionEntry
		if err := rows.Scan(&s.FamilyID, &s.UserAgent, &s.IPAddress, &s.CreatedAt, &s.ExpiresAt); err != nil {
			slog.Error("list my sessions scan failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		s.IsCurrent = (s.FamilyID == currentFamilyID)
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("list my sessions rows error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// RevokeSession handles POST /auth/sessions/{familyId}/revoke (authenticated).
// A user can only revoke their own sessions.
func (h *Handler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	familyID := chi.URLParam(r, "familyId")
	if familyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "family_id_required"})
		return
	}
	if _, err := uuid.Parse(familyID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_session_id"})
		return
	}

	// Verify the family belongs to this user.
	var ownerID string
	err := h.DB.Pool.QueryRow(r.Context(),
		"SELECT user_id FROM refresh_tokens WHERE family_id = $1 LIMIT 1", familyID).Scan(&ownerID)
	if err != nil || ownerID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session_not_found"})
		return
	}

	if err := h.DB.RevokeTokenFamily(r.Context(), familyID); err != nil {
		slog.Error("revoke session failed", "error", err, "user_id", userID, "family_id", familyID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	go h.DB.LogAuthEvent(h.shutdownCtx, userID, "session_revoke", middleware.ClientIP(r), r.UserAgent(), map[string]string{"family_id": familyID})
	slog.Info("session revoked", "user_id", userID, "family_id", familyID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "session_revoked"})
}
