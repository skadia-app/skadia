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

// GetUserMapLayers returns the authenticated user's custom tile-URL layers.
func (h *Handler) GetUserMapLayers(w http.ResponseWriter, r *http.Request) {
	uid, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	list, err := h.DB.ListUserMapLayers(r.Context(), uid)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, list)
}

// CreateUserMapLayer creates a custom layer for the authenticated user.
// Enforces: per-user row limit, URL validation with private-IP block, attribution sanitization.
func (h *Handler) CreateUserMapLayer(w http.ResponseWriter, r *http.Request) {
	uid, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	m, ok := httputil.DecodeJSON[model.UserMapLayer](w, r)
	if !ok {
		return
	}
	m.UserID = uid // always set from context — never trust the body

	if m.Name == "" {
		httputil.WriteJSON(w, http.StatusBadRequest,
			mapLayerErrBody("name_required", "name must not be empty"))
		return
	}
	// blockPrivateIPs=true: user-submitted URLs must not target internal services.
	if err := model.ValidateURLTemplate(m.URLTemplate, true); err != nil {
		code := "invalid_url"
		// Propagate the specific error code so the client can show the right message.
		if errors.Is(err, model.ErrPrivateIPBlocked) || errors.Is(err, model.ErrIPv6ULABlocked) {
			code = "private_address_blocked"
		}
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, err.Error()))
		return
	}
	if m.DefaultOpacity < 0 || m.DefaultOpacity > 100 {
		httputil.WriteJSON(w, http.StatusUnprocessableEntity,
			mapLayerErrBody("invalid_opacity", "default_opacity must be between 0 and 100"))
		return
	}
	if m.Attribution != nil {
		s := model.SanitizeAttribution(*m.Attribution)
		m.Attribution = &s
	}

	limit, err := h.Settings.GetInt("library.user_map_layer_limit")
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "settings error", r.URL.Path)
		return
	}

	insertErr := h.DB.InsertUserMapLayer(r.Context(), &m, limit)
	switch {
	case errors.Is(insertErr, database.ErrLimitReached):
		httputil.WriteJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":   "limit_reached",
			"message": "Maximum custom layers per user reached",
			"limit":   limit,
		})
	case errors.Is(insertErr, database.ErrDuplicateName):
		httputil.WriteJSON(w, http.StatusConflict,
			mapLayerErrBody("duplicate_name", "you already have a layer with this name"))
	case insertErr != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		httputil.WriteJSON(w, http.StatusCreated, m)
	}
}

// PatchUserMapLayer patches a layer owned by the authenticated user.
// User isolation is enforced at the DB layer via WHERE user_id=$1 AND id=$2.
func (h *Handler) PatchUserMapLayer(w http.ResponseWriter, r *http.Request) {
	uid, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid id", r.URL.Path)
		return
	}
	patch, ok := httputil.DecodeJSON[map[string]any](w, r)
	if !ok {
		return
	}

	// Reject unknown fields before DB layer (which panics on unknown keys).
	for k := range patch {
		if !userMapLayerPatchFieldAllowed(k) {
			httputil.WriteJSON(w, http.StatusBadRequest, mapLayerErrBody("unknown_field", "unknown patch field: "+k))
			return
		}
	}

	// Validate url_template if present (private-IP block applies to user layers).
	if v, ok := patch["url_template"].(string); ok {
		if err := model.ValidateURLTemplate(v, true); err != nil {
			code := "invalid_url"
			if errors.Is(err, model.ErrPrivateIPBlocked) || errors.Is(err, model.ErrIPv6ULABlocked) {
				code = "private_address_blocked"
			}
			httputil.WriteJSON(w, http.StatusUnprocessableEntity, mapLayerErrBody(code, err.Error()))
			return
		}
	}
	if v, ok := patch["attribution"].(string); ok {
		patch["attribution"] = model.SanitizeAttribution(v)
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

	patchErr := h.DB.UpdateUserMapLayer(r.Context(), uid, id, patch)
	switch {
	case errors.Is(patchErr, database.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(patchErr, database.ErrDuplicateName):
		httputil.WriteJSON(w, http.StatusConflict,
			mapLayerErrBody("duplicate_name", "you already have a layer with this name"))
	case patchErr != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

// DeleteUserMapLayer deletes a layer owned by the authenticated user.
// User isolation enforced at DB layer via WHERE user_id=$1 AND id=$2.
func (h *Handler) DeleteUserMapLayer(w http.ResponseWriter, r *http.Request) {
	uid, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid id", r.URL.Path)
		return
	}
	delErr := h.DB.DeleteUserMapLayer(r.Context(), uid, id)
	switch {
	case errors.Is(delErr, database.ErrNotFound):
		http.NotFound(w, r)
	case delErr != nil:
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "database error", r.URL.Path)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

// BulkSortUserMapLayers reorders the authenticated user's layers.
// Retries up to 3× on SERIALIZABLE serialization failure (SQLSTATE 40001).
func (h *Handler) BulkSortUserMapLayers(w http.ResponseWriter, r *http.Request) {
	uid, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	items, ok := httputil.DecodeJSON[[]database.SortItem](w, r)
	if !ok {
		return
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		err := h.DB.BulkSortUserMapLayers(r.Context(), uid, items)
		if err == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if errors.Is(err, database.ErrBulkSortItemNotFound) {
			httputil.WriteJSON(w, http.StatusUnprocessableEntity,
				mapLayerErrBody("item_not_found", "one or more layer IDs not found or not owned by you"))
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

// userMapLayerPatchFieldAllowed returns true for fields accepted in a user-layer PATCH.
func userMapLayerPatchFieldAllowed(field string) bool {
	switch field {
	case "name", "url_template", "attribution",
		"min_zoom", "max_zoom", "default_opacity",
		"enabled", "sort_order":
		return true
	}
	return false
}

