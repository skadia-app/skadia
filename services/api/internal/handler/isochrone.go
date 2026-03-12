package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// IsochroneRequest describes the POST body for /api/v1/isochrone.
type IsochroneRequest struct {
	// Start is a [longitude, latitude] coordinate pair.
	Start [2]float64 `json:"start"`
	// Minutes lists the travel-time thresholds (in minutes) for each isochrone
	// band, e.g. [60, 120, 180].  At least one value is required.
	Minutes []int `json:"minutes"`
	// Profile selects the routing cost function, e.g. "hiking" or "bike".
	// Currently informational — cost column already encodes walk speed via
	// the Tobler function defined in Migration 000131.
	Profile string `json:"profile"`
	// POITypes is an optional list of OSM subtypes / peak types to discover
	// within the reachable area, e.g. ["hut", "cafe", "peak"].
	POITypes []string `json:"poi_types"`
}

// IsochroneResponse is the JSON body returned by POST /api/v1/isochrone.
type IsochroneResponse struct {
	// Polygons contains one GeoJSON polygon per requested time band.
	Polygons interface{} `json:"polygons"`
	// POIs lists points of interest found within the outermost isochrone.
	POIs interface{} `json:"pois"`
}

// Isochrone computes reachable areas from a start point using pgRouting's
// pgr_drivingDistance on the osm_ways topology built by Migration 000151.
// One convex-hull polygon is returned for each requested time band.  POIs
// within the outermost band are optionally returned when poi_types is set.
//
// POST /api/v1/isochrone
func (h *Handler) Isochrone(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID", r.URL.Path)
		return
	}

	var req IsochroneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid JSON body", r.URL.Path)
		return
	}

	if len(req.Minutes) == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "minutes must contain at least one value", r.URL.Path)
		return
	}

	// Validate coordinate range (basic sanity check).
	if req.Start[0] < -180 || req.Start[0] > 180 || req.Start[1] < -90 || req.Start[1] > 90 {
		writeError(w, http.StatusBadRequest, "Bad Request", "start coordinates out of range", r.URL.Path)
		return
	}

	// Find the nearest node in the pgRouting topology.
	startNode, err := h.DB.NearestTrailNode(r.Context(), req.Start)
	if err != nil {
		slog.Warn("isochrone: no trail topology available", "error", err, "user_id", userID)
		writeError(w, http.StatusServiceUnavailable, "No Trail Network",
			"trail topology is not available; import OSM data first", r.URL.Path)
		return
	}

	// Determine the maximum requested time budget.
	maxMinutes := 0.0
	for _, m := range req.Minutes {
		if float64(m) > maxMinutes {
			maxMinutes = float64(m)
		}
	}

	// Run pgr_drivingDistance for the full budget; we filter per band in Go.
	reachable, err := h.DB.DrivingDistance(r.Context(), startNode, maxMinutes)
	if err != nil {
		slog.Error("isochrone: pgr_drivingDistance failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Routing Error",
			"isochrone computation failed", r.URL.Path)
		return
	}

	// Build one convex-hull polygon per time band.
	polygons, err := h.DB.BuildIsochronePolygons(r.Context(), reachable, req.Minutes)
	if err != nil {
		slog.Error("isochrone: polygon build failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Polygon Error",
			"failed to build isochrone polygons", r.URL.Path)
		return
	}

	// Optionally find POIs inside the outermost isochrone.
	var pois interface{}
	if len(req.POITypes) > 0 {
		found, err := h.DB.FindPOIsInIsochrone(r.Context(), reachable, req.POITypes)
		if err != nil {
			// Non-fatal: log and continue without POIs.
			slog.Warn("isochrone: POI lookup failed", "error", err, "user_id", userID)
		} else {
			pois = found
		}
	}

	writeJSON(w, http.StatusOK, IsochroneResponse{
		Polygons: polygons,
		POIs:     pois,
	})
}
