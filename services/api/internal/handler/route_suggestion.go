package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/middleware"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// profileMap maps incoming profile names to BadRouter profile names.
// BadRouter knows: hiking, mtb, gravel, trekking, road.
var profileMap = map[string]string{
	"hiking":      "hiking",
	"hiking-sac2": "hiking",
	"trekking":    "trekking",
	"mtb":         "mtb",
	"gravel":      "gravel",
	"road":        "road",
}

// RouteSuggestionRequest is the request body for POST /api/v1/routes/suggest.
type RouteSuggestionRequest struct {
	// Start is [longitude, latitude] of the starting point.
	Start [2]float64 `json:"start"`
	// End is an optional fixed end point. If omitted the route returns to Start (round-trip).
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
	// Profile is the routing profile (e.g. "hiking", "mtb", "gravel").
	Profile string `json:"profile"`
	// PreferUnknown, when true, weights the via-point selection toward
	// trail segments the user has not visited yet (Terra Incognita).
	PreferUnknown bool `json:"prefer_unknown"`
	// LandscapePrefs maps landscape keywords to a preference weight (1–5). Reserved.
	LandscapePrefs map[string]int `json:"landscape_prefs,omitempty"`
	// SurfacePrefs maps surface types to a weight (1–5). Reserved.
	SurfacePrefs map[string]int `json:"surface_prefs,omitempty"`
}

// RouteSuggestion is a single suggested route variant returned by SuggestRoutes.
type RouteSuggestion struct {
	Variant    int             `json:"variant"`
	Profile    string          `json:"profile"`
	ViaPoint   [2]float64      `json:"via_point"`
	DistanceM  float64         `json:"distance_m"`
	AscentM    float64         `json:"ascent_m"`
	DescentM   float64         `json:"descent_m"`
	GeoJSON    json.RawMessage `json:"geojson"`
	IsFallback bool            `json:"is_fallback"`
}

// RouteSuggestionResponse is the JSON response for POST /api/v1/routes/suggest.
type RouteSuggestionResponse struct {
	Suggestions []RouteSuggestion `json:"suggestions"`
}

