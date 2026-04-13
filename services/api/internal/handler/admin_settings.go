package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/settings"
	"github.com/skadia-app/skadia/services/api/internal/shutdown"
)

// testEmailCooldown tracks per-admin cooldown for the test-email endpoint.
var (
	testEmailLastSent   = make(map[string]time.Time)
	testEmailLastSentMu sync.Mutex
	testEmailCooldown   = 60 * time.Second
)

// AdminGetSettings handles GET /api/v1/admin/settings.
// Returns all settings grouped by category; encrypted values are masked as "***".
// Also includes a "_meta" envelope with read-only server-side context that is
// env-only (not settable via PUT /settings).
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

	// Attach read-only server metadata so the frontend can make decisions
	// (e.g. disable Library Sync toggles when storage backend is S3).
	storageBackend := "local"
	if h.Config != nil && h.Config.StorageBackend != "" {
		storageBackend = h.Config.StorageBackend
	}

	resp := map[string]any{}
	for k, v := range all {
		resp[k] = v
	}
	resp["_meta"] = map[string]string{
		"storage_backend": storageBackend,
	}

	httputil.WriteJSONNoCache(w, http.StatusOK, resp)
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

	// Bug 2 fix: capture before-values for audit trail before any mutation.
	beforeValues := make(map[string]string, len(updates))
	for key := range updates {
		beforeValues[key] = h.Settings.Get(key).Value
	}

	// Bug 3 fix: extract timescale.compression before Put() — it must not be
	// persisted by Put() itself; the background job writes it after success.
	var compressionChange *string
	if v, ok := updates["timescale.compression"]; ok {
		compressionChange = v
		delete(updates, "timescale.compression")
	}

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

	// React to runtime-significant setting changes.

	// Hotfolder mode change.
	if newMode, ok := changed["hotfolder.mode"]; ok {
		switch newMode {
		case "disabled":
			if h.WatcherManager != nil {
				h.WatcherManager.StopAll()
			}
		case "self-service", "admin-only":
			// Re-enable watchers for all users that have hotfolder_enabled=true.
			if h.WatcherManager != nil {
				go h.restartHotfolderWatchers()
			}
		}
	}

	// Library sync toggle.
	if newVal, ok := changed["library.sync_enabled"]; ok {
		if newVal == "true" {
			slog.Info("library sync enabled via admin settings")
			// TODO: start library watcher if available
		} else {
			slog.Info("library sync disabled via admin settings")
			// TODO: stop library watcher if available
		}
	}

	// TimescaleDB compression toggle — run as background job.
	// The key is NOT written by Put(); the job writes it after successful completion.
	if compressionChange != nil {
		newVal := *compressionChange
		jobID, err := h.DB.InsertBackgroundJob(r.Context(), adminID, "compression_toggle", nil, nil, 0)
		if err == nil {
			shutdown.Go(jobID, func(_ context.Context) {
				if newVal == "true" {
					h.enableCompression(jobID)
				} else {
					h.disableCompression(jobID)
				}
			})
		}
		// Include the key in changedKeys for the audit log.
		changed["timescale.compression"] = newVal
	}

	// Build the changed keys list for the audit log.
	// For encrypted fields, log only the key name (never the value).
	changedKeys := make([]string, 0, len(changed))
	for key := range changed {
		changedKeys = append(changedKeys, key)
	}

	// Audit log: record changed keys with before/after values for non-encrypted keys.
	meta := map[string]string{
		"changed_keys": strings.Join(changedKeys, ","),
	}
	for _, key := range changedKeys {
		def, ok := settings.KeysByKey[key]
		if ok && !def.Encrypted {
			meta["before_"+key] = beforeValues[key]
			meta["after_"+key] = changed[key]
		}
	}
	go h.DB.LogAuthEvent(r.Context(), adminID, "admin_settings_update",
		middleware.ClientIP(r), r.UserAgent(), meta)

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

