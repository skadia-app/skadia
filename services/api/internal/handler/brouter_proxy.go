package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// validBRouterProfiles is the allowlist of BRouter routing profiles.
// Any profile not in this map is replaced with the default "hiking-mountain".
var validBRouterProfiles = map[string]bool{
	"hiking-mountain":  true, // Offizielles BRouter-Wanderprofil (SAC T1-T6, konfigurierbar)
	"hiking-sac2":      true, // Skadia SAC T2 Profil (einfach, hardcoded)
	"trekking":         true, // Rad-Trekking (Standard BRouter)
	"cycling-road":     true, // Rennrad
	"cycling-mountain": true, // MTB
	"cycling-gravel":   true, // Gravel
	"running":          true, // Laufen (basierend auf hiking-mountain)
	"walking":          true, // Spaziergang (basierend auf hiking-mountain)
}

// RouteRequest is the request body for POST /api/v1/routing/route.
type RouteRequest struct {
	// Waypoints is a list of [longitude, latitude] pairs (at least 2).
	Waypoints    [][2]float64      `json:"waypoints"`
	// Profile is the BRouter routing profile name (e.g. "hiking-sac2", "trekking").
	Profile      string            `json:"profile"`
	// Alternatives is the BRouter alternative-route index (0 = primary).
	Alternatives int               `json:"alternatives"`
	// ProfileParams are optional key=value overrides for BRouter profile variables.
	// Passed as profile:key=value query parameters to BRouter.
	// Example: {"SAC_scale_limit": "2", "consider_elevation": "true"}
	ProfileParams map[string]string `json:"profile_params,omitempty"`
}

// RouteViabrouter proxies a routing request to the BRouter service and returns
// a GeoJSON FeatureCollection. If BRouter is unavailable or returns an error,
// a straight-line (as-the-crow-flies) fallback GeoJSON is returned instead so
// the frontend always has a drawable geometry.
//
// POST /api/v1/routing/route
func (h *Handler) RouteViabrouter(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req RouteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}

	if len(req.Waypoints) < 2 {
		writeError(w, http.StatusBadRequest, "Bad Request", "at least 2 waypoints required", r.URL.Path)
		return
	}
	if !validBRouterProfiles[req.Profile] {
		req.Profile = "hiking-mountain"
	}
	if req.Alternatives < 0 {
		req.Alternatives = 0
	}
	if req.Alternatives > 5 {
		req.Alternatives = 5
	}

	brouterURL := h.brouterBaseURL()
	if brouterURL == "" {
		slog.Warn("BRouter URL not configured, using straight-line fallback")
		h.straightLineFallback(w, req.Waypoints)
		return
	}

	// Build BRouter query URL.
	// BRouter expects: lonlats=lon1,lat1|lon2,lat2|...
	lonlats := make([]string, 0, len(req.Waypoints))
	for _, wp := range req.Waypoints {
		lonlats = append(lonlats, fmt.Sprintf("%.6f,%.6f", wp[0], wp[1]))
	}
	queryURL := fmt.Sprintf(
		"%s/brouter?lonlats=%s&profile=%s&alternativeidx=%d&format=geojson",
		brouterURL,
		strings.Join(lonlats, "|"),
		req.Profile,
		req.Alternatives,
	)

	// profile:key=value Parameter für Fein-Tuning anhängen
	for k, v := range req.ProfileParams {
		if isValidProfileParam(k) && isValidProfileParam(v) {
			queryURL += fmt.Sprintf("&profile:%s=%s", k, v)
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, queryURL, nil)
	if err != nil {
		slog.Warn("BRouter: failed to create request", "error", err)
		h.straightLineFallback(w, req.Waypoints)
		return
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Warn("BRouter request failed, using straight-line fallback",
			"error", err,
			"url", queryURL,
		)
		h.straightLineFallback(w, req.Waypoints)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("BRouter returned non-200, using straight-line fallback",
			"status", resp.StatusCode,
			"body", string(body),
		)
		h.straightLineFallback(w, req.Waypoints)
		return
	}

	// Proxy the GeoJSON response from BRouter directly to the client.
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("BRouter proxy: failed to copy response body", "error", err)
	}
}

// straightLineFallback writes a minimal GeoJSON FeatureCollection with a
// single LineString connecting the given waypoints. It is used whenever
// BRouter is unavailable so the frontend can still display a route.
func (h *Handler) straightLineFallback(w http.ResponseWriter, waypoints [][2]float64) {
	coords := make([]interface{}, len(waypoints))
	for i, wp := range waypoints {
		coords[i] = wp
	}
	result := map[string]interface{}{
		"type": "FeatureCollection",
		"features": []map[string]interface{}{
			{
				"type": "Feature",
				"properties": map[string]interface{}{
					"creator":  "Skadia-fallback",
					"fallback": true,
				},
				"geometry": map[string]interface{}{
					"type":        "LineString",
					"coordinates": coords,
				},
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Warn("straightLineFallback: failed to encode response", "error", err)
	}
}

// isValidProfileParam validates BRouter profile parameter keys and values.
// Only allows alphanumeric, underscore, dot, and hyphen characters.
func isValidProfileParam(s string) bool {
	if len(s) == 0 || len(s) > 50 {
		return false
	}
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// brouterBaseURL returns the configured BRouter base URL, defaulting to the
// standard container address when the environment variable is not set.
func (h *Handler) brouterBaseURL() string {
	if h.Config != nil && h.Config.BRouterURL != "" {
		return h.Config.BRouterURL
	}
	return "http://brouter:17777"
}
