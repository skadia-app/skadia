package auth

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
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
	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	// Get total count for pagination metadata.
	var total int
	if err := h.DB.Pool.QueryRow(r.Context(), `SELECT count(*) FROM users`).Scan(&total); err != nil {
		slog.Error("admin list users count failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, username, email, is_admin, is_active, email_verified,
		        registration_source, last_login_at, created_at
		 FROM users ORDER BY created_at DESC
		 LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		slog.Error("admin list users failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	users := make([]adminUserEntry, 0)
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

	writeJSON(w, http.StatusOK, map[string]any{
		"users":  users,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// AdminActivateUser handles POST /admin/users/{id}/activate.
func (h *Handler) AdminActivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	if err := h.DB.SetUserActive(r.Context(), userID, true); err != nil {
		slog.Error("admin activate user failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_activate_user", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin activated user", "user_id", userID, "admin_id", adminID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "activated"})
}

// AdminDeactivateUser handles POST /admin/users/{id}/deactivate.
func (h *Handler) AdminDeactivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	if userID == adminID {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot_deactivate_self"})
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

	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_deactivate_user", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin deactivated user", "user_id", userID, "admin_id", adminID)
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
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT family_id, user_agent, ip_address::text, created_at, expires_at, revoked
		 FROM refresh_tokens WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, userID, limit, offset)
	if err != nil {
		slog.Error("admin list user sessions failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	sessions := make([]adminSessionEntry, 0)
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
	if _, err := uuid.Parse(userID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Error("admin revoke user sessions failed", "error", err, "user_id", userID)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_revoke_sessions", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin revoked all sessions", "user_id", userID, "admin_id", adminID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "sessions_revoked"})
}
