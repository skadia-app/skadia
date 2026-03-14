package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/middleware"
)

// maxBRouterResponseSize is the maximum accepted BRouter HTTP response size (10 MB).
const maxBRouterResponseSize = 10 << 20

// RouteSuggestionRequest is the request body for POST /api/v1/routes/suggest.
type RouteSuggestionRequest struct {
	// Start is [longitude, latitude] of the starting point.
	Start [2]float64 `json:"start"`
	// End is an optional fixed end point. If omitted and RoundTrip is true
	// (the default when End is nil), the route returns to Start.
	End *[2]float64 `json:"end,omitempty"`
	// RoundTrip indicates the route should return to Start. Ignored when End is set.
	RoundTrip bool `json:"round_trip"`
	// DirectionPoint is an optional point the suggested route should trend toward.
	DirectionPoint *[2]float64 `json:"direction_point,omitempty"`
	// TargetDistanceM is the desired total route length in metres.
	TargetDistanceM float64 `json:"target_distance_m"`
	// TargetElevationGain is the desired total elevation gain in metres (optional hint).
	TargetElevationGain float64 `json:"target_elevation_gain,omitempty"`
	// SportType is the FIT sport type (e.g. "hiking", "cycling").
	SportType string `json:"sport_type"`
	// Profile is the BRouter routing profile (e.g. "hiking-sac2", "trekking").
	// Defaults to "hiking-sac2".
	Profile string `json:"profile"`
	// PreferUnknown, when true, weights the via-point selection toward
	// trail segments the user has not visited yet (Terra Incognita).
	PreferUnknown bool `json:"prefer_unknown"`
	// LandscapePrefs maps landscape keywords (e.g. "forest", "summit") to a
	// preference weight (1–5). Reserved for future use.
	LandscapePrefs map[string]int `json:"landscape_prefs,omitempty"`
	// SurfacePrefs maps surface types (e.g. "unpaved", "asphalt") to a weight (1–5).
	SurfacePrefs map[string]int `json:"surface_prefs,omitempty"`
}

// RouteSuggestion is a single suggested route variant returned by SuggestRoutes.
type RouteSuggestion struct {
	// Variant is the 1-based variant index (1 = primary, 2/3 = alternatives).
	Variant int `json:"variant"`
	// Profile is the BRouter profile used.
	Profile string `json:"profile"`
	// ViaPoint is the [lng, lat] of the intermediate waypoint used for this variant.
	ViaPoint [2]float64 `json:"via_point"`
	// GeoJSON is the BRouter GeoJSON FeatureCollection for this variant.
	// May be a straight-line fallback when BRouter is unavailable.
	GeoJSON json.RawMessage `json:"geojson"`
	// IsFallback is true when the geometry is a straight-line fallback rather
	// than a real routed track.
	IsFallback bool `json:"is_fallback"`
}

// RouteSuggestionResponse is the JSON response for POST /api/v1/routes/suggest.
type RouteSuggestionResponse struct {
	Suggestions []RouteSuggestion `json:"suggestions"`
}

