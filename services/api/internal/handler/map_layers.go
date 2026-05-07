package handler

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
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
	m.IsSystem = true             // admin endpoint always creates system layers
	m.ProxyThroughBackend = false // not implemented in Phase 1
	m.CreatedAt = time.Time{}
	m.UpdatedAt = time.Time{}

	if err := validateMapLayerInput(&m); err != nil {
		// Map specific sentinel errors / well-known prefixes to stable codes
		// so the frontend can surface targeted messages. All validation
		// failures use the structured 422 envelope (no RFC7807 here).
		code := "invalid_input"
		switch {
		case errors.Is(err, model.ErrStyleURLTooLong):
			code = "style_url_too_long"
		case errors.Is(err, model.ErrStyleURLEmpty), errors.Is(err, model.ErrStyleURLInvalid):
			code = "invalid_style_url"
		case strings.HasPrefix(err.Error(), "name_required"):
			code = "name_required"
		case strings.HasPrefix(err.Error(), "name_too_long"):
			code = "name_too_long"
		case strings.HasPrefix(err.Error(), "invalid_zoom_range"):
			code = "invalid_zoom_range"
		case strings.HasPrefix(err.Error(), "invalid_sort_order"):
			code = "invalid_sort_order"
		case strings.HasPrefix(err.Error(), "requires_capability"):
			code = "invalid_capability"
		default:
			code = mapLayerAddressValidationErrorCode(err, code)
		}
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, err.Error()))
		return
	}
	if m.Attribution != nil {
		s := model.SanitizeAttribution(*m.Attribution)
		m.Attribution = &s
	}
	// Auto-generate a key from the name when the client omits it. Slugify to
	// lowercase ASCII tokens joined by hyphens, capped at 32 runes. On a
	// unique-key collision retry with "-2" … "-10" before giving up.
	if m.Key == "" {
		m.Key = slugifyName(m.Name)
	}
	if err := h.DB.InsertMapLayer(r.Context(), &m); err != nil {
		if errors.Is(err, database.ErrDuplicateName) {
			if m.Key != "" {
				// Attempt suffix disambiguation before surfacing a collision error.
				base := m.Key
				succeeded := false
				for suffix := 2; suffix <= 10; suffix++ {
					m.Key = fmt.Sprintf("%s-%d", base, suffix)
					if err2 := h.DB.InsertMapLayer(r.Context(), &m); err2 == nil {
						succeeded = true
						break
					} else if !errors.Is(err2, database.ErrDuplicateName) {
						httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
						return
					}
				}
				if succeeded {
					httputil.WriteJSON(w, http.StatusCreated, m)
					return
				}
			}
			httputil.WriteJSON(w, http.StatusConflict, mapLayerErrBody("key_collision", "could not generate a unique key; supply an explicit key"))
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
	// (which would return 500 instead of a meaningful 422). JSON numbers all
	// decode as float64; reject non-integer values (e.g. 42.7) up-front because
	// SMALLINT cast would silently truncate.
	if v, ok := patch["default_opacity"].(float64); ok {
		if code, msg := validateOpacityFloat(v); code != "" {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, msg))
			return
		}
	}
	// Validate name length when present.
	if v, ok := patch["name"].(string); ok {
		trimmed, code, msg := validateName(v)
		if code != "" {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, msg))
			return
		}
		patch["name"] = trimmed
	}
	// Validate min_zoom / max_zoom / sort_order ranges (clamps to MapLibre's 0..24
	// for zoom; bounded INTEGER range for sort_order).
	if code, msg := validateZoomAndSort(patch); code != "" {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, msg))
		return
	}

	// Silently drop identity fields that must never change. The frontend replays
	// the full object on edit, so these may arrive in the payload without intent.
	// Dropping them (rather than 422-ing) prevents spurious "field_immutable"
	// errors when an admin edits only min_zoom on an existing system layer.
	delete(patch, "key")
	delete(patch, "source_kind")

	// Sanitize attribution in-place.
	if v, ok := patch["attribution"].(string); ok {
		patch["attribution"] = model.SanitizeAttribution(v)
	}
	// Validate URL template if present.
	if v, ok := patch["url_template"].(string); ok {
		// Admin tier: SSRF-block enabled. Admins should not save system layers
		// pointing at internal addresses (localhost, postgres, RFC1918) because
		// the URL is then served to every user via GET /map-layers — the seeded
		// `local` system layer uses a relative `/api/v1/...` path, so legitimate
		// internal references don't need IP literals.
		if err := model.ValidateURLTemplate(v, true); err != nil {
			code := mapLayerAddressValidationErrorCode(err, "invalid_url")
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, err.Error()))
			return
		}
	}
	// Validate style_url shape when present (length, scheme, same-origin /api/v1/
	// relative, no private-IP literals for absolute URLs).
	if v, ok := patch["style_url"].(string); ok {
		// Admin tier: SSRF-block enabled (see url_template comment above).
		if err := model.ValidateStyleURL(v, true); err != nil {
			code := "invalid_style_url"
			switch {
			case errors.Is(err, model.ErrStyleURLTooLong):
				code = "style_url_too_long"
			default:
				code = mapLayerAddressValidationErrorCode(err, code)
			}
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, err.Error()))
			return
		}
	}
	// Validate requires_capability against the known capability key set so an
	// admin typo doesn't permanently capability-block a layer. Empty string and
	// JSON null are treated as "no requirement" (clearing). Other types must
	// be rejected explicitly — otherwise a `false` / number / object would slip
	// past the type-assertion and get silently dropped by the field allow-list.
	if raw, present := patch["requires_capability"]; present {
		switch v := raw.(type) {
		case nil:
			// null → clear the requirement.
		case string:
			if v != "" && !capabilities.ValidKey(v) {
				httputil.WriteJSON(w, http.StatusUnprocessableEntity,
					mapLayerErrBody("invalid_capability",
						"requires_capability must be one of: "+strings.Join(capabilities.ValidKeysSorted(), ", ")))
				return
			}
		default:
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("invalid_capability_type",
					"requires_capability must be a string or null"))
			return
		}
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
		httputil.WriteJSON(w, http.StatusNotFound, mapLayerErrBody("not_found", "map layer not found"))
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
		w.WriteHeader(http.StatusNoContent)
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
		httputil.WriteJSON(w, http.StatusNotFound, mapLayerErrBody("not_found", "map layer not found"))
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
	if len(items) == 0 {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity,
			mapLayerErrBody("items_required", "at least one item is required"))
		return
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		// Honor client cancellation: if the context is already done, stop
		// retrying so we don't return serialization_retry_exhausted on a
		// request the caller has abandoned.
		if ctxErr := r.Context().Err(); ctxErr != nil {
			return
		}
		err := h.DB.BulkSortMapLayers(r.Context(), items)
		if err == nil {
			w.WriteHeader(http.StatusNoContent)
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

// Field bounds applied uniformly to admin + user map layers.
const (
	mapLayerNameMaxLen     = 200
	mapLayerMinZoom        = 0
	mapLayerMaxZoom        = 24
	mapLayerSortOrderLimit = 2_000_000_000
)

func mapLayerErrBody(code, msg string) map[string]any {
	return map[string]any{"error": code, "message": msg}
}

func mapLayerAddressValidationErrorCode(err error, fallback string) string {
	switch {
	case errors.Is(err, model.ErrPrivateIPBlocked), errors.Is(err, model.ErrIPv6ULABlocked),
		errors.Is(err, model.ErrBlockedHostname):
		return "private_address_blocked"
	case errors.Is(err, model.ErrHostResolutionFailed):
		return "host_resolution_failed"
	default:
		return fallback
	}
}

// validateName enforces non-empty + max length on a layer name. Pass the trimmed
// string back via the *string so the caller can persist the cleaned value.
func validateName(name string) (string, string, string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", "name_required", "name must not be empty"
	}
	// Length is checked in characters (runes), not bytes, to match the DB
	// CHECK constraint (Postgres length() counts characters). Without this,
	// multibyte names (Cyrillic, CJK, emoji) ≤ 200 chars but > 200 bytes would
	// be rejected here while the DB would accept them.
	if utf8.RuneCountInString(name) > mapLayerNameMaxLen {
		return "", "name_too_long", fmt.Sprintf("name must be at most %d characters", mapLayerNameMaxLen)
	}
	return trimmed, "", ""
}

