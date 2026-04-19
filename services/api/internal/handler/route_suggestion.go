package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strings"
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

	// Descriptors are dynamic variant labels (e.g. "more forest", "less ascent")
	// populated only when the routing.route_variants_v2 feature flag is enabled.
	// Empty/nil for v1 responses; clients should tolerate its absence.
	Descriptors []VariantDescriptor `json:"variant_descriptors,omitempty"`
}

// RouteSuggestionResponse is the JSON response for POST /api/v1/routes/suggest.
type RouteSuggestionResponse struct {
	Suggestions       []RouteSuggestion `json:"suggestions"`
	CoverageAvailable bool              `json:"coverage_available"`
	PartialSearch     bool              `json:"partial_search"`
	UsedFallback      bool              `json:"used_fallback,omitempty"`
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

	opts := DefaultRoundTripOptions()

	// --- Feature flag: route_variants_v2 ---
	useV2 := h.Settings != nil && h.Settings.Get("routing.route_variants_v2").Value == "true"
	if useV2 {
		h.suggestRoutesV2Tail(w, r, &req, userID, mappedProfile, filtered,
			startLat, startLon, rs, router, cfg, opts, baselineAvgFlat, coverageAvailable, truncated)
		return
	}
	h.suggestRoutesV1Tail(w, r, &req, userID, mappedProfile, filtered,
		startLat, startLon, rs, router, cfg, opts, baselineAvgFlat, coverageAvailable, truncated)
}