// restartHotfolderWatchers re-enables per-user hotfolder watchers for all users
// that have hotfolder_enabled=true in the database. Called when the hotfolder mode
// changes from "disabled" to "self-service" or "admin-only".
func (h *Handler) restartHotfolderWatchers() {
	rows, err := h.DB.Pool.Query(context.Background(),
		`SELECT id FROM users WHERE hotfolder_enabled = true AND is_active = true AND deleted_at IS NULL`)
	if err != nil {
		slog.Error("restartHotfolderWatchers: query failed", "error", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err == nil {
			if err := h.WatcherManager.EnableUser(userID, true); err != nil {
				slog.Warn("restartHotfolderWatchers: enable failed", "user_id", userID, "error", err)
			}
		}
	}
}

// enableCompression runs as a background job to enable TimescaleDB compression on
// activity_streams with a 7-day compression policy.
func (h *Handler) enableCompression(jobID string) {
	ctx := context.Background()
	_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, "enabling compression")

	_, err := h.DB.Pool.Exec(ctx, `ALTER TABLE activity_streams SET (
		timescaledb.compress,
		timescaledb.compress_segmentby = 'activity_id',
		timescaledb.compress_orderby = 'timestamp'
	)`)
	if err != nil {
		slog.Error("enableCompression: ALTER TABLE failed", "error", err)
		_ = h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
		return
	}

	_, err = h.DB.Pool.Exec(ctx, `SELECT add_compression_policy('activity_streams', INTERVAL '7 days', if_not_exists => true)`)
	if err != nil {
		slog.Error("enableCompression: add_compression_policy failed", "error", err)
		_ = h.DB.CompleteBackgroundJob(ctx, jobID, true, err.Error())
		return
	}

	_, _ = h.Settings.Put(ctx, map[string]*string{"timescale.compression": strPtr("true")}, "")

	slog.Info("compression enabled successfully")
	_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// disableCompression runs as a background job to remove the TimescaleDB compression
// policy and decompress all existing chunks on activity_streams.
func (h *Handler) disableCompression(jobID string) {
	ctx := context.Background()
	_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, "removing compression policy")

	h.DB.Pool.Exec(ctx, `SELECT remove_compression_policy('activity_streams', if_exists => true)`)

	_ = h.DB.UpdateBackgroundJobPhase(ctx, jobID, "decompressing chunks")

	rows, err := h.DB.Pool.Query(ctx, `SELECT show_chunks('activity_streams', older_than => INTERVAL '0 seconds')`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var chunkName string
			if rows.Scan(&chunkName) == nil {
				h.DB.Pool.Exec(ctx, `SELECT decompress_chunk($1, if_compressed => true)`, chunkName)
			}
		}
	}

	_, _ = h.Settings.Put(ctx, map[string]*string{"timescale.compression": strPtr("false")}, "")

	slog.Info("compression disabled successfully")
	_ = h.DB.CompleteBackgroundJob(ctx, jobID, false, "")
}

// strPtr returns a pointer to a copy of s. Used for settings Put calls.
func strPtr(s string) *string { return &s }

// AdminToggleUserHotfolder handles POST /api/v1/admin/users/{id}/hotfolder.
// Enables or disables the per-user hotfolder watcher.
// Request body: {"enabled": true|false}
func (h *Handler) AdminToggleUserHotfolder(w http.ResponseWriter, r *http.Request) {
	targetID := chi.URLParam(r, "id")
	if _, err := uuid.Parse(targetID); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_user_id"})
		return
	}

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}

	// Verify the global hotfolder mode allows per-user hotfolders.
	mode := h.Settings.Get("hotfolder.mode").Value
	if mode == "disabled" {
		httputil.WriteJSONNoCache(w, http.StatusBadRequest, map[string]string{"error": "hotfolder_disabled"})
		return
	}

	ctx := r.Context()
	currentEnabled, err := h.DB.GetUserHotfolderEnabled(ctx, targetID)
	if err != nil {
		slog.Error("admin toggle hotfolder: get current state failed", "user_id", targetID, "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if err := h.DB.UpdateUserHotfolderEnabled(ctx, targetID, req.Enabled); err != nil {
		slog.Error("admin toggle hotfolder: update failed", "user_id", targetID, "error", err)
		httputil.WriteJSONNoCache(w, http.StatusInternalServerError, map[string]string{"error": "internal_error"})
		return
	}

	if h.WatcherManager != nil && req.Enabled != currentEnabled {
		if req.Enabled {
			if err := h.WatcherManager.EnableUser(targetID, false); err != nil {
				slog.Warn("admin toggle hotfolder: enable watcher failed", "user_id", targetID, "error", err)
			}
		} else {
			h.WatcherManager.DisableUser(targetID)
		}
	}

	adminID := middleware.GetUserID(ctx)
	go h.DB.LogAuthEvent(ctx, adminID, "admin_toggle_hotfolder",
		middleware.ClientIP(r), r.UserAgent(),
		map[string]string{"target_user_id": targetID, "enabled": strconv.FormatBool(req.Enabled)})

	slog.Info("admin toggled user hotfolder", "user_id", targetID, "enabled", req.Enabled, "admin_id", adminID)
	httputil.WriteJSONNoCache(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"enabled": req.Enabled,
	})
}
