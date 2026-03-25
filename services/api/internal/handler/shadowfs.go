package handler

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
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
		schema = shadowfs.DefaultSchema
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
		resp.HotfolderPath = h.WatcherManager.UserPath(userID)
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

// GetShadowFSSettings returns the user's shadow FS configuration.
//
// Deprecated: legacy handler reading from users.file_naming_schema (dropped in migration 000233).
// Still wired in main.go for backward compatibility; replaced by GetShadowFSSettingsCompat
// which reads from user_file_schemas.
func (h *Handler) GetShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	schema, err := h.DB.GetUserFileNamingSchema(r.Context(), userID)
	if err != nil {
		slog.Error("get file naming schema failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load settings", r.URL.Path)
		return
	}

	if schema == "" {
		schema = shadowfs.DefaultSchema
	}

	// Read hotfolder_enabled from DB (user preference overrides env default).
	hotfolderEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
	if err != nil {
		slog.Warn("get hotfolder enabled failed, using env default", "error", err)
		if h.ShadowFSConfig != nil {
			hotfolderEnabled = h.ShadowFSConfig.HotfolderEnabled
		}
	}

	resp := ShadowFSSettingsResponse{
		Schema:           schema,
		HotfolderEnabled: hotfolderEnabled,
	}

	if h.WatcherManager != nil {
		resp.HotfolderPath = h.WatcherManager.UserPath(userID)
	} else if h.ShadowFSConfig != nil {
		resp.HotfolderPath = h.ShadowFSConfig.HotfolderPath
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// UpdateShadowFSSettings updates the user's file naming schema and/or hotfolder toggle.
//
// Deprecated: legacy handler writing to users.file_naming_schema (dropped in migration 000233).
// Still wired in main.go for backward compatibility; replaced by UpdateShadowFSSettingsCompat
// which writes to user_file_schemas.
func (h *Handler) UpdateShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[ShadowFSSettingsRequest](w, r)
	if !ok {
		return
	}

	// Update schema if provided.
	if req.Schema != "" {
		if err := h.DB.UpdateUserFileNamingSchema(r.Context(), userID, req.Schema); err != nil {
			slog.Error("update file naming schema failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update settings", r.URL.Path)
			return
		}
	}

	// Update hotfolder_enabled if provided.
	if req.HotfolderEnabled != nil {
		// Read current value before update to detect changes.
		currentEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
		if err != nil {
			slog.Warn("get current hotfolder enabled failed", "error", err)
		}

		if err := h.DB.UpdateUserHotfolderEnabled(r.Context(), userID, *req.HotfolderEnabled); err != nil {
			slog.Error("update hotfolder enabled failed", "error", err)
			httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update hotfolder setting", r.URL.Path)
			return
		}

		// Start or stop the per-user watcher if the value changed.
		if h.WatcherManager != nil && *req.HotfolderEnabled != currentEnabled {
			if *req.HotfolderEnabled {
				if err := h.WatcherManager.EnableUser(userID); err != nil {
					slog.Warn("hotfolder watcher enable failed", "user", userID, "error", err)
				}
			} else {
				h.WatcherManager.DisableUser(userID)
			}
		}
	}

	// Return the current full state.
	schema, err := h.DB.GetUserFileNamingSchema(r.Context(), userID)
	if err != nil || schema == "" {
		schema = shadowfs.DefaultSchema
	}
	hotfolderEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
	if err != nil {
		slog.Warn("get hotfolder enabled after update failed", "error", err)
	}

	resp := ShadowFSSettingsResponse{
		Schema:           schema,
		HotfolderEnabled: hotfolderEnabled,
	}
	if h.WatcherManager != nil {
		resp.HotfolderPath = h.WatcherManager.UserPath(userID)
	} else if h.ShadowFSConfig != nil {
		resp.HotfolderPath = h.ShadowFSConfig.HotfolderPath
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// PreviewRenameRequest is the POST body for previewing a rename.
type PreviewRenameRequest struct {
	NewSchema string `json:"new_schema"`
}

// PreviewRename returns a preview of what files would be renamed.
func (h *Handler) PreviewRename(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.ShadowFSConfig == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Not Configured", "shadow file system not enabled", r.URL.Path)
		return
	}

	req, ok := httputil.DecodeJSON[PreviewRenameRequest](w, r)
	if !ok {
		return
	}

	if req.NewSchema == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "new_schema is required", r.URL.Path)
		return
	}

	activities, err := h.DB.ListActivitiesWithLocalPath(r.Context(), userID)
	if err != nil {
		slog.Error("list activities for rename preview failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to load activities", r.URL.Path)
		return
	}

	var items []shadowfs.RenameItem
	for _, a := range activities {
		info := activityToShadowInfo(a)
		newPath := shadowfs.RenderPath(info, req.NewSchema)
		newPath = shadowfs.ResolveCollision(h.ShadowFSConfig.BasePath, newPath)

		oldPath := ""
		if a.LocalFilePath != nil {
			oldPath = *a.LocalFilePath
		}

		if oldPath != newPath {
			items = append(items, shadowfs.RenameItem{
				OldPath: oldPath,
				NewPath: newPath,
			})
		}
	}

	if items == nil {
		items = []shadowfs.RenameItem{}
	}

	httputil.WriteJSON(w, http.StatusOK, items)
}

