package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) QueryCoverage(w http.ResponseWriter, r *http.Request) {
	// Try DiskCoverageReader first (preferred), fall back to TileGridCache
	reader := h.CoverageReader
	cache := h.CoverageCache

	if reader == nil && cache == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable,
			"Coverage not available",
			"no coverage reader or cache initialized",
			r.URL.Path)
		return
	}

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	layersStr := r.URL.Query().Get("layers")

	if latStr == "" || lonStr == "" || layersStr == "" {
		httputil.WriteError(w, http.StatusBadRequest,
			"Missing parameters",
			"lat, lon, and layers are required",
			r.URL.Path)
		return
	}

	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid lat", "lat must be a valid float", r.URL.Path)
		return
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid lon", "lon must be a valid float", r.URL.Path)
		return
	}

	layers := strings.Split(layersStr, ",")
	result := make(map[string]int, len(layers))
	for _, layer := range layers {
		layer = strings.TrimSpace(layer)
		if layer == "" {
			continue
		}
		// Prefer DiskCoverageReader, fall back to TileGridCache
		if reader != nil {
			val, err := reader.SamplePointByLayer(r.Context(), layer, lat, lon)
			if err != nil {
				result[layer] = -1
				continue
			}
			result[layer] = int(val)
		} else {
			val, err := cache.SamplePointByLayer(r.Context(), layer, lat, lon)
			if err != nil {
				result[layer] = -1
				continue
			}
			result[layer] = int(val)
		}
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

// CoverageBuildInfo returns the coverage build timestamp for cache-busting.
func (h *Handler) CoverageBuildInfo(w http.ResponseWriter, r *http.Request) {
	ts := ""
	if h.CoverageReader != nil {
		ts = h.CoverageReader.BuildTimestamp()
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"buildTimestamp": ts,
	})
}