// SuggestRoutes generates up to 3 round-trip route suggestions starting from
// the given point. The algorithm:
//
//  1. Build a CostConfig for the requested BadRouter profile.
//  2. Run FindReachableWithCoords to discover nodes reachable within half the
//     target distance budget (outward leg).
//  3. Select up to 3 via-points from the reachable set that are spread in
//     different compass directions, with Terra Incognita bias when requested.
//  4. For each via-point, call RouteVia (Start → via → Start/End) in parallel
//     and collect the resulting GeoJSON + metrics.
//
// If the tiled graph is not loaded, the endpoint returns 503.
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

	// --- Get UnifiedRouter ---
	if h.GraphManager == nil {
		writeError(w, http.StatusServiceUnavailable,
			"Routing Unavailable",
			"Routing graph not configured. Import OSM data and build tiles first.",
			r.URL.Path)
		return
	}
	router := h.GraphManager.UnifiedRouter()
	if router == nil {
		writeError(w, http.StatusServiceUnavailable,
			"Routing Unavailable",
			"Tiled graph not loaded. Build routing tiles first via Settings → OSM.",
			r.URL.Path)
		return
	}

	// --- Map profile ---
	mappedProfile := profileMap[req.Profile]
	if mappedProfile == "" {
		mappedProfile = "hiking"
	}

	cfg, err := routing.BuildCostConfig(routing.ProfileConfig{Profile: mappedProfile})
	if err != nil {
		slog.Error("suggest routes: BuildCostConfig failed", "error", err, "profile", mappedProfile)
		writeError(w, http.StatusInternalServerError, "Routing Error", "failed to build cost config", r.URL.Path)
		return
	}

	// --- Compute half-budget ---
	halfDistKm := (req.TargetDistanceM / 1000.0) / 2.0
	halfBudgetCost := float32(halfDistKm) * cfg.AvgCostPerKm

	// --- Terra Incognita: load visited way IDs ---
	var visitedWayIDs map[int64]bool
	if req.PreferUnknown {
		rows, dbErr := h.DB.Pool.Query(r.Context(),
			`SELECT DISTINCT way_id FROM heatmap_trail_matches WHERE user_id = $1`, userID)
		if dbErr != nil {
			slog.Warn("suggest routes: could not load visited trail IDs", "error", dbErr)
		} else {
			defer rows.Close()
			visitedWayIDs = make(map[int64]bool)
			for rows.Next() {
				var wayID int64
				if scanErr := rows.Scan(&wayID); scanErr == nil {
					visitedWayIDs[wayID] = true
				}
			}
		}
	}

	// --- Step 1: FindReachableWithCoords (P18: BEFORE spawning variant goroutines) ---
	startLat := req.Start[1]
	startLon := req.Start[0]

	reachable, _, err := router.FindReachableWithCoords(r.Context(), startLat, startLon, cfg, halfBudgetCost, visitedWayIDs)
	if err != nil {
		slog.Error("suggest routes: FindReachableWithCoords failed", "error", err, "user_id", userID)
		writeError(w, http.StatusInternalServerError, "Routing Error", "reachability query failed", r.URL.Path)
		return
	}

	if len(reachable) == 0 {
		writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: []RouteSuggestion{}})
		return
	}

	// --- Step 2: Select up to 3 diverse via-points ---
	viaPoints := selectDiverseViaPoints(req, reachable, halfBudgetCost)

	if len(viaPoints) == 0 {
		writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: []RouteSuggestion{}})
		return
	}

	// --- Step 3: Route each via-point in parallel ---
	ctx60, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	g, gctx := errgroup.WithContext(ctx60)
	results := make([]*routing.RouteResult, len(viaPoints))
	vias := make([][2]float64, len(viaPoints))
	copy(vias, viaPoints)

	for i := range vias {
		g.Go(func() error {
			via := vias[i]
			waypoints := []routing.LatLon{
				{Lat: req.Start[1], Lon: req.Start[0]},
				{Lat: via[1], Lon: via[0]},
				{Lat: req.Start[1], Lon: req.Start[0]}, // round-trip default
			}
			if req.End != nil {
				waypoints[2] = routing.LatLon{Lat: req.End[1], Lon: req.End[0]}
			}
			res, routeErr := router.RouteVia(gctx, waypoints, cfg)
			if routeErr != nil {
				slog.Warn("suggest routes: variant routing failed", "variant", i+1, "error", routeErr)
				return nil // non-fatal
			}
			results[i] = res
			return nil
		})
	}
	_ = g.Wait()

	// --- Step 4: Convert results to suggestions ---
	var suggestions []RouteSuggestion
	for i, res := range results {
		if res == nil {
			continue
		}
		suggestions = append(suggestions, RouteSuggestion{
			Variant:    i + 1,
			Profile:    mappedProfile,
			ViaPoint:   vias[i],
			DistanceM:  res.DistanceM,
			AscentM:    res.AscentM,
			DescentM:   res.DescentM,
			GeoJSON:    json.RawMessage(res.ToGeoJSON()),
			IsFallback: false,
		})
	}

	if suggestions == nil {
		suggestions = []RouteSuggestion{}
	}

	writeJSON(w, http.StatusOK, RouteSuggestionResponse{Suggestions: suggestions})
}

// selectDiverseViaPoints selects up to 3 candidate via-points from the
// reachable node set. Candidates are filtered to nodes whose accumulated cost
// falls within [35%, 65%] of the half-budget (roughly the midpoint of the
// outward leg). The selection maximises angular spread around the start point
// to produce geometrically distinct route variants.
//
// When a node has Unknown=true, it is on an unvisited trail (Terra Incognita)
// and receives a selection bonus.
//
// When req.DirectionPoint is set, candidates within +/-60 deg of that bearing
// are preferred.
func selectDiverseViaPoints(
	req RouteSuggestionRequest,
	reachable []routing.ReachableNode,
	halfBudget float32,
) [][2]float64 {

	minCost := halfBudget * 0.35
	maxCost := halfBudget * 0.65

	type candidate struct {
		lngLat  [2]float64
		bearing float64
		unknown bool
	}

	buildPool := func(filterBand bool) []candidate {
		var pool []candidate
		for _, r := range reachable {
			if filterBand && (r.Cost < minCost || r.Cost > maxCost) {
				continue
			}
			bearing := bearingDeg(req.Start[1], req.Start[0], r.Lat, r.Lon)

			// Direction filter: prefer candidates within +/-60 deg of the requested bearing.
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
				lngLat:  [2]float64{r.Lon, r.Lat},
				bearing: bearing,
				unknown: r.Unknown,
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
			// Terra Incognita bias: 5 deg virtual spread bonus for unknown trails.
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

// suggestSpeed returns a rough average speed (km/h) for a given sport type,
// used to convert a target distance into a cost budget.
func suggestSpeed(sportType string) float64 {
	switch sportType {
	case "cycling", "road_cycling", "gravel_cycling":
		return 20.0
	case "mountain_biking":
		return 12.0
	case "running", "trail_running":
		return 10.0
	default:
		return 5.0
	}
}

// bearingDeg returns the initial bearing (0-360 deg) from (lat1, lon1) to
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