// ExecuteRename performs the actual rename of shadow files.
func (h *Handler) ExecuteRename(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	if h.ShadowFSConfig == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable, "Not Configured", "shadow file system not enabled", r.URL.Path)
		return
	}

	items, ok := httputil.DecodeJSON[[]shadowfs.RenameItem](w, r)
	if !ok {
		return
	}

	result := shadowfs.ExecuteRename(h.ShadowFSConfig.BasePath, items)

	// Update local_file_path in DB for successfully renamed files
	for _, item := range result.SucceededItems {
		if err := h.DB.UpdateActivityLocalFilePathByOldPath(r.Context(), userID, item.OldPath, item.NewPath); err != nil {
			slog.Error("update local file path after rename failed",
				"old", item.OldPath,
				"new", item.NewPath,
				"error", err,
			)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// WriteShadowForActivity writes the shadow file for a newly uploaded activity.
// This is called from the upload handler as a best-effort step.
func (h *Handler) WriteShadowForActivity(ctx context.Context, userID, activityID string, data *model.ActivityData, fileBytes []byte) error {
	schema, err := h.DB.GetUserFileNamingSchema(ctx, userID)
	if err != nil {
		return err
	}
	if schema == "" {
		schema = shadowfs.DefaultSchema
	}

	info := shadowfs.ActivityInfo{
		ID:           activityID,
		StartedAt:    data.StartedAt,
		SportType:    data.SportType,
		Title:        "", // Title not yet set at upload time
		DistanceM:    data.DistanceM,
		ElapsedTimeS: data.ElapsedTimeS,
		City:         "", // Geocoding not yet implemented
		FileExt:      data.FileFormat,
	}

	relPath := shadowfs.RenderPath(info, schema)
	relPath = shadowfs.ResolveCollision(h.ShadowFSConfig.BasePath, relPath)

	metaBytes, err := shadowfs.BuildShadowMeta(
		activityID, data.FileHash, data.OriginalName,
		data.FileFormat, data.SportType, userID,
	)
	if err != nil {
		return err
	}

	if err := shadowfs.WriteShadowFile(h.ShadowFSConfig.BasePath, relPath, fileBytes, metaBytes); err != nil {
		return err
	}

	// Update local_file_path in DB
	if err := h.DB.UpdateActivityLocalFilePath(ctx, activityID, relPath); err != nil {
		slog.Error("update local file path failed", "id", activityID, "error", err)
	}

	return nil
}

// activityToShadowInfo converts an ActivityWithPath to a shadowfs.ActivityInfo.
func activityToShadowInfo(a model.ActivityWithPath) shadowfs.ActivityInfo {
	info := shadowfs.ActivityInfo{
		ID:        a.ID,
		StartedAt: a.StartedAt,
		SportType: a.SportType,
		FileExt:   "fit",
	}

	if a.SubType != nil {
		info.SubCategory = *a.SubType
	}
	if a.Title != nil {
		info.Title = *a.Title
	}
	if a.City != nil {
		info.City = *a.City
	}
	if a.DistanceM != nil {
		info.DistanceM = *a.DistanceM
	}
	if a.ElapsedTimeS != nil {
		info.ElapsedTimeS = *a.ElapsedTimeS
	}
	if a.OriginalFileFormat != nil {
		info.FileExt = *a.OriginalFileFormat
	}

	return info
}