// SuggestRoutes generates up to 3 round-trip route suggestions starting from
// the given point. The algorithm:
//
//  1. Find the nearest node in the osm_ways pgRouting topology.
//  2. Run pgr_drivingDistance to find nodes reachable within half the target
//     distance (assuming a round-trip; cost column stores travel minutes).
//  3. Select up to 3 via-points from the reachable set that are spread in
//     different compass directions. When PreferUnknown is true, nodes on
//     trail segments the user has never visited (heatmap_trail_matches) are
//     preferred.
//  4. For each via-point, call BRouter (Start → via → End/Start) and collect
//     the resulting GeoJSON.
//
// If the pgRouting topology is not yet available (e.g. OSM not imported),
// the endpoint returns a 503 with a clear error message so the frontend can
// surface a helpful prompt.
//
// POST /api/v1/routes/suggest
func (h *Handler) SuggestRoutes(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())
	if userID == "" {
		writeError(w, http.StatusUnauthorized, "Unauthorized", "no user ID in context", r.URL.Path)
		return
	}

	var req RouteSuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Bad Request", "invalid request body", r.URL.Path)
		return
	}

	// --- Input validation ---
	if req.Start[0] == 0 && req.Start[1] == 0 {
		writeError(w, http.StatusBadRequest, "Bad Request", "start coordinate required", r.URL.Path)
		return
	}
	if req.TargetDistanceM <= 0 {
		req.TargetDistanceM = 10000 // default 10 km
	}
	if !validBRouterProfiles[req.Profile] {
		req.Profile = "hiking-sac2"
	}

	// Round-trip when no explicit end point supplied.
	roundTrip := req.RoundTrip || req.End == nil

	// --- Step 1: Nearest topology node ---
	startNode, err := h.DB.NearestTrailNode(r.Context(), req.Start)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusServiceUnavailable,
				"Trail Network Unavailable",
				"No trail topology found. Import OSM data first via Settings → OSM.",
				r.URL.Path)
			return
		}
		slog.Error("suggest routes: nearest trail node failed", "error", err, "user_id", userID)
		writeError(w, http.StatusServiceUnavailable,
			"Trail Network Unavailable",
			"Could not query trail topology. Import OSM data first via Settings → OSM.",
			r.URL.Path)
		return
	}

	// --- Step 2: pgr_drivingDistance ---
	// Travel time budget: half the target distance at sport-appropriate speed.
	// The factor ½ covers the outward leg; the return is the symmetric mirror.
	avgSpeedKmh := routingSpeedKmh(req.SportType)
	halfDistKm := (req.TargetDistanceM / 1000.0) / 2.0
	halfBudgetMin := (halfDistKm / avgSpeedKmh) * 60.0

	reachable, err := h.DB.DrivingDistance(r.Context(), startNode, halfBudgetMin)
	if err != nil {
		slog.Error("suggest routes: pgr_drivingDistance failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Routing Error", "pgRouting query failed", r.URL.Path)
		return
	}

	if len(reachable) == 0 {
		// Topology exists but no reachable edges — return empty suggestions.
		writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: []RouteSuggestion{}})
		return
	}

	// --- Step 3: Resolve node coordinates ---
	nodeCoords, err := h.DB.ResolveNodeCoordinates(r.Context(), reachable)
	if err != nil {
		slog.Warn("suggest routes: resolving node coords failed", "error", err)
		writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: []RouteSuggestion{}})
		return
	}

	// --- Step 4: Optionally load already-visited trail IDs for Terra Incognita bias ---
	var visitedTrailIDs map[int64]bool
	if req.PreferUnknown {
		ids, dbErr := h.DB.GetVisitedTrailIDs(r.Context(), userID)
		if dbErr != nil {
			slog.Warn("suggest routes: could not load visited trail IDs", "error", dbErr)
		}
		visitedTrailIDs = ids
	}

	// --- Step 5: Select up to 3 diverse via-points ---
	viaPoints := selectDiverseViaPoints(req, reachable, nodeCoords, visitedTrailIDs, halfBudgetMin)

	if len(viaPoints) == 0 {
		writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: []RouteSuggestion{}})
		return
	}

	// --- Step 6: Route each via-point through BRouter ---
	var suggestions []RouteSuggestion
	for i, via := range viaPoints {
		var waypoints [][2]float64
		if roundTrip {
			waypoints = [][2]float64{req.Start, via, req.Start}
		} else {
			waypoints = [][2]float64{req.Start, via, *req.End}
		}

		geojson, isFallback, routeErr := h.routeViaBRouterRaw(r.Context(), waypoints, req.Profile)
		if routeErr != nil {
			slog.Warn("suggest routes: BRouter failed for variant", "variant", i+1, "error", routeErr)
			continue
		}

		suggestions = append(suggestions, RouteSuggestion{
			Variant:    i + 1,
			Profile:    req.Profile,
			ViaPoint:   via,
			GeoJSON:    geojson,
			IsFallback: isFallback,
		})
	}

	if suggestions == nil {
		suggestions = []RouteSuggestion{}
	}

	writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: suggestions})
}

