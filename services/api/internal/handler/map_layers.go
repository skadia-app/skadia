package handler

import (
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// GetMapLayers — public (any authenticated user).
// Returns only enabled layers so users never see disabled URLs.
// Returns { layers: [...], system_visibility: { key: {...}, ... } }.
func (h *Handler) GetMapLayers(w http.ResponseWriter, r *http.Request) {
	h.getMapLayersInternal(w, r, false)
}

// GetAdminMapLayers — admin only.
// Returns ALL layers including disabled ones for management purposes.
// Returns { layers: [...], system_visibility: { key: {...}, ... } }.
func (h *Handler) GetAdminMapLayers(w http.ResponseWriter, r *http.Request) {
	h.getMapLayersInternal(w, r, true)
}

func (h *Handler) getMapLayersInternal(w http.ResponseWriter, r *http.Request, includeDisabled bool) {
	layers, err := h.DB.ListMapLayers(r.Context(), includeDisabled)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	vis, err := h.DB.ListSystemLayerVisibility(r.Context())
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	visMap := make(map[string]model.SystemLayerVisibility, len(vis))
	for _, v := range vis {
		visMap[v.LayerKey] = v
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"layers":            layers,
		"system_visibility": visMap,
	})
}

// CreateMapLayer — admin only.
// Decodes a MapLayer from the request body, enforces server-controlled fields,
// validates source-kind-specific constraints, sanitizes attribution, then inserts.
func (h *Handler) CreateMapLayer(w http.ResponseWriter, r *http.Request) {
	m, ok := httputil.DecodeJSON[model.MapLayer](w, r)
	if !ok {
		return
	}
	// Zero out server-controlled fields — callers must not influence these.
	m.ID = 0
	m.IsSystem = false
	m.ProxyThroughBackend = false // not implemented in Phase 1
	m.CreatedAt = time.Time{}
	m.UpdatedAt = time.Time{}

	if err := validateMapLayerInput(&m); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if m.Attribution != nil {
		s := model.SanitizeAttribution(*m.Attribution)
		m.Attribution = &s
	}
	if err := h.DB.InsertMapLayer(r.Context(), &m); err != nil {
		if errors.Is(err, database.ErrDuplicateName) {
			httputil.WriteJSON(w, http.StatusConflict, mapLayerErrBody("duplicate_key", "a layer with this key already exists"))
			return
		}
		if errors.Is(err, database.ErrURLSourceKindMismatch) {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("url_source_kind_mismatch", "url_template/style_url must match source_kind: raster_tiles needs url_template, vector_style needs style_url, protomaps_offline needs neither"))
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, m)
}

// PatchMapLayer — admin only. Honors X-If-Match-Updated-At for concurrent-edit detection.
// For url_template / style_url changes the header is REQUIRED (428 otherwise).
func (h *Handler) PatchMapLayer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid id", r.URL.Path)
		return
	}
	patch, ok := httputil.DecodeJSON[map[string]any](w, r)
	if !ok {
		return
	}

	// Validate default_opacity range before reaching the DB CHECK constraint
	// (which would return 500 instead of a meaningful 422).
	if v, ok := patch["default_opacity"].(float64); ok {
		if v < 0 || v > 100 {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("invalid_opacity", "default_opacity must be between 0 and 100"))
			return
		}
	}

	// Reject attempts to mutate immutable identity fields. Changing key would
	// invalidate any client LocalStorage entries keyed on it; changing source_kind
	// would silently corrupt the layer's URL / style references.
	if containsKey(patch, "key") || containsKey(patch, "source_kind") {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity,
			mapLayerErrBody("field_immutable", "key and source_kind cannot be changed after creation"))
		return
	}

	// Sanitize attribution in-place.
	if v, ok := patch["attribution"].(string); ok {
		patch["attribution"] = model.SanitizeAttribution(v)
	}
	// Validate URL template if present.
	if v, ok := patch["url_template"].(string); ok {
		if err := model.ValidateURLTemplate(v, false); err != nil {
			httputil.WriteJSON(w, http.StatusBadRequest, mapLayerErrBody("invalid_url", err.Error()))
			return
		}
	}
	// Validate style_url is non-empty when present.
	if v, ok := patch["style_url"].(string); ok && v == "" {
		httputil.WriteJSON(w, http.StatusBadRequest, mapLayerErrBody("invalid_style_url", "style_url must not be empty"))
		return
	}

	// Custom header — see plan §Concurrent Edit. RFC 7232 If-Match requires ETag semantics
	// that intermediaries can reformat; an unquoted RFC3339Nano timestamp is unambiguous here.
	var ifMatch *time.Time
	if hv := r.Header.Get("X-If-Match-Updated-At"); hv != "" {
		t, err := time.Parse(time.RFC3339Nano, hv)
		if err != nil {
			httputil.WriteJSON(w, http.StatusBadRequest,
				mapLayerErrBody("bad_if_match", "X-If-Match-Updated-At must be RFC3339Nano"))
			return
		}
		ifMatch = &t
	}
	// url_template / style_url changes require the header so two admins don't clobber each other.
	if (containsKey(patch, "url_template") || containsKey(patch, "style_url")) && ifMatch == nil {
		httputil.WriteJSON(w, http.StatusPreconditionRequired,
			mapLayerErrBody("if_match_required", "PATCH on url_template/style_url requires X-If-Match-Updated-At header"))
		return
	}
	// Reject unknown / unsafe fields before passing to DB layer (which panics on unknown keys).
	for k := range patch {
		if !mapLayerPatchFieldAllowed(k) {
			httputil.WriteJSON(w, http.StatusBadRequest, mapLayerErrBody("unknown_field", "unknown patch field: "+k))
			return
		}
	}

	patchErr := h.DB.UpdateMapLayer(r.Context(), id, patch, ifMatch)
	switch {
	case errors.Is(patchErr, database.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(patchErr, database.ErrDuplicateName):
		httputil.WriteJSON(w, http.StatusConflict, mapLayerErrBody("duplicate_key", "a layer with this key already exists"))
	case errors.Is(patchErr, database.ErrURLSourceKindMismatch):
		httputil.WriteJSON(w, http.StatusUnprocessableEntity,
			mapLayerErrBody("url_source_kind_mismatch", "url_template/style_url must match source_kind: raster_tiles needs url_template, vector_style needs style_url, protomaps_offline needs neither"))
	case errors.Is(patchErr, database.ErrSystemLayerImmutable):
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "system_layer_field_immutable",
			"message": "Cannot modify protected fields on a system layer",
			"fields":  blockedFields(patch),
		})
	case errors.Is(patchErr, database.ErrStaleUpdate):
		httputil.WriteJSON(w, http.StatusPreconditionFailed,
			mapLayerErrBody("stale", "row updated by another session; refetch and retry"))
	case patchErr != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

