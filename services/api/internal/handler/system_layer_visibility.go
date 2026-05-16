package handler

import (
	"errors"
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
	type systemLayerDescriptorResponse struct {
		model.SystemLayerDescriptor
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	}
	resp := make([]systemLayerDescriptorResponse, 0, len(model.SystemLayers))
	for _, layer := range model.SystemLayers {
		item := systemLayerDescriptorResponse{SystemLayerDescriptor: layer, Available: true}
		if layer.RequiredCapability != "" {
			if reason := h.featureUnavailableReasonWithContext(r.Context(), layer.RequiredCapability); reason != "" {
				item.Available = false
				item.Reason = reason
			}
		}
		resp = append(resp, item)
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
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
		"enabled":                       true,
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

	// Validate default_opacity via the shared helper so the rules don't drift
	// between system-visibility and map-layer handlers.
	if v, ok := patch["default_opacity"].(float64); ok {
		if code, msg := validateOpacityFloat(v); code != "" {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, msg))
			return
		}
	}
	for _, layer := range model.SystemLayers {
		if layer.Key != key || layer.RequiredCapability == "" {
			continue
		}
		if reason := h.featureNotConfiguredReasonWithContext(r.Context(), layer.RequiredCapability); reason != "" {
			writeFeatureUnavailable(w, r, layer.RequiredCapability, reason)
			return
		}
		if enablesUnavailableSystemLayer(patch) {
			if reason := h.featureUnavailableReasonWithContext(r.Context(), layer.RequiredCapability); reason != "" {
				writeFeatureUnavailable(w, r, layer.RequiredCapability, reason)
				return
			}
		}
		break
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

func enablesUnavailableSystemLayer(patch map[string]any) bool {
	for _, field := range []string{"enabled", "default_enabled_for_new_users"} {
		v, ok := patch[field]
		if !ok {
			continue
		}
		enabled, ok := v.(bool)
		if ok && enabled {
			return true
		}
	}
	return false
}