// suggestRoutesV1Tail is the legacy (v1) variant-selection pipeline.
// It is called when the routing.route_variants_v2 feature flag is "false" (default).
func (h *Handler) suggestRoutesV1Tail(
	w http.ResponseWriter, r *http.Request,
	req *RouteSuggestionRequest, _ string,
	mappedProfile string,
	filtered []routing.ReachableNode,
	startLat, startLon float64,
	rs *routing.ReachableState,
	router *routing.UnifiedRouter,
	cfg *routing.CostConfig,
	opts RoundTripOptions,
	baselineAvgFlat float32,
	coverageAvailable bool,
	truncated bool,
) {
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

// suggestRoutesV2Tail is the new (v2) variant pipeline: BuildCandidatePool →
// errgroup buildRouteCandidate → CandidateMetrics → PickDiverseIndices →
// assignLabels → VariantIDs filter → response.
// It is called when the routing.route_variants_v2 feature flag is "true".
func (h *Handler) suggestRoutesV2Tail(
	w http.ResponseWriter, r *http.Request,
	req *RouteSuggestionRequest, _ string,
	mappedProfile string,
	filtered []routing.ReachableNode,
	startLat, startLon float64,
	rs *routing.ReachableState,
	router *routing.UnifiedRouter,
	cfg *routing.CostConfig,
	opts RoundTripOptions,
	baselineAvgFlat float32,
	coverageAvailable bool,
	truncated bool,
) {
	ctx := r.Context()

	// 1. Pool size: maxVariants + 2, capped at 7.
	maxVariants := *req.MaxVariants
	poolSize := maxVariants + 2
	if poolSize > 7 {
		poolSize = 7
	}

	// 2. Build candidate pool.
	minDist := routing.MinVariantDistanceM(req.TargetDistanceM)
	pool := routing.BuildCandidatePool(filtered, startLat, startLon, req.TargetDistanceM, poolSize, minDist)
	routingMetricDiversityPoolSize.Observe(float64(len(pool)))
	if len(pool) == 0 {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "no candidate pool built", r.URL.Path)
		return
	}

	// 3. Build all candidates in parallel via errgroup.
	ctx60, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	candidates := make([]*RouteCandidate, len(pool))
	var eg errgroup.Group
	eg.SetLimit(h.GraphManager.MaxConcurrentRoutes())
	for i, sel := range pool {
		i, sel := i, sel
		eg.Go(func() error {
			c, err := h.buildRouteCandidate(ctx60, router, rs, filtered, sel,
				startLat, startLon, cfg, opts, baselineAvgFlat)
			if err != nil {
				slog.Warn("v2: route candidate failed", "nodeIdx", sel.NodeIdx, "err", err)
				routingMetricCandidatesBuiltTotal.WithLabelValues("error").Inc()
				return nil // soft failure — skip this candidate
			}
			candidates[i] = c
			routingMetricCandidatesBuiltTotal.WithLabelValues("ok").Inc()
			return nil
		})
	}
	_ = eg.Wait()

	// 4. Collect successful candidates.
	built := make([]RouteCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c != nil {
			built = append(built, *c)
		}
	}
	if len(built) == 0 {
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error",
			"no routes built in v2 pipeline", r.URL.Path)
		return
	}

	// 5. Build CandidateMetrics with nullable GreenScore / CalmScore.
	metrics := make([]routing.CandidateMetrics, len(built))
	for i, c := range built {
		m := routing.CandidateMetrics{
			DistanceM: c.DistanceM,
			AscentM:   c.AscentM,
			AvgFlat:   c.AvgFlat,
			NodeID:    c.NodeID,
		}
		if c.HasCoverage && (c.MergedForestPct > 0 || c.MergedRiverProximity > 0 || c.MergedProtectedArea > 0) {
			green := (c.MergedForestPct + c.MergedRiverProximity + c.MergedProtectedArea) / 3.0
			m.GreenScore = &green
		}
		if c.HasCoverage && (c.MergedTrafficClass > 0 || c.MergedNoiseScore > 0) {
			trafficClamped := c.MergedTrafficClass
			if trafficClamped > 1.0 {
				trafficClamped = 1.0
			}
			calm := 1.0 - (trafficClamped+c.MergedNoiseScore)/2.0
			if calm < 0 {
				calm = 0
			}
			if calm > 1 {
				calm = 1
			}
			m.CalmScore = &calm
		}
		metrics[i] = m
	}

	// 6. Diversity selection.
	selectedIdx := routing.PickDiverseIndices(metrics, req.TargetDistanceM, req.TargetAscentM, maxVariants)

	// 7. Assign labels and descriptors.
	labeled := assignLabels(built, selectedIdx, req.TargetDistanceM, coverageAvailable, mappedProfile)
	if coverageAvailable {
		routingMetricCoverageAvailable.WithLabelValues("true").Inc()
	} else {
		routingMetricCoverageAvailable.WithLabelValues("false").Inc()
	}

	// 8. Optional VariantIDs filter (deprecated-ID-safe).
	var usedFallback bool
	if len(req.VariantIDs) > 0 {
		validIDs := map[string]bool{"shortest": true, "medium": true, "longest": true, "only": true}
		var keep, deprecated, unknown []string
		for _, id := range req.VariantIDs {
			if validIDs[id] {
				keep = append(keep, id)
			} else if isDeprecatedVariantID(id) {
				deprecated = append(deprecated, id)
			} else {
				unknown = append(unknown, id)
			}
		}
		if len(deprecated) > 0 {
			slog.Warn("v2: deprecated variant_id in request", "ids", deprecated)
		}
		if len(unknown) > 0 {
			slog.Warn("v2: unknown variant_id in request", "ids", unknown)
		}
		if len(keep) == 0 {
			// All requested IDs were deprecated or unknown — return all variants.
			usedFallback = true
		} else {
			keepSet := make(map[string]bool, len(keep))
			for _, k := range keep {
				keepSet[k] = true
			}
			filteredLabeled := make([]LabeledRoute, 0, len(labeled))
			for _, lr := range labeled {
				if keepSet[lr.VariantID] {
					filteredLabeled = append(filteredLabeled, lr)
				} else if keepSet["medium"] && strings.HasPrefix(lr.VariantID, "medium") {
					filteredLabeled = append(filteredLabeled, lr)
				}
			}
			labeled = filteredLabeled
		}
	}

	// 9. Descriptor metrics.
	for _, lr := range labeled {
		for _, d := range lr.Descriptors {
			routingMetricDescriptorAssigned.WithLabelValues(d.Key).Inc()
		}
	}

	// 9b. F26 outdoor coverage availability — once per response.
	// Thresholds are centralised in route_labels.go — do not hardcode here.
	outdoorGate := defaultOutdoorThresholds.CoverageGatePct
	exposedGate := exposedCoverageFloor
	anyOutdoor := false
	for _, lr := range labeled {
		c := lr.Candidate
		if c.TaggedPctMtb >= outdoorGate ||
			c.TaggedPctSac >= outdoorGate ||
			c.TaggedPctTv >= outdoorGate ||
			c.TaggedPctVia >= exposedGate {
			anyOutdoor = true
			break
		}
	}
	label := "false"
	if anyOutdoor {
		label = "true"
	}
	routingMetricOutdoorCoverageAvailable.WithLabelValues(label).Inc()

	// 10. Build response suggestions.
	suggestions := make([]RouteSuggestion, 0, len(labeled))
	for i, lr := range labeled {
		c := lr.Candidate
		merged := c.Merged

		distanceAchieved := math.Abs(c.DistanceM-req.TargetDistanceM) <= 0.15*req.TargetDistanceM
		ascentAchieved := true
		if req.TargetAscentM != nil {
			tolerance := 0.2
			if req.AscentTolerancePct != nil {
				tolerance = *req.AscentTolerancePct / 100.0
			}
			ascentAchieved = math.Abs(c.AscentM-*req.TargetAscentM) <= tolerance**req.TargetAscentM
		}

		suggestions = append(suggestions, RouteSuggestion{
			Variant:                i + 1,
			Profile:                mappedProfile,
			ViaPoint:               [2]float64{c.ViaLon, c.ViaLat},
			VariantID:              lr.VariantID,
			VariantLabelKey:        lr.VariantLabelKey,
			DistanceM:              c.DistanceM,
			AscentM:                c.AscentM,
			DescentM:               c.DescentM,
			AvgFlat:                float64(c.AvgFlat),
			LoopQuality:            string(c.LoopQuality),
			OverlapPct:             c.OverlapPct,
			DistanceTargetAchieved: distanceAchieved,
			AscentTargetAchieved:   ascentAchieved,
			GeoJSON:                json.RawMessage(merged.ToGeoJSON()),
			IsFallback:             c.LoopQuality == LoopQualityOutAndBack,
			Descriptors:            lr.Descriptors,
		})
	}

	// assignLabels already sorts by DistanceM; no redundant sort needed here.

	httputil.WriteJSON(w, http.StatusOK, RouteSuggestionResponse{
		Suggestions:       suggestions,
		CoverageAvailable: coverageAvailable,
		PartialSearch:     truncated,
		UsedFallback:      usedFallback,
	})
}

