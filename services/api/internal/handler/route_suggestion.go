package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
)

// LandscapePreferences holds per-dimension landscape preference weights (0–10).
type LandscapePreferences struct {
	Forest     int `json:"forest"`      // 0-10
	Water      int `json:"water"`       // 0-10 (uses RiverProximity including lakes)
	Protected  int `json:"protected"`   // 0-10
	LowNoise   int `json:"low_noise"`   // 0-10
	LowTraffic int `json:"low_traffic"` // 0-10
}

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
	// SportType is the FIT sport type (e.g. "hiking", "cycling").
	SportType string `json:"sport_type"`
	// Profile is the routing profile (e.g. "hiking", "mtb", "gravel") or a custom profile name.
	Profile string `json:"profile"`
	// PreferUnknown, when true, weights the via-point selection toward
	// trail segments the user has not visited yet (Terra Incognita).
	PreferUnknown bool `json:"prefer_unknown"`
	// LandscapePrefs holds per-dimension landscape preference weights (0–10).
	LandscapePrefs *LandscapePreferences `json:"landscape_prefs,omitempty"`
	// SurfaceTendency selects a surface-type preset: "trails", "forest_roads", "paved", or "mixed".
	SurfaceTendency string `json:"surface_tendency,omitempty"`
	// MaxSacScale overrides the profile's SAC difficulty limit (0–6).
	MaxSacScale *int `json:"max_sac_scale,omitempty"`
	// MaxMtbScale overrides the profile's MTB difficulty limit (0–5).
	MaxMtbScale *int `json:"max_mtb_scale,omitempty"`
	// Ascent targeting
	TargetAscentM      *float64 `json:"target_ascent_m,omitempty"`
	AscentTolerancePct *float64 `json:"ascent_tolerance_pct,omitempty"`
	// Variant control
	MaxVariants *int     `json:"max_variants,omitempty"`
	VariantIDs  []string `json:"variant_ids,omitempty"`
}

// RouteSuggestion is a single suggested route variant returned by SuggestRoutes.
type RouteSuggestion struct {
	Variant  int        `json:"variant"`
	Profile  string     `json:"profile"`
	ViaPoint [2]float64 `json:"via_point"`

	VariantID       string `json:"variant_id"`
	VariantLabelKey string `json:"variant_label_key"`

	DistanceM float64 `json:"distance_m"`
	AscentM   float64 `json:"ascent_m"`
	DescentM  float64 `json:"descent_m"`
	AvgFlat   float64 `json:"avg_flat"`

	LoopQuality string  `json:"loop_quality"`
	OverlapPct  float64 `json:"overlap_pct"`

	DistanceTargetAchieved bool `json:"distance_target_achieved"`
	AscentTargetAchieved   bool `json:"ascent_target_achieved"`

	GeoJSON    json.RawMessage `json:"geojson"`
	IsFallback bool            `json:"is_fallback"`

	Descriptors []VariantDescriptor `json:"variant_descriptors,omitempty"`
}

// RouteSuggestionResponse is the JSON response for POST /api/v1/routes/suggest.
type RouteSuggestionResponse struct {
	Suggestions       []RouteSuggestion `json:"suggestions"`
	CoverageAvailable bool              `json:"coverage_available"`
	PartialSearch     bool              `json:"partial_search"`
}

