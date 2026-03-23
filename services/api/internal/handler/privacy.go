package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) ListPrivacyZones(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	zones, err := h.DB.ListPrivacyZones(r.Context(), userID)
	if err != nil {
		slog.Error("list privacy zones failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to list privacy zones", r.URL.Path)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, zones)
}

func (h *Handler) CreatePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req struct {
		Name     string  `json:"name"`
		Lat      float64 `json:"lat"`
		Lng      float64 `json:"lng"`
		RadiusM  int     `json:"radius_m"`
		Color    string  `json:"color"`
		ZoneType string  `json:"zone_type"` // 'circle' (default) or 'polygon'
		Polygon  *string `json:"polygon"`   // GeoJSON Polygon string, required when zone_type='polygon'
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if req.ZoneType == "" {
		req.ZoneType = "circle"
	}
	if req.ZoneType == "circle" && req.RadiusM <= 0 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "radius_m must be positive for circle zones", r.URL.Path)
		return
	}
	if req.ZoneType == "polygon" && req.Polygon == nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "polygon is required for polygon zones", r.URL.Path)
		return
	}
	if req.Color == "" {
		req.Color = "#F97316"
	}
	// Für Polygone einen sinnvollen radius_m-Defaultwert setzen (wird nicht für Masking genutzt, aber NOT NULL)
	if req.RadiusM <= 0 {
		req.RadiusM = 200
	}

	zone, err := h.DB.CreatePrivacyZone(r.Context(), userID, req.Name, req.Lat, req.Lng, req.RadiusM, req.Color, req.ZoneType, req.Polygon)
	if err != nil {
		slog.Error("create privacy zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to create privacy zone", r.URL.Path)
		return
	}

	// Recompute all tracks async (detached context, request may be done)
	go func() {
		if err := h.DB.RecomputeAllTrackPublic(context.Background(), userID); err != nil {
			slog.Error("recompute tracks after zone create failed", "error", err)
		}
	}()

	httputil.WriteJSON(w, http.StatusCreated, zone)
}

func (h *Handler) UpdatePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	zoneID := chi.URLParam(r, "id")

	var update model.PrivacyZoneUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	zone, err := h.DB.UpdatePrivacyZone(r.Context(), zoneID, userID, &update)
	if err != nil {
		slog.Error("update privacy zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to update privacy zone", r.URL.Path)
		return
	}
	if zone == nil {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "privacy zone not found", r.URL.Path)
		return
	}

	// Recompute all tracks async
	go func() {
		if err := h.DB.RecomputeAllTrackPublic(context.Background(), userID); err != nil {
			slog.Error("recompute tracks after zone update failed", "error", err)
		}
	}()

	httputil.WriteJSON(w, http.StatusOK, zone)
}

func (h *Handler) DeletePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	zoneID := chi.URLParam(r, "id")

	if err := h.DB.DeletePrivacyZone(r.Context(), zoneID, userID); err != nil {
		if err.Error() == "privacy zone not found" {
			httputil.WriteError(w, http.StatusNotFound, "Not Found", "privacy zone not found", r.URL.Path)
			return
		}
		slog.Error("delete privacy zone failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "Internal Error", "failed to delete privacy zone", r.URL.Path)
		return
	}

	// Recompute all tracks async
	go func() {
		if err := h.DB.RecomputeAllTrackPublic(context.Background(), userID); err != nil {
			slog.Error("recompute tracks after zone delete failed", "error", err)
		}
	}()

	w.WriteHeader(http.StatusNoContent)
}
