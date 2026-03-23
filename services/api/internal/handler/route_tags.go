package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

// SetRouteTags replaces all tags on a route with the provided set (PUT semantics).
// PUT /api/v1/routes/{id}/tags
// Body: {"tag_ids": ["uuid1", "uuid2"]}
func (h *Handler) SetRouteTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")

	// Ownership check — GetRoute enforces WHERE user_id = $2.
	if _, err := h.DB.GetRoute(r.Context(), routeID, userID); httputil.HandleDBErr(w, r, err, "route not found") {
		return
	}

	req, ok := httputil.DecodeJSON[struct {
		TagIDs []string `json:"tag_ids"`
	}](w, r)
	if !ok {
		return
	}
	if req.TagIDs == nil {
		req.TagIDs = []string{}
	}

	if err := h.DB.SetRouteTags(r.Context(), routeID, userID, req.TagIDs); err != nil {
		slog.Error("set route tags failed", "route_id", routeID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to set route tags", r.URL.Path)
		return
	}

	tags, err := h.DB.GetRouteTags(r.Context(), routeID)
	if err != nil {
		slog.Error("set route tags: get tags failed", "route_id", routeID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to read updated tags", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, tags)
}

// AddRouteTag adds a single tag to a route.
// POST /api/v1/routes/{id}/tags/{tagId}
func (h *Handler) AddRouteTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")

	// Ownership check on the route.
	if _, err := h.DB.GetRoute(r.Context(), routeID, userID); httputil.HandleDBErr(w, r, err, "route not found") {
		return
	}

	if err := h.DB.AssignRouteTag(r.Context(), routeID, tagID, userID); err != nil {
		slog.Error("add route tag failed", "route_id", routeID, "tag_id", tagID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to add tag to route", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RemoveRouteTag removes a single tag from a route.
// DELETE /api/v1/routes/{id}/tags/{tagId}
func (h *Handler) RemoveRouteTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}
	routeID := chi.URLParam(r, "id")
	tagID := chi.URLParam(r, "tagId")

	// Ownership check on the route.
	if _, err := h.DB.GetRoute(r.Context(), routeID, userID); httputil.HandleDBErr(w, r, err, "route not found") {
		return
	}

	if err := h.DB.RemoveRouteTag(r.Context(), routeID, tagID); err != nil {
		slog.Error("remove route tag failed", "route_id", routeID, "tag_id", tagID, "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to remove tag from route", r.URL.Path)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