// selectDiverseViaPoints selects up to 3 candidate via-points from the
// pgr_drivingDistance result set. Candidates are filtered to nodes whose
// accumulated cost falls within [35%, 65%] of the half-budget (i.e. roughly
// the midpoint of the outward leg). The selection maximises angular spread
// around the start point to produce geometrically distinct route variants.
//
// When visitedTrailIDs is non-nil, nodes that are adjacent to unvisited trails
// are preferred (Terra Incognita bias).
//
// When req.DirectionPoint is set, candidates within ±60° of that bearing are
// preferred.
func selectDiverseViaPoints(
	req RouteSuggestionRequest,
	reachable []database.ReachableNode,
	coords map[int64][2]float64, // nodeID → [lng, lat]
	visitedTrailIDs map[int64]bool,
	halfBudgetMin float64,
) [][2]float64 {

	minCost := halfBudgetMin * 0.35
	maxCost := halfBudgetMin * 0.65

	type candidate struct {
		nodeID  int64
		lngLat  [2]float64
		bearing float64
		unknown bool
	}

	buildPool := func(filterBand bool) []candidate {
		var pool []candidate
		for _, n := range reachable {
			lngLat, ok := coords[n.NodeID]
			if !ok {
				continue
			}
			if filterBand && (n.Cost < minCost || n.Cost > maxCost) {
				continue
			}
			bearing := bearingDeg(req.Start[1], req.Start[0], lngLat[1], lngLat[0])
			unknown := visitedTrailIDs != nil && !visitedTrailIDs[n.NodeID]

			// Direction filter: prefer candidates within ±60° of the requested bearing.
			if req.DirectionPoint != nil {
				targetBearing := bearingDeg(req.Start[1], req.Start[0], req.DirectionPoint[1], req.DirectionPoint[0])
				diff := math.Abs(bearing - targetBearing)
				if diff > 180 {
					diff = 360 - diff
				}
				if diff > 60 {
					continue
				}
			}

			pool = append(pool, candidate{
				nodeID:  n.NodeID,
				lngLat:  lngLat,
				bearing: bearing,
				unknown: unknown,
			})
		}
		return pool
	}

	pool := buildPool(true)
	// Fall back to full reachable set when the distance band leaves nothing.
	if len(pool) == 0 {
		pool = buildPool(false)
	}
	if len(pool) == 0 {
		return nil
	}

	// Terra Incognita score: unknown trails get a bonus for greedy selection.
	score := func(c candidate) float64 {
		if c.unknown {
			return 1.0
		}
		return 0.0
	}

	// Seed with the highest-scored (or first) candidate.
	best := 0
	for i := 1; i < len(pool); i++ {
		if score(pool[i]) > score(pool[best]) {
			best = i
		}
	}

	selected := []candidate{pool[best]}
	pool = append(pool[:best], pool[best+1:]...)

	// Greedily add candidates that maximise minimum angular distance to already-selected.
	for len(selected) < 3 && len(pool) > 0 {
		maxMinAngle := -1.0
		nextIdx := 0
		for i, c := range pool {
			minAngle := math.MaxFloat64
			for _, s := range selected {
				diff := math.Abs(c.bearing - s.bearing)
				if diff > 180 {
					diff = 360 - diff
				}
				if diff < minAngle {
					minAngle = diff
				}
			}
			// Terra Incognita bias: 5° virtual spread bonus for unknown trails.
			effective := minAngle + score(c)*5.0
			if effective > maxMinAngle {
				maxMinAngle = effective
				nextIdx = i
			}
		}
		selected = append(selected, pool[nextIdx])
		pool = append(pool[:nextIdx], pool[nextIdx+1:]...)
	}

	result := make([][2]float64, len(selected))
	for i, s := range selected {
		result[i] = s.lngLat
	}
	return result
}