// SuggestRoutes generates round-trip route suggestions starting from the given
// point. The algorithm:
//
//  1. Build a CostConfig for the requested BadRouter profile.
//  2. Run Pareto FindReachableWithCoords to discover nodes reachable within
//     half the target distance budget (outward leg).
//  3. Filter by distance band and select semantic variants via SelectVariants.
//  4. For each variant, build a round-trip with quality gate in parallel.
//  5. Classify loop quality, compute overlap, and return enriched suggestions.
//
// If the tiled graph is not loaded, the endpoint returns 503.
//
// POST /api/v1/routes/suggest
func (h *Handler) SuggestRoutes(w http.ResponseWriter, r *http.Request) {
	suggestStart := time.Now()
	defer func() {
		routing.SuggestRoutesDuration.WithLabelValues("total").Observe(time.Since(suggestStart).Seconds())
	}()

	userID, ok := httputil.RequireUserID(w, r)
	if !ok {
		return
	}

	req, ok := httputil.DecodeJSON[RouteSuggestionRequest](w, r)
	if !ok {
		return
	}

	// --- Input validation ---
	if req.Start[0] < -180 || req.Start[0] > 180 || req.Start[1] < -90 || req.Start[1] > 90 {
		httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "start coordinates out of range", r.URL.Path)
		return
	}
	if req.End != nil {
		if req.End[0] < -180 || req.End[0] > 180 || req.End[1] < -90 || req.End[1] > 90 {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request", "end coordinates out of range", r.URL.Path)
			return
		}
	}
	if req.TargetDistanceM <= 0 {
		req.TargetDistanceM = 10000 // default 10 km
	}
	if req.SurfaceTendency != "" {
		validTendencies := map[string]bool{"trails": true, "forest_roads": true, "mixed": true, "paved": true}
		if !validTendencies[req.SurfaceTendency] {
			httputil.WriteError(w, http.StatusBadRequest, "Bad Request",
				"surface_tendency must be one of: trails, forest_roads, mixed, paved", r.URL.Path)
			return
		}
	}

	// --- Clamp LandscapePreferences to 0–10 ---
	if req.LandscapePrefs != nil {
		lp := req.LandscapePrefs
		clamp := func(v *int) {
			if *v < 0 {
				*v = 0
			}
			if *v > 10 {
				*v = 10
			}
		}
		clamp(&lp.Forest)
		clamp(&lp.Water)
		clamp(&lp.Protected)
		clamp(&lp.LowNoise)
		clamp(&lp.LowTraffic)
	}

	// --- MaxVariants default ---
	if req.MaxVariants == nil {
		v := 3
		req.MaxVariants = &v
	}

	// --- Get UnifiedRouter ---
	if h.GraphManager == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable,
			"Routing Unavailable",
			"Routing graph not configured. Import OSM data and build tiles first.",
			r.URL.Path)
		return
	}
	router := h.GraphManager.UnifiedRouter()
	if router == nil {
		httputil.WriteError(w, http.StatusServiceUnavailable,
			"Routing Unavailable",
			"Tiled graph not loaded. Build routing tiles first via Settings → OSM.",
			r.URL.Path)
		return
	}

	// --- Map profile ---
	mappedProfile := profileMap[req.Profile]
	profileCfg := routing.ProfileConfig{Profile: mappedProfile}

	// If profile not in system defaults, try loading custom profile from DB.
	if mappedProfile == "" {
		customProfile, dbErr := h.DB.GetRoutingProfileByNameOrGlobal(r.Context(), userID, req.Profile)
		if dbErr == nil && customProfile != nil {
			_ = json.Unmarshal(customProfile.Config, &profileCfg)
			profileCfg.Profile = customProfile.BaseProfile
			mappedProfile = customProfile.BaseProfile
		} else {
			mappedProfile = "hiking" // fallback
			profileCfg.Profile = "hiking"
		}
	}

	if req.MaxSacScale != nil {
		v := max(0, min(*req.MaxSacScale, 6))
		req.MaxSacScale = &v
		profileCfg.MaxSacScale = req.MaxSacScale
	}
	if req.MaxMtbScale != nil {
		v := max(0, min(*req.MaxMtbScale, 5))
		req.MaxMtbScale = &v
		profileCfg.MaxMtbScale = req.MaxMtbScale
	}
	routing.ApplySurfaceTendency(&profileCfg, req.SurfaceTendency)

	cfg, err := routing.BuildCostConfig(profileCfg)
	if err != nil {
		slog.Error("suggest routes: BuildCostConfig failed", "error", err, "profile", mappedProfile)
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error", "failed to build cost config", r.URL.Path)
		return
	}
	baselineAvgFlat := routing.ProfileBaselineAvgFlat(mappedProfile)

	// Start coordinates (needed for budget computation and FindReachableWithCoords)
	startLat := req.Start[1]
	startLon := req.Start[0]

	// --- Budget ---
	halfDistKm := (req.TargetDistanceM / 1000.0) / 2.0
	halfBudgetCost := float32(halfDistKm) * cfg.AvgCostPerKm * 1.5

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

	// --- Step 1: Pareto findReachable ---
	reachable, rs, truncated, err := router.FindReachableWithCoords(
		r.Context(), startLat, startLon, cfg, halfBudgetCost, visitedWayIDs)
	if err != nil {
		slog.Error("suggest routes: FindReachableWithCoords failed", "error", err, "user_id", userID)
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error", "reachability query failed", r.URL.Path)
		return
	}

	coverageAvailable := h.CoverageReader != nil

	if len(reachable) == 0 {
		httputil.WriteJSON(w, http.StatusOK, RouteSuggestionResponse{
			Suggestions: []RouteSuggestion{}, CoverageAvailable: coverageAvailable, PartialSearch: truncated,
		})
		return
	}

	// --- Step 2: Filter by distance band ---
	targetHalfDistM := req.TargetDistanceM / 2.0
	filtered := filterByDistanceBand(reachable, targetHalfDistM, 0.5, 1.5)
	if len(filtered) == 0 {
		filtered = reachable
	}

	// Sort filtered nodes by NodeID for deterministic variant selection across calls.
	slices.SortFunc(filtered, func(a, b routing.ReachableNode) int {
		if a.NodeID < b.NodeID {
			return -1
		}
		if a.NodeID > b.NodeID {
			return 1
		}
		return 0
	})

	// Hard limit to prevent O(n²) in SelectVariants with huge pools.
	const maxVariantPool = 10000
	if len(filtered) > maxVariantPool {
		filtered = filtered[:maxVariantPool]
	}

	// --- Step 3: Select semantic variants ---
	selCtx := routing.VariantSelectionContext{
		TargetDistanceM: req.TargetDistanceM,
		TargetAscentM:   req.TargetAscentM,
	}
	if req.LandscapePrefs != nil {
		selCtx.LandscapePrefs = &routing.LandscapePrefsInternal{
			Forest:     req.LandscapePrefs.Forest,
			Water:      req.LandscapePrefs.Water,
			Protected:  req.LandscapePrefs.Protected,
			LowNoise:   req.LandscapePrefs.LowNoise,
			LowTraffic: req.LandscapePrefs.LowTraffic,
		}
	}
	selected := routing.SelectVariants(filtered, startLat, startLon,
		routing.DefaultVariants, *req.MaxVariants, selCtx)

	if len(selected) == 0 {
		httputil.WriteJSON(w, http.StatusOK, RouteSuggestionResponse{
			Suggestions: []RouteSuggestion{}, CoverageAvailable: coverageAvailable, PartialSearch: truncated,
		})
		return
	}

	// --- Step 4: Build round-trip routes for each variant (parallel) ---
	ctx60, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	opts := DefaultRoundTripOptions()
	results := make([]*RoundTripResult, len(selected))
	g, gctx := errgroup.WithContext(ctx60)

	for i := range selected {
		g.Go(func() error {
			res, err := suggestRoundTripVariant(gctx, router, rs,
				startLat, startLon, selected[i], cfg, opts, baselineAvgFlat)
			if err != nil {
				slog.Warn("round trip variant failed",
					"variant", selected[i].Variant.ID, "error", err)
				return nil
			}
			results[i] = res
			return nil
		})
	}
	_ = g.Wait()

	// Check if all variants failed — return 500 instead of an empty 200.
	allFailed := true
	for _, res := range results {
		if res != nil {
			allFailed = false
			break
		}
	}
	if allFailed && len(selected) > 0 {
		slog.Error("suggest routes: all variant constructions failed", "variants", len(selected))
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error",
			"all route variants failed to compute", r.URL.Path)
		return
	}

	// --- Step 5: Build response ---
	var suggestions []RouteSuggestion
	for i, res := range results {
		if res == nil {
			continue
		}
		sv := selected[i]

		merged := routing.MergeLegs([]*routing.RouteResult{res.Leg1, res.Leg2})

		distanceAchieved := math.Abs(merged.DistanceM-req.TargetDistanceM) <= 0.15*req.TargetDistanceM
		ascentAchieved := true
		if req.TargetAscentM != nil {
			tolerance := 0.2 // default 20%
			if req.AscentTolerancePct != nil {
				tolerance = *req.AscentTolerancePct / 100.0 // convert percent to factor (e.g. 20 → 0.20)
			}
			ascentAchieved = math.Abs(merged.AscentM-*req.TargetAscentM) <= tolerance**req.TargetAscentM
		}

		suggestions = append(suggestions, RouteSuggestion{
			Variant:                i + 1,
			Profile:                mappedProfile,
			ViaPoint:               [2]float64{sv.Node.Lon, sv.Node.Lat},
			VariantID:              sv.Variant.ID,
			VariantLabelKey:        sv.Variant.LabelKey,
			DistanceM:              merged.DistanceM,
			AscentM:                merged.AscentM,
			DescentM:               merged.DescentM,
			AvgFlat:                float64(merged.AvgFlat()),
			LoopQuality:            string(res.LoopQuality),
			OverlapPct:             res.OverlapPct,
			DistanceTargetAchieved: distanceAchieved,
			AscentTargetAchieved:   ascentAchieved,
			GeoJSON:                json.RawMessage(merged.ToGeoJSON()),
			IsFallback:             res.LoopQuality == LoopQualityOutAndBack,
		})
	}

	if suggestions == nil {
		suggestions = []RouteSuggestion{}
	}

	httputil.WriteJSON(w, http.StatusOK, RouteSuggestionResponse{
		Suggestions: suggestions, CoverageAvailable: coverageAvailable, PartialSearch: truncated,
	})
}

// filterByDistanceBand selects reachable nodes where at least one Pareto state
// distance falls within [target*lowPct, target*highPct].
func filterByDistanceBand(reachable []routing.ReachableNode, target, lowPct, highPct float64) []routing.ReachableNode {
	var out []routing.ReachableNode
	lowM := float32(target * lowPct)
	highM := float32(target * highPct)
	for _, rn := range reachable {
		for _, s := range rn.States {
			if s.DistanceM >= lowM && s.DistanceM <= highM {
				out = append(out, rn)
				break // node has at least one matching state
			}
		}
	}
	return out
}
