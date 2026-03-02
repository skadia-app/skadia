package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func (h *Handler) ListPrivacyZones(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	zones, err := h.DB.ListPrivacyZones(r.Context(), userID)
	if err != nil {
		slog.Error("list privacy zones failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to list privacy zones", r.URL.Path)
		return
	}

	writeJSON(w, http.StatusOK, zones)
}

func (h *Handler) CreatePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req struct {
		Name    string  `json:"name"`
		Lat     float64 `json:"lat"`
		Lng     float64 `json:"lng"`
		RadiusM int     `json:"radius_m"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Bad Request", "name is required", r.URL.Path)
		return
	}
	if req.RadiusM <= 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "radius_m must be positive", r.URL.Path)
		return
	}

	zone, err := h.DB.CreatePrivacyZone(r.Context(), userID, req.Name, req.Lat, req.Lng, req.RadiusM)
	if err != nil {
		slog.Error("create privacy zone failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to create privacy zone", r.URL.Path)
		return
	}

	// Recompute all tracks async (detached context, request may be done)
	go func() {
		if err := h.DB.RecomputeAllTrackPublic(context.Background(), userID); err != nil {
			slog.Error("recompute tracks after zone create failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusCreated, zone)
}

func (h *Handler) UpdatePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	zoneID := chi.URLParam(r, "id")

	var update model.PrivacyZoneUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	zone, err := h.DB.UpdatePrivacyZone(r.Context(), zoneID, userID, &update)
	if err != nil {
		slog.Error("update privacy zone failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to update privacy zone", r.URL.Path)
		return
	}
	if zone == nil {
		writeError(w, http.StatusNotFound, "Not Found", "privacy zone not found", r.URL.Path)
		return
	}

	// Recompute all tracks async
	go func() {
		if err := h.DB.RecomputeAllTrackPublic(context.Background(), userID); err != nil {
			slog.Error("recompute tracks after zone update failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusOK, zone)
}

func (h *Handler) DeletePrivacyZone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	zoneID := chi.URLParam(r, "id")

	if err := h.DB.DeletePrivacyZone(r.Context(), zoneID, userID); err != nil {
		if err.Error() == "privacy zone not found" {
			writeError(w, http.StatusNotFound, "Not Found", "privacy zone not found", r.URL.Path)
			return
		}
		slog.Error("delete privacy zone failed", "error", err)
		writeError(w, http.StatusInternalServerError, "Internal Error", "failed to delete privacy zone", r.URL.Path)
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
