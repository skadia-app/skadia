package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// routeETag formats a planned-route version as a strong RFC 7232 ETag.
// Strong validators are correct here because the body is a deterministic
// projection of the row state — equal version implies byte-equal payload.
func routeETag(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

// parseIfMatchResult is the outcome of parseIfMatchVersion.
type parseIfMatchResult int

const (
	ifMatchAbsent    parseIfMatchResult = iota // header not sent or wildcard — use legacy path
	ifMatchValid                               // header present and parseable — use concurrency check
	ifMatchMalformed                           // header present but unparseable — caller must return 400
)

// parseIfMatchVersion extracts a numeric version from an If-Match header.
//
// Returns:
//   - ifMatchAbsent   (0)  when the header is missing or the wildcard "*".
//     Callers fall back to the legacy unconditional-update path.
//   - ifMatchValid    (n)  when the header carries a single strong ("N") or
//     weak (W/"N") ETag with a positive decimal integer. Callers use n for
//     the optimistic-concurrency check.
//   - ifMatchMalformed (0) when the header is present but does not conform to
//     the above grammar (no quotes, non-numeric payload, multiple tags, …).
//     Callers must reject the request with 400.
//
// OWS (optional whitespace) around the header value is stripped per RFC 7230.
// Multi-tag values (comma-separated) are rejected as malformed because this
// server sets exactly one ETag per resource and cannot meaningfully evaluate
// a tag list.
func parseIfMatchVersion(raw string) (int64, parseIfMatchResult) {
	v := strings.TrimSpace(raw)
	if v == "" || v == "*" {
		// Absent header or wildcard: both treated as "no version constraint".
		// Wildcard maps to legacy rather than unconditional-match-success
		// because a buggy proxy injecting "*" would otherwise silently disable
		// the concurrency guard.
		return 0, ifMatchAbsent
	}

	// Reject multi-tag values (e.g. `"1", "2"`) — we set exactly one ETag.
	if strings.Contains(v, ",") {
		return 0, ifMatchMalformed
	}

	// Accept both strong `"N"` and weak `W/"N"` forms.
	v = strings.TrimPrefix(v, "W/")

	// The value must now be a quoted string.
	if !strings.HasPrefix(v, `"`) || !strings.HasSuffix(v, `"`) || len(v) < 2 {
		return 0, ifMatchMalformed
	}
	inner := v[1 : len(v)-1]
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return 0, ifMatchMalformed
	}

	n, err := strconv.ParseInt(inner, 10, 64)
	if err != nil || n <= 0 {
		return 0, ifMatchMalformed
	}
	return n, ifMatchValid
}