// validateOpacityFloat checks a JSON-decoded opacity value: integer in [0,100].
// Returns ("", "") on success; otherwise (errorCode, errorMessage).
func validateOpacityFloat(v float64) (string, string) {
	if v < 0 || v > 100 || v != math.Trunc(v) {
		return "invalid_opacity", "default_opacity must be an integer between 0 and 100"
	}
	return "", ""
}

// validateZoomAndSort validates min_zoom, max_zoom, and sort_order from a JSON
// patch / create map. Each field is optional; missing keys are skipped. The
// helper enforces 0..24 zoom range, min<=max, and a generous sort_order bound to
// avoid raw 500s from the INTEGER column overflow.
func validateZoomAndSort(p map[string]any) (string, string) {
	var (
		minZ, maxZ     *int64
		minSet, maxSet bool
	)
	if v, ok := p["min_zoom"]; ok && v != nil {
		f, fok := v.(float64)
		if !fok || f != math.Trunc(f) {
			return "invalid_zoom_range", "min_zoom must be an integer"
		}
		if f < mapLayerMinZoom || f > mapLayerMaxZoom {
			return "invalid_zoom_range",
				fmt.Sprintf("min_zoom must be between %d and %d", mapLayerMinZoom, mapLayerMaxZoom)
		}
		i := int64(f)
		minZ = &i
		minSet = true
	}
	if v, ok := p["max_zoom"]; ok && v != nil {
		f, fok := v.(float64)
		if !fok || f != math.Trunc(f) {
			return "invalid_zoom_range", "max_zoom must be an integer"
		}
		if f < mapLayerMinZoom || f > mapLayerMaxZoom {
			return "invalid_zoom_range",
				fmt.Sprintf("max_zoom must be between %d and %d", mapLayerMinZoom, mapLayerMaxZoom)
		}
		i := int64(f)
		maxZ = &i
		maxSet = true
	}
	if minSet && maxSet && *minZ > *maxZ {
		return "invalid_zoom_range", "min_zoom must be <= max_zoom"
	}
	if v, ok := p["sort_order"]; ok && v != nil {
		f, fok := v.(float64)
		if !fok || f != math.Trunc(f) {
			return "invalid_sort_order", "sort_order must be an integer"
		}
		if f < -mapLayerSortOrderLimit || f > mapLayerSortOrderLimit {
			return "invalid_sort_order",
				fmt.Sprintf("sort_order must be between %d and %d", -mapLayerSortOrderLimit, mapLayerSortOrderLimit)
		}
	}
	return "", ""
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
	trimmed, code, msg := validateName(m.Name)
	if code != "" {
		// Caller maps the leading token of the error to a stable code.
		return errors.New(code + ": " + msg)
	}
	m.Name = trimmed
	if m.Role != "base" && m.Role != "overlay" {
		return errors.New("role must be 'base' or 'overlay'")
	}
	if m.DefaultOpacity < 0 || m.DefaultOpacity > 100 {
		return errors.New("default_opacity must be between 0 and 100")
	}
	if m.MinZoom != nil && (*m.MinZoom < mapLayerMinZoom || *m.MinZoom > mapLayerMaxZoom) {
		return fmt.Errorf("invalid_zoom_range: min_zoom must be between %d and %d", mapLayerMinZoom, mapLayerMaxZoom)
	}
	if m.MaxZoom != nil && (*m.MaxZoom < mapLayerMinZoom || *m.MaxZoom > mapLayerMaxZoom) {
		return fmt.Errorf("invalid_zoom_range: max_zoom must be between %d and %d", mapLayerMinZoom, mapLayerMaxZoom)
	}
	if m.MinZoom != nil && m.MaxZoom != nil && *m.MinZoom > *m.MaxZoom {
		return errors.New("invalid_zoom_range: min_zoom must be <= max_zoom")
	}
	if m.SortOrder < -mapLayerSortOrderLimit || m.SortOrder > mapLayerSortOrderLimit {
		return fmt.Errorf("invalid_sort_order: sort_order must be between %d and %d", -mapLayerSortOrderLimit, mapLayerSortOrderLimit)
	}
	switch m.SourceKind {
	case "raster_tiles":
		if m.URLTemplate == nil {
			return errors.New("url_template required for raster_tiles")
		}
		// Admin tier: SSRF-block enabled — system layers must not reference
		// internal addresses; relative /api/v1/ paths are still accepted by
		// the validators for legitimate internal endpoints.
		if err := model.ValidateURLTemplate(*m.URLTemplate, true); err != nil {
			return err
		}
	case "vector_style":
		if m.StyleURL == nil {
			return errors.New("style_url required for vector_style")
		}
		if err := model.ValidateStyleURL(*m.StyleURL, true); err != nil {
			return err
		}
	case "protomaps_offline":
		// style_url is optional for offline layers; validate shape if present.
		if m.StyleURL != nil && *m.StyleURL != "" {
			if err := model.ValidateStyleURL(*m.StyleURL, true); err != nil {
				return err
			}
		}
	default:
		return errors.New("source_kind must be 'raster_tiles', 'vector_style', or 'protomaps_offline'")
	}
	if m.RequiresCapability != nil && *m.RequiresCapability != "" {
		if !capabilities.ValidKey(*m.RequiresCapability) {
			return errors.New("requires_capability must be one of: " +
				strings.Join(capabilities.ValidKeysSorted(), ", "))
		}
	}
	return nil
}