// routingSpeedKmh returns a rough average speed (km/h) for a given sport type,
// used to convert a target distance into a pgr_drivingDistance time budget.
func routingSpeedKmh(sportType string) float64 {
	switch sportType {
	case "cycling", "road_cycling", "mountain_biking", "gravel_cycling":
		return 20.0
	case "running", "trail_running":
		return 10.0
	case "skiing", "cross_country_skiing", "backcountry_skiing":
		return 12.0
	default:
		return 5.0 // hiking / walking baseline
	}
}

// bearingDeg returns the initial bearing (0–360°) from (lat1, lon1) to
// (lat2, lon2), where all angles are in decimal degrees.
func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	lat1r := lat1 * math.Pi / 180
	lat2r := lat2 * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180

	x := math.Sin(dLon) * math.Cos(lat2r)
	y := math.Cos(lat1r)*math.Sin(lat2r) - math.Sin(lat1r)*math.Cos(lat2r)*math.Cos(dLon)
	bearing := math.Atan2(x, y) * 180 / math.Pi
	return math.Mod(bearing+360, 360)
}

// routeViaBRouterRaw calls BRouter for the given waypoints and returns the raw
// GeoJSON bytes plus a fallback flag. On BRouter unavailability or error it
// returns a straight-line GeoJSON with isFallback=true.
func (h *Handler) routeViaBRouterRaw(ctx context.Context, waypoints [][2]float64, profile string) (json.RawMessage, bool, error) {
	if len(waypoints) < 2 {
		return nil, false, fmt.Errorf("at least 2 waypoints required")
	}

	// Validate profile against allowlist.
	if !validBRouterProfiles[profile] {
		profile = "hiking-sac2"
	}

	brouterURL := h.brouterBaseURL()
	if brouterURL == "" {
		return h.straightLineGeoJSON(waypoints), true, nil
	}

	lonlats := make([]string, 0, len(waypoints))
	for _, wp := range waypoints {
		lonlats = append(lonlats, fmt.Sprintf("%.6f,%.6f", wp[0], wp[1]))
	}

	queryURL := fmt.Sprintf(
		"%s/brouter?lonlats=%s&profile=%s&alternativeidx=0&format=geojson",
		brouterURL,
		strings.Join(lonlats, "|"),
		profile,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, queryURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("create request: %w", err)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Warn("SuggestRoutes: BRouter unavailable, using straight-line fallback", "error", err)
		return h.straightLineGeoJSON(waypoints), true, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBRouterResponseSize))
		slog.Warn("SuggestRoutes: BRouter non-200, using straight-line fallback",
			"status", resp.StatusCode, "body", string(body))
		return h.straightLineGeoJSON(waypoints), true, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBRouterResponseSize))
	if err != nil {
		slog.Warn("SuggestRoutes: failed to read BRouter response, using fallback", "error", err)
		return h.straightLineGeoJSON(waypoints), true, nil
	}
	return json.RawMessage(body), false, nil
}

// straightLineGeoJSON builds a minimal GeoJSON FeatureCollection with a single
// LineString through all waypoints. Used as fallback when BRouter is unreachable.
func (h *Handler) straightLineGeoJSON(waypoints [][2]float64) json.RawMessage {
	coords := make([]any, len(waypoints))
	for i, wp := range waypoints {
		coords[i] = wp
	}
	result := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{
			{
				"type": "Feature",
				"properties": map[string]any{
					"creator":  "Skadia-fallback",
					"fallback": true,
				},
				"geometry": map[string]any{
					"type":        "LineString",
					"coordinates": coords,
				},
			},
		},
	}
	b, _ := json.Marshal(result)
	return json.RawMessage(b)
}
