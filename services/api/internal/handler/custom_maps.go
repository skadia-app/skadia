package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListCustomMaps(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	maps, err := h.DB.ListCustomMaps(r.Context(), userID)
	if err != nil {
		slog.Error("list custom maps failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list custom maps", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, maps)
}

func (h *Handler) CreateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	var req model.CreateCustomMapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if req.Color == "" {
		req.Color = "#F97316"
	}
	m, err := h.DB.CreateCustomMap(r.Context(), userID, req.Name, req.Color, req.Description)
	if err != nil {
		slog.Error("create custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create custom map", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) UpdateCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	var req model.UpdateCustomMapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	m, err := h.DB.UpdateCustomMap(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("update custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update custom map", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) DeleteCustomMap(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	if err := h.DB.DeleteCustomMap(r.Context(), mapID, userID); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Not Found", "custom map not found", r.URL.Path)
			return
		}
		slog.Error("delete custom map failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete custom map", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCustomMapMarkers(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markers, err := h.DB.ListCustomMapMarkers(r.Context(), mapID, userID)
	if err != nil {
		slog.Error("list custom map markers failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list markers", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, markers)
}

func (h *Handler) CreateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	var req model.CreateMarkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	mk, err := h.DB.CreateCustomMapMarker(r.Context(), mapID, userID, req)
	if err != nil {
		slog.Error("create custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create marker", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusCreated, mk)
}

func (h *Handler) UpdateCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")
	var req model.UpdateMarkerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	mk, err := h.DB.UpdateCustomMapMarker(r.Context(), markerID, mapID, userID, req)
	if err != nil {
		slog.Error("update custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update marker", r.URL.Path)
		return
	}
	writeJSON(w, http.StatusOK, mk)
}

func (h *Handler) DeleteCustomMapMarker(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	mapID := chi.URLParam(r, "mapID")
	markerID := chi.URLParam(r, "markerID")
	if err := h.DB.DeleteCustomMapMarker(r.Context(), markerID, mapID, userID); err != nil {
		if err == pgx.ErrNoRows {
			writeError(w, http.StatusNotFound, "Not Found", "marker not found", r.URL.Path)
			return
		}
		slog.Error("delete custom map marker failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete marker", r.URL.Path)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
