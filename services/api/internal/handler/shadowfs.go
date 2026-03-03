package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
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

// GetShadowFSSettings returns the user's shadow FS configuration.
func (h *Handler) GetShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	schema, err := h.DB.GetUserFileNamingSchema(r.Context(), userID)
	if err != nil {
		slog.Error("get file naming schema failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load settings", r.URL.Path)
		return
	}

	if schema == "" {
		schema = shadowfs.DefaultSchema
	}

	// Read hotfolder_enabled from DB (user preference overrides env default).
	hotfolderEnabled, err := h.DB.GetUserHotfolderEnabled(r.Context(), userID)
	if err != nil {
		slog.Warn("get hotfolder enabled failed, using env default", "error", err)
		if h.ShadowFS != nil {
			hotfolderEnabled = h.ShadowFS.HotfolderEnabled
		}
	}

	resp := ShadowFSSettingsResponse{
		Schema:           schema,
		HotfolderEnabled: hotfolderEnabled,
	}

	if h.ShadowFS != nil {
		resp.HotfolderPath = h.ShadowFS.HotfolderPath
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateShadowFSSettings updates the user's file naming schema and/or hotfolder toggle.
func (h *Handler) UpdateShadowFSSettings(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req ShadowFSSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	// Update schema if provided.
	if req.Schema != "" {
		if err := h.DB.UpdateUserFileNamingSchema(r.Context(), userID, req.Schema); err != nil {
			slog.Error("update file naming schema failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update settings", r.URL.Path)
			return
		}
	}

	// Update hotfolder_enabled if provided.
	if req.HotfolderEnabled != nil {
		if err := h.DB.UpdateUserHotfolderEnabled(r.Context(), userID, *req.HotfolderEnabled); err != nil {
			slog.Error("update hotfolder enabled failed", "error", err)
			writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update hotfolder setting", r.URL.Path)
			return
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
	if h.ShadowFS != nil {
		resp.HotfolderPath = h.ShadowFS.HotfolderPath
	}

	writeJSON(w, http.StatusOK, resp)
}

// PreviewRenameRequest is the POST body for previewing a rename.
type PreviewRenameRequest struct {
	NewSchema string `json:"new_schema"`
}

// PreviewRename returns a preview of what files would be renamed.
func (h *Handler) PreviewRename(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	if h.ShadowFS == nil {
		writeError(w, http.StatusServiceUnavailable, "Not Configured", "shadow file system not enabled", r.URL.Path)
		return
	}

	var req PreviewRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	if req.NewSchema == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "new_schema is required", r.URL.Path)
		return
	}

	activities, err := h.DB.ListActivitiesWithLocalPath(r.Context(), userID)
	if err != nil {
		slog.Error("list activities for rename preview failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to load activities", r.URL.Path)
		return
	}

	var items []shadowfs.RenameItem
	for _, a := range activities {
		info := activityToShadowInfo(a)
		newPath := shadowfs.RenderPath(info, req.NewSchema)
		newPath = shadowfs.ResolveCollision(h.ShadowFS.BasePath, newPath)

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

	writeJSON(w, http.StatusOK, items)
}

// ExecuteRename performs the actual rename of shadow files.
func (h *Handler) ExecuteRename(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	if h.ShadowFS == nil {
		writeError(w, http.StatusServiceUnavailable, "Not Configured", "shadow file system not enabled", r.URL.Path)
		return
	}

	var items []shadowfs.RenameItem
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	result := shadowfs.ExecuteRename(h.ShadowFS.BasePath, items)

	// Update local_file_path in DB for successfully renamed files
	for _, item := range items {
		if err := h.DB.UpdateActivityLocalFilePathByOldPath(r.Context(), userID, item.OldPath, item.NewPath); err != nil {
			slog.Error("update local file path after rename failed",
				"old", item.OldPath,
				"new", item.NewPath,
				"error", err,
			)
		}
	}

	writeJSON(w, http.StatusOK, result)
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
	relPath = shadowfs.ResolveCollision(h.ShadowFS.BasePath, relPath)

	metaBytes, err := shadowfs.BuildShadowMeta(
		activityID, data.FileHash, data.OriginalName,
		data.FileFormat, data.SportType, userID,
	)
	if err != nil {
		return err
	}

	if err := shadowfs.WriteShadowFile(h.ShadowFS.BasePath, relPath, fileBytes, metaBytes); err != nil {
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