// buildRouteCandidate constructs a RouteCandidate from a CandidateSelection by
// calling suggestRoundTripVariant and merging coverage from Leg-1 (Pareto state)
// and Leg-2 (edge-level aggregation).
func (h *Handler) buildRouteCandidate(
	ctx context.Context,
	router *routing.UnifiedRouter,
	rs *routing.ReachableState,
	reachable []routing.ReachableNode,
	sel routing.CandidateSelection,
	startLat, startLon float64,
	cfg *routing.CostConfig,
	opts RoundTripOptions,
	baselineAvgFlat float32,
) (*RouteCandidate, error) {
	if sel.NodeIdx < 0 || sel.NodeIdx >= len(reachable) {
		return nil, fmt.Errorf("buildRouteCandidate: NodeIdx %d out of range (len=%d)",
			sel.NodeIdx, len(reachable))
	}
	if sel.StateIdx < 0 || sel.StateIdx >= len(reachable[sel.NodeIdx].States) {
		return nil, fmt.Errorf("buildRouteCandidate: StateIdx %d out of range for node %d (len=%d)",
			sel.StateIdx, sel.NodeIdx, len(reachable[sel.NodeIdx].States))
	}

	svCompat := routing.SelectedVariant{
		Variant:  routing.SemanticVariant{ID: "pool"},
		Node:     &reachable[sel.NodeIdx],
		StateIdx: sel.StateIdx,
	}

	res, err := suggestRoundTripVariant(ctx, router, rs, startLat, startLon, svCompat, cfg, opts, baselineAvgFlat)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}

	merged := routing.MergeLegs([]*routing.RouteResult{res.Leg1, res.Leg2})

	// Leg-1 coverage from Pareto state (distance-weighted averages already computed).
	state := &reachable[sel.NodeIdx].States[sel.StateIdx]
	leg1Dist := state.DistanceM

	// Leg-2 coverage via edge-level aggregation.
	leg2Cov := aggregateLeg2Coverage(res.Leg2.EdgeHandles, router.Reader())

	mergedForest := mergeCoverage(state.ForestPct, leg1Dist, leg2Cov.Forest, leg2Cov.DistM)
	mergedNoise := mergeCoverage(state.NoiseScore, leg1Dist, leg2Cov.Noise, leg2Cov.DistM)
	mergedTraffic := mergeCoverage(state.TrafficClass, leg1Dist, leg2Cov.Traffic, leg2Cov.DistM)
	mergedProtected := mergeCoverage(state.ProtectedArea, leg1Dist, leg2Cov.Protected, leg2Cov.DistM)
	mergedRiver := mergeCoverage(state.RiverProximity, leg1Dist, leg2Cov.River, leg2Cov.DistM)

	hasCov := leg2Cov.HasAnyCov ||
		state.ForestPct > 0 ||
		state.NoiseScore > 0 ||
		state.TrafficClass > 0 ||
		state.ProtectedArea > 0 ||
		state.RiverProximity > 0

	// F26: outdoor-tag aggregation.
	outdoor := aggregateLegOutdoor(merged.EdgeHandles, router.Reader())
	var taggedPctMtb, taggedPctSac, taggedPctVia, taggedPctTv, intermediatePctTv float32
	if outdoor.DistM > 0 {
		taggedPctMtb = outdoor.TaggedDistMtb / outdoor.DistM
		taggedPctSac = outdoor.TaggedDistSac / outdoor.DistM
		taggedPctVia = outdoor.TaggedDistVia / outdoor.DistM
		taggedPctTv = outdoor.TaggedDistTv / outdoor.DistM
		intermediatePctTv = outdoor.IntermediateDistTv / outdoor.DistM
	}

	return &RouteCandidate{
		NodeID:               reachable[sel.NodeIdx].NodeID,
		ViaLat:               reachable[sel.NodeIdx].Lat,
		ViaLon:               reachable[sel.NodeIdx].Lon,
		Leg1:                 res.Leg1,
		Leg2:                 res.Leg2,
		Merged:               merged,
		DistanceM:            merged.DistanceM,
		AscentM:              merged.AscentM,
		DescentM:             merged.DescentM,
		AvgFlat:              merged.AvgFlat(),
		MergedForestPct:      mergedForest,
		MergedRiverProximity: mergedRiver,
		MergedProtectedArea:  mergedProtected,
		MergedNoiseScore:     mergedNoise,
		MergedTrafficClass:   mergedTraffic,
		HasCoverage:          hasCov,
		LoopQuality:          res.LoopQuality,
		OverlapPct:           res.OverlapPct,

		// F26 outdoor
		MaxMtbScale:          outdoor.MaxMtbScale,
		MaxSacScale:          outdoor.MaxSacScale,
		MaxViaFerrataScale:   outdoor.MaxViaFerrataScale,
		HasViaFerrataHighway: outdoor.HasViaFerrataHighway,
		MaxTrailVisibility:   outdoor.MaxTrailVisibility,
		IntermediatePctTv:    intermediatePctTv,
		TaggedPctMtb:         taggedPctMtb,
		TaggedPctSac:         taggedPctSac,
		TaggedPctVia:         taggedPctVia,
		TaggedPctTv:          taggedPctTv,
	}, nil
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

// isDeprecatedVariantID reports whether id is a v1-era variant identifier that
// is no longer valid in the v2 pipeline. These are recognised and logged
// separately from genuinely unknown IDs.
func isDeprecatedVariantID(id string) bool {
	switch id {
	case "calmest", "greenest", "hilly", "balanced", "geometric":
		return true
	}
	return false
}
