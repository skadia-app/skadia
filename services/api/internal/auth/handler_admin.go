package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// adminUserEntry is the JSON response shape for admin user listings.
type adminUserEntry struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	DisplayName        *string    `json:"display_name,omitempty"`
	Email              *string    `json:"email,omitempty"`
	IsAdmin            bool       `json:"is_admin"`
	IsActive           bool       `json:"is_active"`
	EmailVerified      bool       `json:"email_verified"`
	RegistrationSource *string    `json:"registration_source,omitempty"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
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

	search := r.URL.Query().Get("search")
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}

	// Escape ILIKE special characters to prevent pattern injection.
	escapedSearch := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(search)

	const whereClause = `($1 = '' OR username ILIKE '%' || $1 || '%'
       OR email ILIKE '%' || $1 || '%'
       OR display_name ILIKE '%' || $1 || '%')
  AND (CASE $2
       WHEN 'active'   THEN is_active = true  AND deleted_at IS NULL
       WHEN 'inactive' THEN is_active = false AND deleted_at IS NULL
       WHEN 'deleted'  THEN deleted_at IS NOT NULL
       WHEN 'all'      THEN true
       ELSE is_active = true AND deleted_at IS NULL END)`

	// Get total count for pagination metadata.
	var total int
	countQuery := `SELECT count(*) FROM users WHERE ` + whereClause
	if err := h.DB.Pool.QueryRow(r.Context(), countQuery, escapedSearch, status).Scan(&total); err != nil {
		slog.Error("admin list users count failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	query := `SELECT id, username, display_name, email, is_admin, is_active, email_verified,
    registration_source, last_login_at, created_at, deleted_at
FROM users
WHERE ` + whereClause + `
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`

	rows, err := h.DB.Pool.Query(r.Context(), query, escapedSearch, status, limit, offset)
	if err != nil {
		slog.Error("admin list users failed", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	users := make([]adminUserEntry, 0)
	for rows.Next() {
		var u adminUserEntry
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.IsAdmin, &u.IsActive,
			&u.EmailVerified, &u.RegistrationSource, &u.LastLoginAt, &u.CreatedAt, &u.DeletedAt); err != nil {
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

	// Revoke existing tokens so fresh JWT claims are issued on next login.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Warn("admin activate: revoke tokens failed", "error", err, "user_id", userID)
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

// AdminResetPassword handles POST /admin/users/{id}/reset-password.
// Two modes:
//  1. Set a temporary password directly (must_change_password=true, all tokens revoked).
//  2. Send a password-reset email via SMTP.
func (h *Handler) AdminResetPassword(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	var req struct {
		Password  *string `json:"password,omitempty"`
		SendEmail bool    `json:"send_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	targetUser, err := h.DB.GetUserByID(r.Context(), targetID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	ip := middleware.ClientIP(r)

	if req.Password != nil {
		// HIBP-Check — only block if definitively compromised; network errors are non-fatal.
		compromised, err := CheckHIBP(*req.Password)
		if err == nil && compromised {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
			return
		}
		hash, err := argon2id.CreateHash(*req.Password, GetArgon2Params())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.UpdatePassword(r.Context(), targetID, hash); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if _, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET must_change_password = true WHERE id = $1`, targetID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.RevokeAllUserTokens(r.Context(), targetID); err != nil {
			slog.Warn("admin reset-password: revoke tokens failed", "error", err, "user_id", targetID)
		}
		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_reset_password", ip, r.UserAgent(),
			map[string]string{"target_user_id": targetID, "method": "password"})
		slog.Info("admin reset password (direct)", "target_user_id", targetID, "admin_id", adminID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "password_reset"})

	} else if req.SendEmail {
		if h.Config.SMTPHost == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "smtp_not_configured"})
			return
		}
		token, err := GenerateToken()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(targetID),
			HashToken(token), "reset", time.Now().Add(1*time.Hour)); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		go h.Mailer.SendResetEmail(*targetUser.Email, targetUser.Username, token)
		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_reset_password", ip, r.UserAgent(),
			map[string]string{"target_user_id": targetID, "method": "email"})
		slog.Info("admin reset password (email)", "target_user_id", targetID, "admin_id", adminID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "reset_email_sent"})

	} else {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
	}
}

// notifyAdminsNewRegistration emails all active admins about a pending approval request.
// Must be called in a goroutine — uses context.Background() since the request context
// is already cancelled by the time the goroutine runs.
func (h *Handler) notifyAdminsNewRegistration(user *model.User) {
	rows, err := h.DB.Pool.Query(context.Background(),
		`SELECT email FROM users WHERE is_admin = true AND is_active = true AND email IS NOT NULL AND deleted_at IS NULL`)
	if err != nil {
		slog.Warn("notifyAdminsNewRegistration: query admins failed", "error", err)
		return
	}
	defer rows.Close()

	var admins []string
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err == nil {
			admins = append(admins, email)
		}
	}

	if user.Email == nil {
		return
	}
	for _, email := range admins {
		h.Mailer.SendAdminApprovalNeeded(email, user.Username, *user.Email)
	}
}
