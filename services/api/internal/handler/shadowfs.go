package handler

import (
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// ShadowFSSettingsResponse is the GET response for shadow FS settings.
type ShadowFSSettingsResponse struct {
	Schema           string `json:"schema"`
	HotfolderPath    string `json:"hotfolder_path"`
	HotfolderEnabled bool   `json:"hotfolder_enabled"`
}

// ShadowFSSettingsRequest is the PUT request body for updating shadow FS settings.
type ShadowFSSettingsRequest struct {
	Schema           string `json:"schema"`
	HotfolderEnabled *bool  `json:"hotfolder_enabled,omitempty"`
}

// GetShadowFSSettingsCompat is a backward-compatibility shim for GET /settings/shadow-fs.
// It reads the activity schema from user_file_schemas (written by migration 000232) and
// returns it in the legacy ShadowFSSettingsResponse shape so existing clients continue to work.
//
// Deprecated: use GET /settings/file-schemas instead.
func (h *Handler) GetShadowFSSettingsCompat(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	// Fetch activity schema from user_file_schemas (populated by migration 000232).
	var schema string
	_ = h.DB.Pool.QueryRow(r.Context(),
		`SELECT schema_template FROM user_file_schemas WHERE user_id = $1 AND file_type = 'activity'`,
		userID,
	).Scan(&schema)
	if schema == "" {
		schema = shadowfs.NewSchemaRenderer().DefaultSchema("activity")
	}

	hotfolderEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
	if err != nil {
		slog.Warn("shadow-fs compat: get hotfolder enabled failed, using env default", "error", err)
		if h.ShadowFSConfig != nil {
			hotfolderEnabled = h.ShadowFSConfig.HotfolderEnabled
		}
	}

	resp := ShadowFSSettingsResponse{
		Schema:           schema,
		HotfolderEnabled: hotfolderEnabled,
	}
	if h.WatcherManager != nil {
		if p, err := h.WatcherManager.UserPath(userID); err == nil {
			resp.HotfolderPath = p
		}
	} else if h.ShadowFSConfig != nil {
		resp.HotfolderPath = h.ShadowFSConfig.HotfolderPath
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// UpdateShadowFSSettingsCompat is a backward-compatibility shim for PUT /settings/shadow-fs.
// Schema changes are written directly to user_file_schemas for the "activity" file type.
// HotfolderEnabled is still managed via the users table (no ShadowFS equivalent).
//
// Deprecated: use PUT /settings/file-schemas/activity instead.
func (h *Handler) UpdateShadowFSSettingsCompat(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[ShadowFSSettingsRequest](w, r)
	if !ok {
		return
	}

	// Validate schema before persisting.
	if req.Schema != "" {
		renderer := shadowfs.NewSchemaRenderer()
		if _, err := renderer.ValidateSchema("activity", req.Schema); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
			return
		}
	}

	// Persist schema update to user_file_schemas (activity type).
	if req.Schema != "" {
		if _, err := h.DB.Pool.Exec(r.Context(),
			`INSERT INTO user_file_schemas (user_id, file_type, schema_template)
			 VALUES ($1, 'activity', $2)
			 ON CONFLICT (user_id, file_type) DO UPDATE SET schema_template = EXCLUDED.schema_template`,
			userID, req.Schema,
		); err != nil {
			slog.Error("shadow-fs compat: upsert activity schema failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update settings", r.URL.Path)
			return
		}
	}

	// Handle hotfolder toggle (no ShadowFS equivalent — still managed via users table).
	if req.HotfolderEnabled != nil {
		currentEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
		if err != nil {
			slog.Warn("shadow-fs compat: get hotfolder enabled failed", "error", err)
		}
		if err := h.DB.UpdateUserHotfolderEnabled(r.Context(), userID, *req.HotfolderEnabled); err != nil {
			slog.Error("shadow-fs compat: update hotfolder enabled failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update hotfolder setting", r.URL.Path)
			return
		}
		if h.WatcherManager != nil && *req.HotfolderEnabled != currentEnabled {
			if *req.HotfolderEnabled {
				if err := h.WatcherManager.EnableUser(userID); err != nil {
					slog.Warn("shadow-fs compat: hotfolder watcher enable failed", "user", userID, "error", err)
				}
			} else {
				h.WatcherManager.DisableUser(userID)
			}
		}
	}

	// Return the current full state using the compat GET handler.
	h.GetShadowFSSettingsCompat(w, r)
}

// GetShadowFSSettings is a deleted legacy handler.
// Callers should use GetShadowFSSettingsCompat instead (reads from user_file_schemas).
// This stub remains so existing route wiring does not break at compile time.
func (h *Handler) GetShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	h.GetShadowFSSettingsCompat(w, r)
}

// UpdateShadowFSSettings is a deleted legacy handler.
// Callers should use UpdateShadowFSSettingsCompat instead (writes to user_file_schemas).
// This stub remains so existing route wiring does not break at compile time.
func (h *Handler) UpdateShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	h.UpdateShadowFSSettingsCompat(w, r)
}

// PreviewRename is a deleted legacy handler.
// File renaming is now managed entirely through the FileStore/ShadowFS layer.
func (h *Handler) PreviewRename(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, []struct{}{})
}

// ExecuteRename is a deleted legacy handler.
// File renaming is now managed entirely through the FileStore/ShadowFS layer.
func (h *Handler) ExecuteRename(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"succeeded": 0, "failed": 0})
}
