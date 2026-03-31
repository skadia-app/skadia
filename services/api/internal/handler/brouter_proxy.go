package handler

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
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
	// Waypoints is a list of [latitude, longitude] pairs (at least 2).
	// Frontend convention is latlng; the proxy swaps to lonlat for BRouter.
	Waypoints [][2]float64 `json:"waypoints"`
	// Profile is the BRouter routing profile name (e.g. "hiking-sac2", "trekking").
	Profile string `json:"profile"`
	// Alternatives is the BRouter alternative-route index (0 = primary).
	Alternatives int `json:"alternatives"`
	// ProfileParams are optional key=value overrides for BRouter profile variables.
	// Passed as profile:key=value query parameters to BRouter.
	// Example: {"SAC_scale_limit": "2", "consider_elevation": "true"}
	ProfileParams map[string]string `json:"profile_params,omitempty"`
}

// RouteViabrouter proxies a routing request to the BRouter service and returns
// a GeoJSON FeatureCollection. If BRouter is unavailable or returns an error,
// a structured JSON error with a "status" field is returned.
//
// POST /api/v1/routing/route
func (h *Handler) RouteViabrouter(w http.ResponseWriter, r *http.Request) {
	_, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[RouteRequest](w, r)
	if !ok {
		return
	}

	if len(req.Waypoints) < 2 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "at least 2 waypoints required", r.URL.Path)
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
		slog.Warn("BRouter URL not configured")
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "brouter_unavailable",
			"error":  "BRouter nicht verfügbar",
		})
		return
	}

	// Build BRouter query URL.
	// BRouter expects: lonlats=lon1,lat1|lon2,lat2|...
	// Frontend sends [lat, lon] (latlng convention), so we swap to [lon, lat] for BRouter.
	lonlats := make([]string, 0, len(req.Waypoints))
	for _, wp := range req.Waypoints {
		lonlats = append(lonlats, fmt.Sprintf("%.6f,%.6f", wp[1], wp[0]))
	}
	var queryURL strings.Builder
	queryURL.WriteString(fmt.Sprintf(
		"%s/brouter?lonlats=%s&profile=%s&alternativeidx=%d&format=geojson",
		brouterURL,
		strings.Join(lonlats, "|"),
		req.Profile,
		req.Alternatives,
	))

	// profile:key=value Parameter für Fein-Tuning anhängen
	for k, v := range req.ProfileParams {
		if isValidProfileParam(k) && isValidProfileParam(v) {
			queryURL.WriteString(fmt.Sprintf("&profile:%s=%s", k, v))
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, queryURL.String(), nil)
	if err != nil {
		slog.Warn("BRouter: failed to create request", "error", err)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "brouter_unavailable",
			"error":  "BRouter nicht erreichbar",
		})
		return
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Warn("BRouter request failed",
			"error", err,
			"url", queryURL.String(),
		)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "brouter_unavailable",
			"error":  "BRouter nicht erreichbar",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("BRouter returned non-200",
			"status", resp.StatusCode,
			"body", string(body),
		)
		httputil.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "no_route",
			"error":  string(body),
		})
		return
	}

	// Proxy the GeoJSON response from BRouter directly to the client.
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, resp.Body); err != nil {
		slog.Warn("BRouter proxy: failed to copy response body", "error", err)
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
