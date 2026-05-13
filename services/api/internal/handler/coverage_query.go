package handler

import (
	"crypto/subtle"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
)

func (h *Handler) QueryCoverage(w http.ResponseWriter, r *http.Request) {
	if secret := coverageQuerySecret(h); secret != "" {
		got := r.Header.Get("X-ML-Callback-Secret")
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			httputil.WriteError(w, http.StatusUnauthorized, "Unauthorized", "internal service authentication failed", r.URL.Path)
			return
		}
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
	if !validCoverageCoordinate(lat, lon) {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid coordinates", "lat/lon out of range", r.URL.Path)
		return
	}

	layers, err := parseCoverageQueryLayers(layersStr)
	if err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid layers", err.Error(), r.URL.Path)
		return
	}

	if h.CoverageReader == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable,
			"Coverage reader not initialized",
			"coverage binaries are not available on this server",
			r.URL.Path)
		return
	}

	result := make(map[string]int, len(layers))
	for _, layer := range layers {
		val, err := h.CoverageReader.SamplePointByLayer(r.Context(), layer, lat, lon)
		if err != nil {
			result[layer] = -1
			continue
		}
		result[layer] = int(val)
	}

	httputil.WriteJSON(w, http.StatusOK, result)
}

func coverageQuerySecret(h *Handler) string {
	if h == nil || h.Config == nil {
		return ""
	}
	return strings.TrimSpace(h.Config.MLCallbackSecret)
}

func validCoverageCoordinate(lat, lon float64) bool {
	return !math.IsNaN(lat) && !math.IsInf(lat, 0) &&
		!math.IsNaN(lon) && !math.IsInf(lon, 0) &&
		lat >= -90 && lat <= 90 &&
		lon >= -180 && lon <= 180
}

func parseCoverageQueryLayers(raw string) ([]string, error) {
	allowed := map[string]bool{
		"forest": true, "landuse": true, "water": true, "protected": true,
		"urbanization": true, "river_proximity": true, "noise": true, "traffic": true,
		"populate": true, "industrial": true, "mw_density": true, "hw_density": true,
	}
	seen := map[string]bool{}
	layers := make([]string, 0)
	for _, part := range strings.Split(raw, ",") {
		layer := strings.TrimSpace(part)
		if layer == "" {
			continue
		}
		if !allowed[layer] {
			return nil, fmt.Errorf("invalid coverage layer %q", layer)
		}
		if seen[layer] {
			continue
		}
		seen[layer] = true
		layers = append(layers, layer)
	}
	if len(layers) == 0 {
		return nil, fmt.Errorf("at least one coverage layer is required")
	}
	if len(layers) > 8 {
		return nil, fmt.Errorf("too many coverage layers (max 8)")
	}
	return layers, nil
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
