package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
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
	HotfolderEnabled   bool       `json:"hotfolder_enabled"`
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
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	query := `SELECT id, username, display_name, email, is_admin, is_active, email_verified,
    registration_source, last_login_at, created_at, deleted_at, hotfolder_enabled
FROM users
WHERE ` + whereClause + `
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`

	rows, err := h.DB.Pool.Query(r.Context(), query, escapedSearch, status, limit, offset)
	if err != nil {
		slog.Error("admin list users failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	users := make([]adminUserEntry, 0)
	for rows.Next() {
		var u adminUserEntry
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Email, &u.IsAdmin, &u.IsActive,
			&u.EmailVerified, &u.RegistrationSource, &u.LastLoginAt, &u.CreatedAt, &u.DeletedAt, &u.HotfolderEnabled); err != nil {
			slog.Error("admin list users scan failed", "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin list users rows error", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
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
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	if err := h.DB.SetUserActive(r.Context(), userID, true); err != nil {
		slog.Error("admin activate user failed", "error", err, "user_id", userID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Revoke existing tokens so fresh JWT claims are issued on next login.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Warn("admin activate: revoke tokens failed", "error", err, "user_id", userID)
	}

	adminID := middleware.GetUserID(r.Context())
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_activate_user", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin activated user", "user_id", userID, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "activated"})
}

// AdminDeactivateUser handles POST /admin/users/{id}/deactivate.
func (h *Handler) AdminDeactivateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	if userID == adminID {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "cannot_deactivate_self"})
		return
	}

	if err := h.DB.SetUserActive(r.Context(), userID, false); err != nil {
		slog.Error("admin deactivate user failed", "error", err, "user_id", userID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Revoke all tokens to force immediate logout.
	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Warn("admin deactivate: revoke tokens failed", "error", err, "user_id", userID)
	}

	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_deactivate_user", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin deactivated user", "user_id", userID, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "deactivated"})
}

// adminSessionEntry is the JSON response shape for admin session listings.
type adminSessionEntry struct {
	FamilyID  string    `json:"family_id"`
	UserAgent *string   `json:"user_agent,omitempty"`
	IPAddress *string   `json:"ip_address,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// AdminListUserSessions handles GET /admin/users/{id}/sessions.
func (h *Handler) AdminListUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
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
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	defer rows.Close()

	sessions := make([]adminSessionEntry, 0)
	for rows.Next() {
		var s adminSessionEntry
		if err := rows.Scan(&s.FamilyID, &s.UserAgent, &s.IPAddress,
			&s.CreatedAt, &s.ExpiresAt, &s.Revoked); err != nil {
			slog.Error("admin list user sessions scan failed", "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		slog.Error("admin list user sessions rows error", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// AdminRevokeUserSessions handles POST /admin/users/{id}/revoke-sessions.
func (h *Handler) AdminRevokeUserSessions(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(userID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	if err := h.DB.RevokeAllUserTokens(r.Context(), userID); err != nil {
		slog.Error("admin revoke user sessions failed", "error", err, "user_id", userID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_revoke_sessions", middleware.ClientIP(r), r.UserAgent(), map[string]string{"target_user_id": userID})
	slog.Info("admin revoked all sessions", "user_id", userID, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "sessions_revoked"})
}

// AdminResetPassword handles POST /admin/users/{id}/reset-password.
// Two modes:
//  1. Set a temporary password directly (must_change_password=true, all tokens revoked).
//  2. Send a password-reset email via SMTP.
func (h *Handler) AdminResetPassword(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	var req struct {
		Password  *string `json:"password,omitempty"`
		SendEmail bool    `json:"send_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	targetUser, err := h.DB.GetUserByID(r.Context(), targetID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
		return
	}

	adminID := middleware.GetUserID(r.Context())
	ip := middleware.ClientIP(r)

	if req.Password != nil {
		if len(*req.Password) < 10 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_too_short"})
			return
		}
		if len(*req.Password) > 1000 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_too_long"})
			return
		}
		// HIBP-Check — only block if definitively compromised; network errors are non-fatal.
		compromised, err := CheckHIBP(*req.Password)
		if err == nil && compromised {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "password_compromised"})
			return
		}
		hash, err := argon2id.CreateHash(*req.Password, GetArgon2Params())
		if err != nil {
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.UpdatePassword(r.Context(), targetID, hash); err != nil {
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if _, err := h.DB.Pool.Exec(r.Context(),
			`UPDATE users SET must_change_password = true WHERE id = $1`, targetID); err != nil {
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.RevokeAllUserTokens(r.Context(), targetID); err != nil {
			slog.Warn("admin reset-password: revoke tokens failed", "error", err, "user_id", targetID)
		}
		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_reset_password", ip, r.UserAgent(),
			map[string]string{"target_user_id": targetID, "method": "password"})
		slog.Info("admin reset password (direct)", "target_user_id", targetID, "admin_id", adminID)
		httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "password_reset"})

	} else if req.SendEmail {
		if h.Settings.Get("smtp.host").Value == "" {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "smtp_not_configured"})
			return
		}
		if targetUser.Email == nil {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "user_has_no_email"})
			return
		}
		token, err := GenerateToken()
		if err != nil {
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if err := h.DB.CreateEmailToken(r.Context(), uuid.MustParse(targetID),
			HashToken(token), "reset", time.Now().Add(1*time.Hour)); err != nil {
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		go h.Mailer.SendResetEmail(*targetUser.Email, targetUser.Username, token)
		go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_reset_password", ip, r.UserAgent(),
			map[string]string{"target_user_id": targetID, "method": "email"})
		slog.Info("admin reset password (email)", "target_user_id", targetID, "admin_id", adminID)
		httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "reset_email_sent"})

	} else {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
	}
}

