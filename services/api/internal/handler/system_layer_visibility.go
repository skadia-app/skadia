package handler

import (
	"errors"
	"math"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
)

// GetSystemLayerKeys — public (no auth required).
// Returns the 14 SystemLayerDescriptors that are the code source-of-truth.
// Mounted on the top-level router (no jwtAuth middleware) in main.go.
func (h *Handler) GetSystemLayerKeys(w http.ResponseWriter, r *http.Request) {
	httputil.WriteJSON(w, http.StatusOK, model.SystemLayers)
}

// PatchSystemLayerVisibility — admin only.
// PATCH /admin/system-layer-visibility/{key}
// Accepted fields: enabled, default_enabled_for_new_users, default_opacity, label_override.
// Unknown fields → 400. Unknown key → 404.
func (h *Handler) PatchSystemLayerVisibility(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if _, ok := model.SystemLayerKeySet()[key]; !ok {
		httputil.WriteJSON(w, http.StatusNotFound,
			mapLayerErrBody("unknown_key", "no system layer with key: "+key))
		return
	}

	patch, ok := httputil.DecodeJSON[map[string]any](w, r)
	if !ok {
		return
	}
	allowed := map[string]bool{
		"enabled":                      true,
		"default_enabled_for_new_users": true,
		"default_opacity":               true,
		"label_override":                true,
	}
	for k := range patch {
		if !allowed[k] {
			httputil.WriteJSON(w, http.StatusBadRequest,
				mapLayerErrBody("unknown_field", "unknown field: "+k))
			return
		}
	}

	// Validate default_opacity: integer in [0, 100]. JSON numbers decode as
	// float64; reject non-integer values so SMALLINT cast doesn't truncate.
	if v, ok := patch["default_opacity"].(float64); ok {
		if v < 0 || v > 100 || v != math.Trunc(v) {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("invalid_opacity", "default_opacity must be an integer between 0 and 100"))
			return
		}
	}

	err := h.DB.UpdateSystemLayerVisibility(r.Context(), key, patch)
	switch {
	case errors.Is(err, database.ErrNotFound):
		http.NotFound(w, r)
	case err != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
