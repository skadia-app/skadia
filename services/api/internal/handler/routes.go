package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

// ListRoutes returns paginated planned routes for the authenticated user.
// GET /api/v1/routes
func (h *Handler) ListRoutes(w http.ResponseWriter, r *http.Request) {
	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	routes, err := h.DB.ListRoutes(r.Context(), userID, limit, offset)
	if err != nil {
		slog.Error("list routes failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list routes", r.URL.Path)
		return
	}

	total, err := h.DB.CountRoutes(r.Context(), userID)
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

	id, err := h.DB.CreateRoute(r.Context(), &route)
	if err != nil {
		slog.Error("create route failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create route", r.URL.Path)
		return
	}

	route.ID = id
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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("get route failed", "error", err, "id", id, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to get route", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, route)
}

// UpdateRoute updates mutable fields of a planned route, enforcing ownership.
// PATCH /api/v1/routes/{id}
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

	route, ok := httputil.DecodeJSON[model.PlannedRoute](w, r)
	if !ok {
		return
	}

	route.ID = id
	route.UserID = userID

	// UpdateRoute enforces ownership via WHERE id = $1 AND user_id = $2
	if err := h.DB.UpdateRoute(r.Context(), &route); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("update route failed", "error", err, "id", id, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update route", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, route)
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

	// DeleteRoute enforces ownership via WHERE id = $1 AND user_id = $2
	if err := h.DB.DeleteRoute(r.Context(), id, userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("delete route failed", "error", err, "id", id, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete route", r.URL.Path)
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
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "route not found", r.URL.Path)
			return
		}
		slog.Error("duplicate route failed", "error", err, "id", id, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to duplicate route", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusCreated, map[string]string{"id": newID})
}
