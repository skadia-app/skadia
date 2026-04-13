package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// testEmailCooldown tracks per-admin cooldown for the test-email endpoint.
var (
	testEmailLastSent   = make(map[string]time.Time)
	testEmailLastSentMu sync.Mutex
	testEmailCooldown   = 60 * time.Second
)

// AdminGetSettings handles GET /api/v1/admin/settings.
// Returns all settings grouped by category; encrypted values are masked as "***".
func (h *Handler) AdminGetSettings(w http.ResponseWriter, r *http.Request) {
	all := h.Settings.GetAll()

	// Mask encrypted values: replace plaintext with "***" for the API response.
	for group, keys := range all {
		for key, cs := range keys {
			if cs.Encrypted {
				cs.Value = "***"
				all[group][key] = cs
			}
		}
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, all)
}

// AdminPutSettings handles PUT /api/v1/admin/settings.
// Accepts a map[string]*string where nil = delete (revert to default) and "***" = no change.
func (h *Handler) AdminPutSettings(w http.ResponseWriter, r *http.Request) {
	var updates map[string]*string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	adminID := middleware.GetUserID(r.Context())

	changed, err := h.Settings.Put(r.Context(), updates, adminID)
	if err != nil {
		msg := err.Error()
		// Validation/policy errors (unknown key, locked field, cross-field constraint,
		// value format) contain user-readable detail and should be returned as 400.
		// DB/crypto errors contain internal detail — return 500 with a generic message.
		isClientError := strings.Contains(msg, "unknown settings key") ||
			strings.Contains(msg, "is locked by environment variable") ||
			strings.Contains(msg, "requires") ||
			strings.Contains(msg, "must be") ||
			strings.Contains(msg, "cannot store")
		if isClientError {
			httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": msg})
			return
		}
		slog.Error("admin put settings failed", "error", err, "admin_id", adminID)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	// Build the changed keys list for the audit log.
	// For encrypted fields, log only the key name (never the value).
	changedKeys := make([]string, 0, len(changed))
	for key := range changed {
		changedKeys = append(changedKeys, key)
	}

	// Audit log: record which keys changed (no values logged for any key).
	auditDetails := make(map[string]string, 1)
	if len(changedKeys) > 0 {
		// Encode the list as a comma-joined string to fit the map[string]string audit schema.
		joined := ""
		for i, k := range changedKeys {
			if i > 0 {
				joined += ","
			}
			joined += k
		}
		auditDetails["changed_keys"] = joined
	}
	go h.DB.LogAuthEvent(r.Context(), adminID, "admin_settings_update",
		middleware.ClientIP(r), r.UserAgent(), auditDetails)

	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"status":  "settings_updated",
		"changed": changedKeys,
	})
}

// AdminReloadSettings handles POST /api/v1/admin/settings/reload.
// Forces a cache reload from the database (useful when DB was modified externally).
func (h *Handler) AdminReloadSettings(w http.ResponseWriter, r *http.Request) {
	if err := h.Settings.Reload(r.Context()); err != nil {
		slog.Error("admin reload settings failed", "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "cache_reloaded"})
}

// AdminTestEmail handles POST /api/v1/admin/settings/test-email.
// Sends a test email to the requesting admin to verify SMTP configuration.
// Rate-limited to 1 request per minute per admin to prevent SMTP flooding.
func (h *Handler) AdminTestEmail(w http.ResponseWriter, r *http.Request) {
	smtpHost := h.Settings.Get("smtp.host").Value
	if smtpHost == "" {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "smtp_not_configured"})
		return
	}

	adminID := middleware.GetUserID(r.Context())

	// Per-admin cooldown.
	testEmailLastSentMu.Lock()
	if last, ok := testEmailLastSent[adminID]; ok && time.Since(last) < testEmailCooldown {
		remaining := testEmailCooldown - time.Since(last)
		testEmailLastSentMu.Unlock()
		httputil.WriteJSONNoCache(w, http.StatusTooManyRequests, map[string]any{
			"error":       "rate_limited",
			"retry_after": int(remaining.Seconds()),
		})
		return
	}
	testEmailLastSent[adminID] = time.Now()
	testEmailLastSentMu.Unlock()

	admin, err := h.DB.GetUserByID(r.Context(), adminID)
	if err != nil {
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}
	if admin.Email == nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "admin_has_no_email"})
		return
	}

	if err := h.Mailer.SendTestEmail(*admin.Email, admin.Username); err != nil {
		slog.Error("admin test email failed", "error", err, "admin_id", adminID)
		httputil.WriteJSONNoCache(w, http.StatusBadGateway, map[string]string{"error": "smtp_send_failed"})
		return
	}

	go h.DB.LogAuthEvent(r.Context(), adminID, "admin_test_email",
		middleware.ClientIP(r), r.UserAgent(), nil)
	slog.Info("admin test email sent", "admin_id", adminID, "to", *admin.Email)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]string{"status": "test_email_sent"})
}
