package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// GetMLSystemSettings returns ML system-wide settings (nightly jobs config).
//
// GET /api/v1/settings/ml
func (h *Handler) GetMLSystemSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		"SELECT setting_key, value_json FROM ml_system_settings ORDER BY setting_key")
	if err != nil {
		slog.Error("get ml system settings failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get ML settings", r.URL.Path)
		return
	}
	defer rows.Close()

	result := map[string]any{}
	for rows.Next() {
		var key string
		var valueJSON []byte
		if err := rows.Scan(&key, &valueJSON); err != nil {
			continue
		}
		var val any
		_ = json.Unmarshal(valueJSON, &val)
		result[key] = val
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// validMLSystemSettingKeys is the allowlist of accepted system-level ML setting keys.
var validMLSystemSettingKeys = map[string]bool{
	"nightly_feature_engineering":   true,
	"nightly_tsfm_analysis":         true,
	"nightly_changepoint_detection": true,
}

// PutMLSystemSetting updates a single ML system setting.
//
// PUT /api/v1/settings/ml/{key}
func (h *Handler) PutMLSystemSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing setting key", r.URL.Path)
		return
	}
	if !validMLSystemSettingKeys[key] {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid setting key", r.URL.Path)
		return
	}

	var body any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	valueJSON, err := json.Marshal(body)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to marshal value", r.URL.Path)
		return
	}

	_, err = h.DB.Pool.Exec(r.Context(),
		`INSERT INTO ml_system_settings (setting_key, value_json, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (setting_key) DO UPDATE SET value_json = EXCLUDED.value_json, updated_at = NOW()`,
		key, valueJSON)
	if err != nil {
		slog.Error("put ml system setting failed", "key", key, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save ML setting", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}

// GetMLUserSettings returns ML user preferences (notification toggles).
//
// GET /api/v1/settings/ml/user
func (h *Handler) GetMLUserSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	rows, err := h.DB.Pool.Query(r.Context(),
		"SELECT setting_key, value_json FROM ml_user_settings WHERE user_id = $1 ORDER BY setting_key",
		userID)
	if err != nil {
		slog.Error("get ml user settings failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get ML user settings", r.URL.Path)
		return
	}
	defer rows.Close()

	result := map[string]any{}
	for rows.Next() {
		var key string
		var valueJSON []byte
		if err := rows.Scan(&key, &valueJSON); err != nil {
			continue
		}
		var val any
		_ = json.Unmarshal(valueJSON, &val)
		result[key] = val
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// validMLUserSettingKeys is the allowlist of accepted user-level ML setting keys.
var validMLUserSettingKeys = map[string]bool{
	"chat_profile":      true,
	"notify_enrichment": true,
	"notify_anomaly":    true,
	"notify_gear":       true,
	"notify_chat":       true,
}

// PutMLUserSetting upserts a single ML user setting.
//
// PUT /api/v1/settings/ml/user/{key}
func (h *Handler) PutMLUserSetting(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	key := chi.URLParam(r, "key")
	if key == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing setting key", r.URL.Path)
		return
	}
	if !validMLUserSettingKeys[key] {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid setting key", r.URL.Path)
		return
	}

	var body any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON", r.URL.Path)
		return
	}

	valueJSON, err := json.Marshal(body)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to marshal value", r.URL.Path)
		return
	}

	_, err = h.DB.Pool.Exec(r.Context(),
		`INSERT INTO ml_user_settings (user_id, setting_key, value_json, updated_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (user_id, setting_key) DO UPDATE SET value_json = EXCLUDED.value_json, updated_at = NOW()`,
		userID, key, valueJSON)
	if err != nil {
		slog.Error("put ml user setting failed", "key", key, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save ML user setting", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "saved"})
}