// DeleteMapLayer — admin only.
func (h *Handler) DeleteMapLayer(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid id", r.URL.Path)
		return
	}
	delErr := h.DB.DeleteMapLayer(r.Context(), id)
	switch {
	case errors.Is(delErr, database.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(delErr, database.ErrSystemLayerImmutable):
		httputil.WriteJSON(w, http.StatusConflict,
			mapLayerErrBody("system_layer_immutable", "system layers cannot be deleted; toggle 'enabled' instead"))
	case delErr != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// BulkSortMapLayers — admin only. Body: [{id, sort_order}, ...].
// Retries up to 3× on serialization failure (Postgres SQLSTATE 40001) because
// BulkSortMapLayers runs under SERIALIZABLE isolation.
func (h *Handler) BulkSortMapLayers(w http.ResponseWriter, r *http.Request) {
	items, ok := httputil.DecodeJSON[[]database.SortItem](w, r)
	if !ok {
		return
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		err := h.DB.BulkSortMapLayers(r.Context(), items)
		if err == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, database.ErrBulkSortItemNotFound) {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("item_not_found", "one or more layer IDs not found"))
			return
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "40001" {
			lastErr = err
			// Jittered backoff: 0–19 ms between retries to spread concurrent conflicts.
			time.Sleep(time.Duration(rand.IntN(20)) * time.Millisecond)
			continue
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", err.Error(), r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusConflict,
		mapLayerErrBody("serialization_retry_exhausted", lastErr.Error()))
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mapLayerErrBody(code, msg string) map[string]any {
	return map[string]any{"error": code, "message": msg}
}

func containsKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

func blockedFields(patch map[string]any) []string {
	var out []string
	for k := range patch {
		if _, blocked := model.SystemLayerImmutableFields[k]; blocked {
			out = append(out, k)
		}
	}
	return out
}

// mapLayerPatchFieldAllowed returns true for fields the handler accepts in a PATCH.
// key and source_kind are not listed here — they are rejected earlier with a specific
// field_immutable error and must never reach the DB layer.
func mapLayerPatchFieldAllowed(field string) bool {
	switch field {
	case "name", "enabled", "sort_order", "default_opacity",
		"attribution", "url_template", "style_url",
		"min_zoom", "max_zoom", "requires_capability":
		return true
	}
	return false
}

// validateMapLayerInput enforces required fields per source_kind and basic role validation.
func validateMapLayerInput(m *model.MapLayer) error {
	if m.Name == "" {
		return errors.New("name required")
	}
	if m.Role != "base" && m.Role != "overlay" {
		return errors.New("role must be 'base' or 'overlay'")
	}
	if m.DefaultOpacity < 0 || m.DefaultOpacity > 100 {
		return errors.New("default_opacity must be between 0 and 100")
	}
	switch m.SourceKind {
	case "raster_tiles":
		if m.URLTemplate == nil {
			return errors.New("url_template required for raster_tiles")
		}
		if err := model.ValidateURLTemplate(*m.URLTemplate, false); err != nil {
			return err
		}
	case "vector_style":
		if m.StyleURL == nil || *m.StyleURL == "" {
			return errors.New("style_url required for vector_style")
		}
	case "protomaps_offline":
		// Both fields are nullable for offline layers.
	default:
		return errors.New("source_kind must be 'raster_tiles', 'vector_style', or 'protomaps_offline'")
	}
	return nil
}
