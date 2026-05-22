package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListTargetPlaces(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	places, err := h.DB.ListTargetPlaces(r.Context(), model.ListTargetObjectsParams{
		UserID:                userID,
		IsAdmin:               middleware.GetIsAdmin(r.Context()),
		VisibleAccessScopeIDs: scopes,
		Kinds:                 splitCSV(r.URL.Query().Get("kinds")),
		BBox:                  parseTargetBBox(r.URL.Query().Get("bbox")),
		Search:                r.URL.Query().Get("search"),
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"places": places})
}

func (h *Handler) CreateTargetPlace(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetPlaceCreate](w, r)
	if !ok {
		return
	}
	place, err := h.DB.CreateTargetPlace(r.Context(), userID, input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"place": place})
}

func (h *Handler) GetTargetPlace(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	place, err := h.DB.GetTargetPlace(r.Context(), userID, chi.URLParam(r, "id"), scopes)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	if r.URL.Query().Get("include_overlay_summary") == "true" && place != nil {
		summary, err := h.DB.GetTargetOverlaySummary(r.Context(), "place", place.ID, nil)
		if handleTargetHandlerErr(w, r, err) {
			return
		}
		place.OverlaySummary = summary
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"place": place})
}

func (h *Handler) UpdateTargetPlace(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetPlaceUpdate](w, r)
	if !ok {
		return
	}
	place, err := h.DB.UpdateTargetPlace(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"place": place})
}

func (h *Handler) DeleteTargetPlace(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if err := h.DB.ArchiveTargetPlace(r.Context(), userID, chi.URLParam(r, "id")); handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) ListTargetRegions(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	regions, err := h.DB.ListTargetRegions(r.Context(), model.ListTargetObjectsParams{
		UserID:                userID,
		IsAdmin:               middleware.GetIsAdmin(r.Context()),
		VisibleAccessScopeIDs: scopes,
		Kinds:                 splitCSV(r.URL.Query().Get("kinds")),
		BBox:                  parseTargetBBox(r.URL.Query().Get("bbox")),
		Search:                r.URL.Query().Get("search"),
		Limit:                 parseIntDefault(r.URL.Query().Get("limit"), 100),
		Offset:                parseIntDefault(r.URL.Query().Get("offset"), 0),
	})
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

func (h *Handler) CreateTargetRegion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetRegionCreate](w, r)
	if !ok {
		return
	}
	region, err := h.DB.CreateTargetRegion(r.Context(), userID, input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusCreated, map[string]any{"region": region})
}

func (h *Handler) GetTargetRegion(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	scopes := h.resolveTargetVisibleScopes(w, r, userID)
	if scopes == nil && userID != "" {
		return
	}
	region, err := h.DB.GetTargetRegion(r.Context(), userID, chi.URLParam(r, "id"), scopes)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	if r.URL.Query().Get("include_overlay_summary") == "true" && region != nil {
		summary, err := h.DB.GetTargetOverlaySummary(r.Context(), "region", region.ID, nil)
		if handleTargetHandlerErr(w, r, err) {
			return
		}
		region.OverlaySummary = summary
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"region": region})
}

func (h *Handler) UpdateTargetRegion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	input, ok := httputil.DecodeJSON[model.TargetRegionUpdate](w, r)
	if !ok {
		return
	}
	region, err := h.DB.UpdateTargetRegion(r.Context(), userID, chi.URLParam(r, "id"), input)
	if handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]any{"region": region})
}

func (h *Handler) DeleteTargetRegion(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	if err := h.DB.ArchiveTargetRegion(r.Context(), userID, chi.URLParam(r, "id")); handleTargetHandlerErr(w, r, err) {
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) resolveTargetVisibleScopes(w http.ResponseWriter, r *http.Request, userID string) []string {
	return h.resolveVisibleAccessScopesForRequest(w, r, userID)
}

func handleTargetHandlerErr(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, database.ErrTargetValidation) || errors.Is(err, database.ErrObjectAccessValidation) {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return true
	}
	if errors.Is(err, database.ErrTargetNotFound) || errors.Is(err, database.ErrObjectAccessNotFound) {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "target not found", r.URL.Path)
		return true
	}
	httputil.HandleDBErr(w, r, err, "target not found")
	return true
}

func parseTargetBBox(raw string) *model.GeoBBox {
	parts := strings.Split(raw, ",")
	if len(parts) != 4 {
		return nil
	}
	values := make([]float64, 4)
	for i, part := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return nil
		}
		values[i] = v
	}
	return &model.GeoBBox{MinLon: values[0], MinLat: values[1], MaxLon: values[2], MaxLat: values[3]}
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for part := range strings.SplitSeq(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseIntDefault(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}