// AdminAuditLog handles GET /admin/audit-log — paginated, filtered auth audit log.
func (h *Handler) AdminAuditLog(w http.ResponseWriter, r *http.Request) {
	eventType := r.URL.Query().Get("event_type")
	userID := r.URL.Query().Get("user_id")
	limit := parseIntDefault(r.URL.Query().Get("limit"), 50)
	offset := parseIntDefault(r.URL.Query().Get("offset"), 0)
	if limit > 200 {
		limit = 200
	}

	var from, to *time.Time
	if v := r.URL.Query().Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_from_timestamp"})
			return
		}
		from = &t
	}
	if v := r.URL.Query().Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_to_timestamp"})
			return
		}
		to = &t
	}

	entries, total, err := h.DB.GetAuditLog(r.Context(), eventType, userID, from, to, limit, offset)
	if err != nil {
		slog.Error("admin audit log query failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"entries": entries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

// parseIntDefault parses s as an integer, returning def on empty string, parse error, or negative value.
func parseIntDefault(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return def
	}
	return v
}

// AdminSetUserAdmin handles POST /admin/users/{id}/set-admin.
// Promotes or demotes a user's admin status.
// Request body: {"is_admin": true|false}
func (h *Handler) AdminSetUserAdmin(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	adminID := middleware.GetUserID(r.Context())
	if targetID == adminID && !req.IsAdmin {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "cannot_demote_self"})
		return
	}
	if !req.IsAdmin {
		var count int
		if err := h.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM users WHERE is_admin = true AND is_active = true AND id != $1 AND deleted_at IS NULL`,
			targetID).Scan(&count); err != nil {
			slog.Error("admin set-admin: count admins failed", "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if count == 0 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "last_admin"})
			return
		}
	}
	if _, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET is_admin = $2 WHERE id = $1`, targetID, req.IsAdmin); err != nil {
		slog.Error("admin set-admin failed", "error", err, "target_user_id", targetID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	h.DB.RevokeAllUserTokens(r.Context(), targetID)
	event := "admin_promote_user"
	if !req.IsAdmin {
		event = "admin_demote_user"
	}
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, event, middleware.ClientIP(r), r.UserAgent(),
		map[string]string{"target_user_id": targetID})
	slog.Info("admin set-admin", "target_user_id", targetID, "is_admin", req.IsAdmin, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "ok"})
}

// AdminDeleteUser handles DELETE /admin/users/{id}.
// Soft-deletes a user after username confirmation.
// Request body: {"confirm": "<username>"}
func (h *Handler) AdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	adminID := middleware.GetUserID(r.Context())
	if targetID == adminID {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "cannot_delete_self"})
		return
	}
	target, err := h.DB.GetUserByID(r.Context(), targetID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusNotFound, map[string]string{"error": "user_not_found"})
		return
	}
	if req.Confirm != target.Username {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "confirmation_mismatch"})
		return
	}
	if target.IsAdmin {
		var count int
		if err := h.DB.Pool.QueryRow(r.Context(),
			`SELECT count(*) FROM users WHERE is_admin = true AND is_active = true AND id != $1 AND deleted_at IS NULL`,
			targetID).Scan(&count); err != nil {
			slog.Error("admin delete user: count admins failed", "error", err)
			httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
			return
		}
		if count == 0 {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "last_admin"})
			return
		}
	}
	if _, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE users SET deleted_at = now(), is_active = false WHERE id = $1`, targetID); err != nil {
		slog.Error("admin delete user failed", "error", err, "target_user_id", targetID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	h.DB.RevokeAllUserTokens(r.Context(), targetID)
	go h.DB.LogAuthEvent(h.shutdownCtx, adminID, "admin_delete_user", middleware.ClientIP(r), r.UserAgent(),
		map[string]string{"target_user_id": targetID, "target_username": target.Username})
	slog.Info("admin deleted user", "target_user_id", targetID, "target_username", target.Username, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// notifyAdminsNewRegistration emails all active admins about a pending approval request.
// Must be called in a goroutine — uses shutdownCtx so the query is cancelled on shutdown.
func (h *Handler) notifyAdminsNewRegistration(user *model.User) {
	rows, err := h.DB.Pool.Query(h.shutdownCtx,
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
