package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/geo"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
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
	if err := routing.ValidateBRouterProfileParams(req.Profile, req.ProfileParams); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", err.Error(), r.URL.Path)
		return
	}
	if req.Alternatives < 0 {
		req.Alternatives = 0
	}
	if req.Alternatives > 5 {
		req.Alternatives = 5
	}

	if !h.requireFeatureAvailable(w, r, capabilities.KeyBRouter) {
		return
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
		queryURL.WriteString(fmt.Sprintf("&profile:%s=%s", k, v))
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

	// Buffer the upstream body (BRouter responses are 200-500 kB; no streaming
	// needed). Try to JSON-decode and inject elevation_profile; on any decode
	// failure or unexpected shape we fall back to a verbatim passthrough of
	// the buffered body (Spec §R4) so the frontend keeps the legacy 3D-coords
	// pipeline rather than seeing a 5xx.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Warn("BRouter proxy: failed to read upstream body", "error", err)
		http.Error(w, "BRouter Upstream nicht erreichbar", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	var gj map[string]any
	if err := json.Unmarshal(bodyBytes, &gj); err != nil {
		slog.Warn("BRouter proxy: JSON decode failed, falling back to passthrough", "error", err)
		if _, werr := w.Write(bodyBytes); werr != nil {
			slog.Warn("BRouter proxy: passthrough write failed", "error", werr)
		}
		return
	}

	injectElevationProfileFromInlineZ(gj, h.DEMSvc)

	if err := json.NewEncoder(w).Encode(gj); err != nil {
		slog.Warn("BRouter proxy: failed to encode modified response", "error", err)
	}
}

// brouterCoord captures one BRouter coordinate with explicit per-point
// Z-availability. Used by extractCoordinates so that a single null/missing
// Z-value does NOT discard the whole coord-list.
type brouterCoord struct {
	Lon, Lat float64
	Ele      float64 // valid only when EleOK == true
	EleOK    bool
}

// extractCoordinates parses feat.geometry.coordinates into []brouterCoord with
// per-point Z-availability flag. Returns nil only if the geometry is structurally
// missing (no `coordinates` array) — individual point-level issues (null Z,
// wrong-length point) are recorded as EleOK=false rather than discarding everything.
func extractCoordinates(feat map[string]any) []brouterCoord {
	geomRaw, ok := feat["geometry"].(map[string]any)
	if !ok {
		return nil
	}
	coordsRaw, ok := geomRaw["coordinates"].([]any)
	if !ok {
		return nil
	}
	out := make([]brouterCoord, 0, len(coordsRaw))
	for _, c := range coordsRaw {
		arr, ok := c.([]any)
		if !ok || len(arr) < 2 {
			continue
		}
		lon, lonOK := arr[0].(float64)
		lat, latOK := arr[1].(float64)
		if !lonOK || !latOK {
			continue
		}
		bc := brouterCoord{Lon: lon, Lat: lat}
		if len(arr) >= 3 {
			if z, ok := arr[2].(float64); ok {
				bc.Ele = z
				bc.EleOK = true
			}
		}
		out = append(out, bc)
	}
	return out
}

// isPlausibleSrtmEle filters obvious sentinel/garbage values:
//   - BRouter sentinel: ±9999 / ±99999 ("unknown" markers)
//   - SRTM-realistic range: -500 m to +9000 m
//   - Exactly 0.0 is treated as unreliable (BRouter "no DEM available")
func isPlausibleSrtmEle(ele float64) bool {
	if ele == 0 {
		return false
	}
	abs := ele
	if abs < 0 {
		abs = -abs
	}
	if abs >= 9999 {
		return false
	}
	return ele > -500 && ele < 9000
}

// buildProfileFromInlineZ constructs an elevation_profile from per-point
// brouterCoords. Returns nil only if NO coordinate has a plausible Z-value
// (Plain-Wrapper-BRouter without SRTM bundle, or all values implausible).
//
// Distance axis: cumulative Haversine over (lat, lon) using
// geo.DistanceHaversine. Per-point with EleOK=false or implausible Ele:
// the profile omits this point, but the cumulative distance keeps advancing
// over ALL coords (Spec §P5).
func buildProfileFromInlineZ(coords []brouterCoord) []map[string]any {
	if len(coords) < 2 {
		return nil
	}
	hasUsableZ := false
	for _, c := range coords {
		if c.EleOK && isPlausibleSrtmEle(c.Ele) {
			hasUsableZ = true
			break
		}
	}
	if !hasUsableZ {
		return nil
	}

	profile := make([]map[string]any, 0, len(coords))
	var distAccum float64
	for i, c := range coords {
		if i > 0 {
			distAccum += geo.DistanceHaversine(coords[i-1].Lat, coords[i-1].Lon, c.Lat, c.Lon)
		}
		if !c.EleOK || !isPlausibleSrtmEle(c.Ele) {
			continue
		}
		profile = append(profile, map[string]any{
			"dist_m": distAccum,
			"ele_m":  c.Ele,
		})
	}
	if len(profile) < 2 {
		return nil
	}
	return profile
}

// injectElevationProfileFromInlineZ walks gj.features[].properties and sets
// "elevation_profile" derived from each feature's inline 3D coordinates. If a
// feature has no usable Z, the value is nil (frontend triggers fallback per
// Spec §6.2 Entscheidungstabelle).
//
// When demSvc is non-nil, coordinates with missing or implausible inline-Z are
// filled from the DEM service in a single batch call per feature (Spec §2.1
// owner guarantee — defense-in-depth; likely never triggered in production).
//
// Robustness (Spec §R4 — Fallback-Strategie ist Caller-Side):
//   - Missing/non-array features → no-op
//   - Feature without (or null) properties → fresh map with elevation_profile
//   - Feature with non-map properties → preserve unchanged (no data loss)
//   - demSvc == nil → no DEM fallback attempted, profile stays as-is
func injectElevationProfileFromInlineZ(gj map[string]any, demSvc dem.Servicer) {
	features, ok := gj["features"].([]any)
	if !ok {
		return
	}
	for _, f := range features {
		feat, ok := f.(map[string]any)
		if !ok {
			continue
		}
		coords := extractCoordinates(feat)
		profile := buildProfileFromInlineZ(coords)

		// When the inline-Z pass yields an incomplete or missing profile,
		// fill gaps from the DEM service (single batch call per feature).
		// Defense-in-depth: demSvc may be nil when not configured.
		if demSvc != nil && coords != nil {
			profile = fillMissingElevFromDEM(coords, profile, demSvc)
		}

		// Store as a nil-interface rather than a typed-nil slice so that
		// downstream `val == nil` checks (and the encoded JSON `null`) behave
		// as callers expect. A typed-nil `[]map[string]any` wrapped in
		// `interface{}` would compare != nil even though it JSON-encodes as
		// null — avoid that foot-gun at the boundary.
		var profileVal any
		if profile != nil {
			profileVal = profile
		}

		rawProps, hasProps := feat["properties"]
		if !hasProps || rawProps == nil {
			feat["properties"] = map[string]any{"elevation_profile": profileVal}
			continue
		}
		props, ok := rawProps.(map[string]any)
		if !ok {
			continue
		}
		props["elevation_profile"] = profileVal
	}
}

// fillMissingElevFromDEM fills coords whose inline-Z is missing or implausible
// from demSvc in a single batch call, then rebuilds the elevation_profile.
//
// Strategy:
//  1. Identify coords without a plausible inline-Z.
//  2. If none are missing and profile is already complete (len == usable inline-Z
//     count), return profile unchanged.
//  3. Build a merged coord list: inline-Z where available, DEM where not.
//  4. Call buildProfileFromInlineZ on the merged list.
//
// Returns the original profile unchanged on any DEM error (no degradation).
// NoData results (dem.NoData sentinel) remain absent from the profile (ele_m
// stays nil by omission — the point is simply not emitted).
func fillMissingElevFromDEM(coords []brouterCoord, profile []map[string]any, demSvc dem.Servicer) []map[string]any {
	// Collect indices of coords that lack a plausible inline-Z.
	missing := make([]int, 0, len(coords))
	for i, c := range coords {
		if !c.EleOK || !isPlausibleSrtmEle(c.Ele) {
			missing = append(missing, i)
		}
	}
	if len(missing) == 0 {
		// All coords already have plausible inline-Z — nothing to fill.
		return profile
	}

	// Encode missing coordinates as int32×1e7 for the DEM batch call.
	lats := make([]int32, len(missing))
	lons := make([]int32, len(missing))
	for j, idx := range missing {
		lats[j] = int32(coords[idx].Lat * 1e7)
		lons[j] = int32(coords[idx].Lon * 1e7)
	}

	demEles, err := demSvc.ElevationBatch(lats, lons)
	if err != nil {
		slog.Debug("BRouter proxy: DEM fallback failed, keeping inline-Z profile", "error", err)
		return profile
	}

	// Build a merged coord slice: keep original inline-Z where plausible,
	// substitute DEM elevation for gaps. Points where DEM returns NoData are
	// left with EleOK=false so buildProfileFromInlineZ omits them naturally.
	merged := make([]brouterCoord, len(coords))
	copy(merged, coords)
	for j, idx := range missing {
		if j >= len(demEles) {
			break
		}
		dm := demEles[j]
		if dm == dem.NoData {
			// DEM has no coverage here — leave EleOK=false.
			continue
		}
		// DEM returns decimeters; convert to meters for the profile.
		merged[idx].Ele = float64(dm) / 10.0
		merged[idx].EleOK = true
	}

	rebuilt := buildProfileFromInlineZ(merged)
	if rebuilt == nil {
		// DEM data didn't produce a usable profile either — keep original.
		return profile
	}
	return rebuilt
}

// brouterBaseURL returns the configured BRouter base URL. An empty URL disables
// BRouter consistently with capability detection.
func (h *Handler) brouterBaseURL() string {
	if h.Config != nil && h.Config.BRouterURL != "" {
		return h.Config.BRouterURL
	}
	return ""
}