// slugifyName converts a display name to a URL-safe key suitable for use as a
// MapLibre source ID. Rules: fold to ASCII-only lowercase, collect [a-z0-9]
// token runs, join with hyphens, cap at 32 runes. An empty input (or one that
// has no ASCII alphanumeric chars) returns "layer".
func slugifyName(name string) string {
	const maxRunes = 32
	// Decompose Unicode and fold to ASCII-range equivalents. We iterate rune by
	// rune: keep [a-z0-9], convert [A-Z] to lowercase, treat everything else as
	// a word boundary.
	var tokens []string
	var cur strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			cur.WriteRune(r)
		} else if r >= 'A' && r <= 'Z' {
			cur.WriteRune(unicode.ToLower(r))
		} else {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	if len(tokens) == 0 {
		return "layer"
	}
	slug := strings.Join(tokens, "-")
	// Truncate to maxRunes. Cut at a hyphen boundary if possible so the result
	// doesn't end in a partial token.
	if utf8.RuneCountInString(slug) > maxRunes {
		runes := []rune(slug)[:maxRunes]
		slug = string(runes)
		// Trim a trailing partial token (anything after the last hyphen).
		if idx := strings.LastIndexByte(slug, '-'); idx > 0 {
			slug = slug[:idx]
		}
	}
	return slug
}
