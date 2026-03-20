package handler

import (
	"net/http"
	"strconv"
	"strings"
)

// QueryCoverage samples coverage grid values at a geographic point for one or
// more layers. Query params: lat, lon, layers (comma-separated).
// Example: GET /api/v1/coverage/query?lat=47.5&lon=11.2&layers=forest,noise,traffic
func (h *Handler) QueryCoverage(w http.ResponseWriter, r *http.Request) {
	if h.CoverageCache == nil {
		writeError(w, http.StatusServiceUnavailable,
			"Coverage cache not initialized",
			"coverage cache is not available on this server",
			r.URL.Path)
		return
	}

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	layersStr := r.URL.Query().Get("layers")

	if latStr == "" || lonStr == "" || layersStr == "" {
		writeError(w, http.StatusBadRequest,
			"Missing parameters",
			"lat, lon, and layers are required",
			r.URL.Path)
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid lat", "lat must be a valid float", r.URL.Path)
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Invalid lon", "lon must be a valid float", r.URL.Path)
		return
	}

	layers := strings.Split(layersStr, ",")
	result := make(map[string]int, len(layers))
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		val, err := h.CoverageCache.SamplePointByLayer(r.Context(), layer, lat, lon)
		if err != nil {
			result[layer] = -1
			continue
		}
		result[layer] = int(val)
	}

	writeJSON(w, http.StatusOK, result)
}