// ListRoutes returns paginated planned routes for the authenticated user.
// GET /api/v1/routes
func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit, offset := httputil.ParsePagination(r, 50, 200)
	predicates, err := sport.PredicatesFromFilterTokens(r.URL.Query().Get("sport_predicates"))
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	filters := database.RouteListFilters{
		SportType:       r.URL.Query().Get("sport_type"),
		SubType:         r.URL.Query().Get("sub_type"),
		Predicates:      predicates,
		ModifierFilters: parseRouteModifierFilters(r),
		Search:          r.URL.Query().Get("search"),
		Sort:            r.URL.Query().Get("sort"),
	}

	routes, err := h.DB.ListRoutes(r.Context(), userID, limit, offset, filters)
	if err != nil {
		slog.Error("list routes failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list routes", r.URL.Path)
		return
	}

	total, err := h.DB.CountRoutes(r.Context(), userID, filters)
	if err != nil {
		slog.Error("count routes failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to count routes", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"routes": routes,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// CreateRoute creates a new planned route for the authenticated user.
// POST /api/v1/routes
func (h *Handler) CreateRoute(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	route, ok := httputil.DecodeJSON[model.PlannedRoute](w, r)
	if !ok {
		return
	}

	route.UserID = userID
	if err := normalizeRouteSportForRequest(&route); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	id, err := h.DB.CreateRoute(r.Context(), &route)
	if err != nil {
		slog.Error("create route failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create route", r.URL.Path)
		return
	}

	route.ID = id
	w.Header().Set("ETag", routeETag(route.Version))
	httputil.WriteJSON(w, http.StatusCreated, route)
}

// GetRoute fetches a single planned route by ID, enforcing ownership.
// GET /api/v1/routes/{id}
func (h *Handler) GetRoute(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing route id", r.URL.Path)
		return
	}

	// GetRoute already enforces ownership via WHERE user_id = $2
	route, err := h.DB.GetRoute(r.Context(), id, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", id, "user_id", userID) {
		return
	}

	w.Header().Set("ETag", routeETag(route.Version))
	httputil.WriteJSON(w, http.StatusOK, route)
}

// patchableRouteFields lists the JSON keys the client is allowed to PATCH.
// Keys outside this list (id, user_id, created_at, updated_at, source_*,
// geometry_hash) are silently ignored to prevent privilege-escalation and to
// keep server-controlled fields authoritative.
var patchableRouteFields = map[string]struct{}{
	"name":                 {},
	"description":          {},
	"sport_type":           {},
	"sub_type":             {},
	"is_electric":          {},
	"is_indoor":            {},
	"is_virtual":           {},
	"is_commute":           {},
	"difficulty":           {},
	"profile":              {},
	"geometry_geojson":     {},
	"waypoints":            {},
	"segments":             {},
	"total_distance_m":     {},
	"total_elevation_gain": {},
	"total_elevation_loss": {},
	"estimated_time_min":   {},
	"time_override":        {},
	"auto_tag":             {},
	"elevation_profile":    {},
	"enrichment":           {},
	"routing_engine":       {},
}

// applyRoutePatch merges JSON-patch fields into an existing route.
// Only keys listed in patchableRouteFields are applied; unknown or immutable
// keys are ignored. Keys present with an explicit null clear nullable fields
// (e.g. description → NULL); keys absent from the body preserve the DB value.
func applyRoutePatch(route *model.PlannedRoute, fields map[string]json.RawMessage) error {
	for key, raw := range fields {
		if _, ok := patchableRouteFields[key]; !ok {
			continue
		}
		var target any
		switch key {
		case "name":
			target = &route.Name
		case "description":
			target = &route.Description
		case "sport_type":
			target = &route.SportType
		case "sub_type":
			target = &route.SubType
		case "is_electric":
			target = &route.IsElectric
		case "is_indoor":
			target = &route.IsIndoor
		case "is_virtual":
			target = &route.IsVirtual
		case "is_commute":
			target = &route.IsCommute
		case "difficulty":
			target = &route.Difficulty
		case "profile":
			target = &route.Profile
		case "geometry_geojson":
			target = &route.GeometryGeoJSON
		case "waypoints":
			target = &route.Waypoints
		case "segments":
			target = &route.Segments
		case "total_distance_m":
			target = &route.TotalDistanceM
		case "total_elevation_gain":
			target = &route.TotalElevationGain
		case "total_elevation_loss":
			target = &route.TotalElevationLoss
		case "estimated_time_min":
			target = &route.EstimatedTimeMin
		case "time_override":
			target = &route.TimeOverride
		case "auto_tag":
			target = &route.AutoTag
		case "elevation_profile":
			// json.RawMessage: explicit null clears (raw == "null"), any value passes through.
			route.ElevationProfile = append(route.ElevationProfile[:0], raw...)
			continue
		case "enrichment":
			route.Enrichment = append(route.Enrichment[:0], raw...)
			continue
		case "routing_engine":
			target = &route.RoutingEngine
		default:
			continue
		}
		if err := json.Unmarshal(raw, target); err != nil {
			return fmt.Errorf("patch field %q: %w", key, err)
		}
	}
	return nil
}

// UpdateRoute applies a partial update to a planned route, enforcing ownership.
// PATCH /api/v1/routes/{id}
//
// Semantics: fields present in the JSON body overwrite the DB value (including
// explicit null for nullable columns). Fields absent from the body are
// preserved. See patchableRouteFields for the allow-list.
func (h *Handler) UpdateRoute(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing route id", r.URL.Path)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<20)) // 16 MiB cap
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "failed to read body", r.URL.Path)
		return
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	// Fetch existing route (also enforces ownership — returns ErrNoRows for
	// other users or missing rows, mapped to 404 without leaking existence).
	existing, err := h.DB.GetRoute(r.Context(), id, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", id, "user_id", userID) {
		return
	}

	if err := applyRoutePatch(existing, fields); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if err := normalizeRouteSportForRequest(existing); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}

	// IDs are never patchable — reassert from URL/context for UpdateRoute's
	// WHERE id = $1 AND user_id = $2 clause.
	existing.ID = id
	existing.UserID = userID

	// Optimistic-concurrency: parse If-Match.
	//   absent   → legacy silent-overwrite path (backwards compat; emits Warn)
	//   valid    → concurrency check; ErrVersionMismatch → 412
	//   malformed → 400 so that a buggy proxy injecting a broken header does
	//               not silently disable the guard
	expectedVersion, ifMatchState := parseIfMatchVersion(r.Header.Get("If-Match"))
	switch ifMatchState {
	case ifMatchAbsent:
		slog.Warn("PATCH /routes without If-Match header — silent overwrite path",
			"route_id", id, "user_id", userID)
	case ifMatchMalformed:
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid If-Match header", r.URL.Path)
		return
	}

	err = h.DB.UpdateRoute(r.Context(), existing, expectedVersion)
	if errors.Is(err, database.ErrVersionMismatch) {
		// 412 body carries the current resource state so the client can
		// diff/merge or reload without an extra GET roundtrip. Same shape
		// as GET /routes/{id}.
		current, getErr := h.DB.GetRoute(r.Context(), id, userID)
		if getErr != nil {
			// Race: row was deleted between the failed UPDATE and the
			// follow-up GET. Surface as 404 — the entity is genuinely gone.
			httputil.HandleDBErr(w, r, getErr, "route not found", "id", id)
			return
		}
		w.Header().Set("ETag", routeETag(current.Version))
		httputil.WriteJSON(w, http.StatusPreconditionFailed, current)
		return
	}
	if httputil.HandleDBErr(w, r, err, "route not found", "id", id) {
		return
	}

	w.Header().Set("ETag", routeETag(existing.Version))
	httputil.WriteJSON(w, http.StatusOK, existing)
}

