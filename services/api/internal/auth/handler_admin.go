package auth

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// adminUserEntry is the JSON response shape for admin user listings.
type adminUserEntry struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              *string    `json:"email,omitempty"`
	IsAdmin            bool       `json:"is_admin"`
	IsActive           bool       `json:"is_active"`
	EmailVerified      bool       `json:"email_verified"`
	RegistrationSource *string    `json:"registration_source,omitempty"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

// AdminListUsers handles GET /admin/users — list all users with auth status.
func (h *Handler) AdminListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, username, email, is_admin, is_active, email_verified,
		        registration_source, last_login_at, created_at
		 FROM users ORDER BY created_at DESC`)
	if err != nil {
		slog.Error("admin list users failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	var users []adminUserEntry
	for rows.Next() {
		var u adminUserEntry
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.IsAdmin, &u.IsActive,
			&u.EmailVerified, &u.RegistrationSource, &u.LastLoginAt, &u.CreatedAt); err != nil {
			slog.Error("admin list users scan failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin list users rows error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// AdminActivateUser handles POST /admin/users/{id}/activate.
func (h *Handler) AdminActivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id_required"})
		return
	}

	if err := h.DB.SetUserActive(r.Context(), userID, true); err != nil {
		slog.Error("admin activate user failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	slog.Info("admin activated user", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// AdminDeactivateUser handles POST /admin/users/{id}/deactivate.
func (h *Handler) AdminDeactivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id_required"})
		return
	}

	if err := h.DB.SetUserActive(r.Context(), userID, false); err != nil {
		slog.Error("admin deactivate user failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Revoke all tokens to force immediate logout.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Warn("admin deactivate: revoke tokens failed", "error", err, "user_id", userID)
	}

	slog.Info("admin deactivated user", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "deactivated"})
}

// adminSessionEntry is the JSON response shape for admin session listings.
type adminSessionEntry struct {
	FamilyID  string     `json:"family_id"`
	UserAgent *string    `json:"user_agent,omitempty"`
	IPAddress *string    `json:"ip_address,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	Revoked   bool       `json:"revoked"`
}

// AdminListUserSessions handles GET /admin/users/{id}/sessions.
func (h *Handler) AdminListUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id_required"})
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT family_id, user_agent, ip_address::text, created_at, expires_at, revoked
		 FROM refresh_tokens WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT 50`, userID)
	if err != nil {
		slog.Error("admin list user sessions failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	var sessions []adminSessionEntry
	for rows.Next() {
		var s adminSessionEntry
		if err := rows.Scan(&s.FamilyID, &s.UserAgent, &s.IPAddress,
			&s.CreatedAt, &s.ExpiresAt, &s.Revoked); err != nil {
			slog.Error("admin list user sessions scan failed", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin list user sessions rows error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// AdminRevokeUserSessions handles POST /admin/users/{id}/revoke-sessions.
func (h *Handler) AdminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id_required"})
		return
	}

	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Error("admin revoke user sessions failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	slog.Info("admin revoked all sessions", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "sessions_revoked"})
}
