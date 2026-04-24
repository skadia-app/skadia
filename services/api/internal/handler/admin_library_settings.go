// services/api/internal/handler/admin_library_settings.go
package handler

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/shadowfs"
)

// userFacingSchemaFileTypes is the canonical list from shadowfs (MED #26).
var userFacingSchemaFileTypes = shadowfs.UserFacingFileTypes

// GetAdminLibrarySettings returns all library.* settings for admin consumption.
// GET /api/v1/admin/library-settings
func (h *Handler) GetAdminLibrarySettings(w http.ResponseWriter, r *http.Request) {
	userCanEditDir := h.Settings.Get("library.user_can_edit_dir_name").Value == "true"
	userCanEditSchemas := h.Settings.Get("library.user_can_edit_schemas").Value == "true"

	defaultSchemas := make(map[string]string, 5)
	for _, ft := range userFacingSchemaFileTypes {
		defaultSchemas[ft] = h.Settings.Get("library.default_schema." + ft).Value
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"user_can_edit_dir_name": userCanEditDir,
		"user_can_edit_schemas":  userCanEditSchemas,
		"default_schemas":        defaultSchemas,
	})
}

// UpdateAdminLibrarySettings replaces all 7 library.* keys atomically.
// PUT /api/v1/admin/library-settings
func (h *Handler) UpdateAdminLibrarySettings(w http.ResponseWriter, r *http.Request) {
	adminID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	body, ok := httputil.DecodeJSON[struct {
		UserCanEditDirName bool              `json:"user_can_edit_dir_name"`
		UserCanEditSchemas bool              `json:"user_can_edit_schemas"`
		DefaultSchemas     map[string]string `json:"default_schemas"`
	}](w, r)
	if !ok {
		return
	}

	// LOW #27: nil means the key was absent in the JSON body entirely.
	if body.DefaultSchemas == nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
			"default_schemas is required", r.URL.Path)
		return
	}

	// HIGH #12: Require all 5 file-type keys to be present. Missing keys
	// would silently leave stale DB values; extra keys would pass through
	// the loop below unchecked. Exact-5 enforces full-replacement semantics.
	// NIT #17: Collect all missing AND extra keys before returning so the caller
	// gets a single diagnostic message instead of one-at-a-time failures.
	{
		expected := userFacingSchemaFileTypes
		var missing, extras []string
		for _, ft := range expected {
			if _, ok := body.DefaultSchemas[ft]; !ok {
				missing = append(missing, ft)
			}
		}
		for ft := range body.DefaultSchemas {
			found := false
			for _, e := range expected {
				if ft == e {
					found = true
					break
				}
			}
			if !found {
				extras = append(extras, ft)
			}
		}
		if len(missing) > 0 || len(extras) > 0 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				fmt.Sprintf("default_schemas: missing=%v extras=%v", missing, extras), r.URL.Path)
			return
		}
	}

	// Validate schemas
	renderer := shadowfs.NewSchemaRenderer()
	for ft, schema := range body.DefaultSchemas {
		// All keys are validated as known above (missing/extras check), so no
		// unknown-file-type branch is needed here.

		if piiKey := shadowfs.ContainsPIIVariable(schema); piiKey != "" {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				fmt.Sprintf("default_schemas.%s contains PII variable {%s}", ft, piiKey), r.URL.Path)
			return
		}

		if _, err := renderer.ValidateSchema(ft, schema); err != nil {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				fmt.Sprintf("default_schemas.%s: %v", ft, err), r.URL.Path)
			return
		}
	}

	// Build updates map for SettingsStore.Put
	updates := map[string]*string{}
	boolToStr := func(b bool) *string {
		if b {
			s := "true"
			return &s
		}
		s := "false"
		return &s
	}
	updates["library.user_can_edit_dir_name"] = boolToStr(body.UserCanEditDirName)
	updates["library.user_can_edit_schemas"] = boolToStr(body.UserCanEditSchemas)
	for ft, schema := range body.DefaultSchemas {
		s := schema
		updates["library.default_schema."+ft] = &s
	}

	if _, err := h.Settings.Put(r.Context(), updates, adminID); err != nil {
		slog.Error("admin library settings: put", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to save", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