func normalizeRouteSportForRequest(route *model.PlannedRoute) error {
	rawSubType := ""
	if route.SubType != nil {
		rawSubType = *route.SubType
	}
	tuple, patch, err := sport.NormalizeSelectableSportTuple(route.SportType, rawSubType)
	if err != nil {
		return err
	}
	route.SportType = tuple.SportType
	if tuple.SubType == "" {
		route.SubType = nil
	} else {
		route.SubType = &tuple.SubType
	}
	if patch.IsElectric != nil && *patch.IsElectric {
		route.IsElectric = true
	}
	if patch.IsIndoor != nil && *patch.IsIndoor {
		route.IsIndoor = true
	}
	return nil
}

func parseRouteModifierFilters(r *http.Request) sport.ModifierFilters {
	query := r.URL.Query()
	parse := func(key string) *bool {
		raw := query.Get(key)
		if raw == "" {
			return nil
		}
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return nil
		}
		return &value
	}
	return sport.ModifierFilters{
		IsElectric: parse("is_electric"),
		IsIndoor:   parse("is_indoor"),
		IsVirtual:  parse("is_virtual"),
		IsCommute:  parse("is_commute"),
	}
}

// DeleteRoute removes a planned route, enforcing ownership.
// DELETE /api/v1/routes/{id}
func (h *Handler) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing route id", r.URL.Path)
		return
	}

	// Clean up route photo files in ShadowFS before the CASCADE delete removes the DB rows.
	h.cleanupRoutePhotoFiles(r.Context(), id, userID)

	// DeleteRoute enforces ownership via WHERE id = $1 AND user_id = $2
	if err := h.DB.DeleteRoute(r.Context(), id, userID); httputil.HandleDBErr(w, r, err, "route not found", "id", id) {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DuplicateRoute creates a copy of a planned route owned by the authenticated user.
// POST /api/v1/routes/{id}/duplicate
func (h *Handler) DuplicateRoute(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "missing route id", r.URL.Path)
		return
	}

	// DuplicateRoute enforces ownership via WHERE id = $1 AND user_id = $2
	newID, err := h.DB.DuplicateRoute(r.Context(), id, userID)
	if httputil.HandleDBErr(w, r, err, "route not found", "id", id, "user_id", userID) {
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": newID})
}

// cleanupRoutePhotoFiles deletes ShadowFS (or legacy S3) files for all photos
// belonging to a route. Must be called BEFORE the route is deleted (CASCADE
// would drop the route_photos rows and lose the file_id references).
func (h *Handler) cleanupRoutePhotoFiles(ctx context.Context, routeID, userID string) {
	if h.ShadowFS != nil {
		rows, err := h.DB.Pool.Query(ctx,
			`SELECT COALESCE(file_id::text, '') FROM route_photos WHERE route_id = $1 AND user_id = $2`,
			routeID, userID,
		)
		if err != nil {
			slog.Warn("route delete: query route_photos file_ids failed", "route_id", routeID, "error", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var fileID string
			if err := rows.Scan(&fileID); err != nil || fileID == "" {
				continue
			}
			if delErr := h.ShadowFS.Delete(ctx, userID, fileID); delErr != nil {
				slog.Warn("route delete: ShadowFS.Delete route photo failed", "route_id", routeID, "file_id", fileID, "error", delErr)
			}
		}
	}
}
