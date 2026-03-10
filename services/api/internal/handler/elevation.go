package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// QueryElevation returns DEM elevations for a list of [lng, lat] coordinate pairs.
//
// POST /api/v1/elevation/query
//
//	{
//	  "points": [[lng, lat], ...]
//	}
//
// Response:
//
//	{
//	  "elevations": [float64, ...],
//	  "source":     "srtm30" | "copernicus30" | "none" | ...
//	}
func (h *Handler) QueryElevation(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Points [][2]float64 `json:"points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Points) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "points must not be empty", r.URL.Path)
		return
	}
	if len(req.Points) > 10000 {
		writeError(w, http.StatusBadRequest, "Bad Request", "too many points (max 10000)", r.URL.Path)
		return
	}

	elevations, source, err := h.DB.GetElevation(r.Context(), req.Points)
	if err != nil {
		slog.Error("elevation query failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "elevation query failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"elevations": elevations,
		"source":     source,
	}); err != nil {
		slog.Error("elevation query: encode response", "error", err)
	}
}

// ElevationProfile returns a sampled elevation profile along a GeoJSON geometry.
//
// POST /api/v1/elevation/profile
//
//	{
//	  "geometry":   <GeoJSON geometry object>,
//	  "num_points": 100
//	}
//
// Response:
//
//	{
//	  "profile": [
//	    {"fraction": 0.0, "elevation": 1234.5, "lat": 47.1, "lng": 11.2},
//	    ...
//	  ]
//	}
func (h *Handler) ElevationProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req struct {
		Geometry  json.RawMessage `json:"geometry"`
		NumPoints int             `json:"num_points"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}
	if len(req.Geometry) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "geometry is required", r.URL.Path)
		return
	}

	profile, err := h.DB.GetElevationProfile(r.Context(), string(req.Geometry), req.NumPoints)
	if err != nil {
		slog.Error("elevation profile failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Internal Error", "elevation profile failed", r.URL.Path)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"profile": profile,
	}); err != nil {
		slog.Error("elevation profile: encode response", "error", err)
	}
}
