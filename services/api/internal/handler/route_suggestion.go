package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/skadia-app/skadia/services/api/internal/database"
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
	MaxVariants *int `json:"max_variants,omitempty"`
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

	// Descriptors are dynamic variant labels (e.g. "more forest", "less ascent").
	// Always present as an array (may be empty); never null.
	Descriptors []VariantDescriptor `json:"variant_descriptors"`

	// Route aggregates (0.0–1.0). Propagated from RouteCandidate.MergedForestPct
	// and MergedProtectedArea (computed during coverage scoring).
	ForestPct    float32 `json:"forest_pct"`
	ProtectedPct float32 `json:"protected_pct"`

	// Range-based highlights per variant. omitempty because data may be absent
	// for e.g. approach-only routes. Coord indices reference GeoJSON.features[0]
	// .geometry.coordinates within the same suggestion.
	ForestRanges    []CoordRange            `json:"forest_ranges,omitempty"`
	ProtectedRanges []CoordRange            `json:"protected_ranges,omitempty"`
	SacRanges       map[string][]CoordRange `json:"sac_ranges,omitempty"`
	MtbRanges       map[string][]CoordRange `json:"mtb_ranges,omitempty"`
	// Via-Ferrata — keys "K1".."K6".
	// VfUnknownRanges: K?-Ranges (highway=via_ferrata AND scale=NULL), Invariant I9.
	VfRanges        map[string][]CoordRange `json:"vf_ranges,omitempty"`
	VfUnknownRanges []CoordRange            `json:"vf_unknown_ranges,omitempty"`

	// Summits and passes passed within the user-configured radius.
	// Uses handler-internal DTO (ResponsePassedSummit) to avoid leaking DB types.
	SummitsPassed []ResponsePassedSummit `json:"summits_passed,omitempty"`
}

// ResponsePassedSummit is a handler-internal DTO for database.PassedSummit.
// It avoids serialising DB types directly (DTO-layer isolation).
type ResponsePassedSummit struct {
	OsmID    int64   `json:"osm_id"`
	Name     string  `json:"name"`
	EleM     *int    `json:"ele_m,omitempty"`
	Category string  `json:"category,omitempty"`
	DistM    float64 `json:"dist_m"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
}

// toResponseSummit maps a database.PassedSummit to a ResponsePassedSummit DTO.
func toResponseSummit(s database.PassedSummit) ResponsePassedSummit {
	var eleM *int
	if s.EleM != nil {
		v := int(*s.EleM)
		eleM = &v
	}
	return ResponsePassedSummit{
		OsmID:    s.OsmID,
		Name:     s.Name,
		EleM:     eleM,
		Category: s.Category,
		DistM:    float64(s.DistM),
		Lat:      s.Lat,
		Lng:      s.Lng,
	}
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
//  3. Filter by distance band and select candidates via BuildCandidatePool + PickDiverseIndices.
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

	// --- MaxVariants default + clamp ---
	if req.MaxVariants == nil {
		v := 3
		req.MaxVariants = &v
	}
	if *req.MaxVariants < 1 {
		*req.MaxVariants = 1
	}
	if *req.MaxVariants > 5 {
		*req.MaxVariants = 5
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

	// Step 1: Pareto findReachable
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

	// Step 2: Load routing state (opts + ctx are request-scoped, not part of pool helper)
	opts := DefaultRoundTripOptions()
	ctx := r.Context()
	maxVariants := *req.MaxVariants

	// Step 3: Build candidate pool (filter + sort + truncate + diversify)
	filtered, pool := buildCandidatePool(reachable, startLat, startLon, req.TargetDistanceM, maxVariants)
	routingMetricDiversityPoolSize.Observe(float64(len(pool)))
	if len(pool) == 0 {
		httputil.WriteError(w, http.StatusNotFound, "Not Found", "no candidate pool built", r.URL.Path)
		return
	}

	// Step 4: Build all candidates in parallel via errgroup.
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
				slog.Warn("route candidate failed", "nodeIdx", sel.NodeIdx, "err", err)
				routingMetricCandidatesBuiltTotal.WithLabelValues("error").Inc()
				return nil // soft failure — skip this candidate
			}
			candidates[i] = c
			routingMetricCandidatesBuiltTotal.WithLabelValues("ok").Inc()
			return nil
		})
	}
	_ = eg.Wait()

	// Step 5: Collect successful candidates.
	built := make([]RouteCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c != nil {
			built = append(built, *c)
		}
	}
	if len(built) == 0 {
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error",
			"no routes built", r.URL.Path)
		return
	}

	// Step 6: Derive candidate metrics (nullable GreenScore / CalmScore).
	metrics := deriveCandidateMetrics(built)

	// Step 7: Pick diverse indices.
	selectedIdx := routing.PickDiverseIndices(metrics, req.TargetDistanceM, req.TargetAscentM, maxVariants)

	// Step 8: Assign labels and descriptors.
	labeled := assignLabels(built, selectedIdx, req.TargetDistanceM, coverageAvailable, mappedProfile)
	if coverageAvailable {
		routingMetricCoverageAvailable.WithLabelValues("true").Inc()
	} else {
		routingMetricCoverageAvailable.WithLabelValues("false").Inc()
	}

	for _, lr := range labeled {
		for _, d := range lr.Descriptors {
			routingMetricDescriptorAssigned.WithLabelValues(d.Key).Inc()
		}
	}

	// Step 9: Outdoor coverage gate (metric emit — once per response).
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

	// Step 10: Build and write response.
	// Load per-user summit detection radius once, before the variant loop.
	// userID already obtained from httputil.RequireUserID above.
	// h.DB is nil when the handler is constructed without a database (e.g. offline
	// routing-only deployments or unit-test harnesses that do not provision DB).
	// In that case summit detection is skipped and the default radius is used.
	radiusM := 80.0 // Default fallback (matches Migration 000315 DEFAULT)
	if h.DB != nil && userID != "" {
		if settings, err := h.DB.GetEnrichmentSettings(r.Context(), userID); err == nil && settings != nil {
			radiusM = settings.SummitDetectionRadiusM
		} else if err != nil {
			slog.Warn("get enrichment settings failed; falling back to default radius",
				"user_id", userID, "error", err)
		}
	}

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

		descriptors := lr.Descriptors
		if descriptors == nil {
			descriptors = []VariantDescriptor{}
		}

		// Range computation per variant.
		lastCoordIdx := len(merged.Coordinates) - 1

		forestRanges := computeForestRanges(merged.EdgeCoordStart, merged.EdgeTags2, lastCoordIdx)
		protectedRanges := computeProtectedRanges(merged.EdgeCoordStart, merged.EdgeTags2, lastCoordIdx)

		sacRanges := make(map[string][]CoordRange)
		for _, level := range []string{"T1", "T2", "T3", "T4", "T5", "T6"} {
			if r := computeScaleRanges(merged.EdgeCoordStart, merged.EdgeTags1, lastCoordIdx, "sac", level); len(r) > 0 {
				sacRanges[level] = r
			}
		}
		mtbRanges := make(map[string][]CoordRange)
		for _, level := range []string{"S0", "S1", "S2", "S3", "S4", "S5"} {
			if r := computeScaleRanges(merged.EdgeCoordStart, merged.EdgeTags1, lastCoordIdx, "mtb", level); len(r) > 0 {
				mtbRanges[level] = r
			}
		}
		if len(sacRanges) == 0 {
			sacRanges = nil
		}
		if len(mtbRanges) == 0 {
			mtbRanges = nil
		}

		vfRanges := make(map[string][]CoordRange)
		for _, level := range []string{"K1", "K2", "K3", "K4", "K5", "K6"} {
			if r := computeVfRanges(merged.EdgeCoordStart, merged.EdgeTags2, lastCoordIdx, level); len(r) > 0 {
				vfRanges[level] = r
			}
		}
		if len(vfRanges) == 0 {
			vfRanges = nil
		}
		vfUnknownRanges := computeVfUnknownRanges(merged.EdgeCoordStart, merged.EdgeTags1, merged.EdgeTags2, lastCoordIdx)

		// Summit detection per variant.
		// Requires a live DB connection (h.DB non-nil, h.DB.Pool non-nil).
		// Skipped gracefully for offline/test deployments where DB is unavailable.
		var summitsPassed []database.PassedSummit
		if h.DB != nil && h.DB.Pool != nil && radiusM > 0 && len(merged.Coordinates) >= 2 {
			wkbBytes, wkbErr := coordsToLineStringWKB(merged.Coordinates)
			if wkbErr != nil {
				if errors.Is(wkbErr, errTooFewCoords) {
					// Approach-only route: normal corner-case, silent skip.
				} else {
					slog.Warn("summit-along-geom: WKB failed", "err", wkbErr, "variant", lr.VariantID)
				}
			} else {
				var err error
				summitsPassed, err = database.FindSummitsAlongGeom(r.Context(), h.DB.Pool, wkbBytes, radiusM)
				if err != nil {
					slog.Warn("summits along route failed", "variant", lr.VariantID, "error", err)
					summitsPassed = nil // per-variant isolation (Spec §4.1.5)
				}
			}
		}
		responseSummits := make([]ResponsePassedSummit, 0, len(summitsPassed))
		for _, s := range summitsPassed {
			responseSummits = append(responseSummits, toResponseSummit(s))
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
			Descriptors:            descriptors,
			ForestPct:              float32(c.MergedForestPct),
			ProtectedPct:           float32(c.MergedProtectedArea),
			ForestRanges:           forestRanges,
			ProtectedRanges:        protectedRanges,
			SacRanges:              sacRanges,
			MtbRanges:              mtbRanges,
			VfRanges:               vfRanges,
			VfUnknownRanges:        vfUnknownRanges,
			SummitsPassed:          responseSummits,
		})
	}

	// assignLabels already sorts by DistanceM; no redundant sort needed here.

	httputil.WriteJSON(w, http.StatusOK, RouteSuggestionResponse{
		Suggestions:       suggestions,
		CoverageAvailable: coverageAvailable,
		PartialSearch:     truncated,
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

	res, err := suggestRoundTripVariant(ctx, router, rs, startLat, startLon, &reachable[sel.NodeIdx], sel.StateIdx, cfg, opts, baselineAvgFlat)
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

// buildCandidatePool applies distance-band filtering, deterministic sorting,
// hard-size truncation (O(n²) guard in BuildCandidatePool), and delegates to
// routing.BuildCandidatePool. Returns the filtered reachable-set and the
// selected candidate pool.
//
// Pure computation — emits only prometheus metrics (truncation counter) and
// slog warnings on truncation. No HTTP writes.
func buildCandidatePool(
	reachable []routing.ReachableNode,
	startLat, startLon float64,
	targetDistanceM float64,
	maxVariants int,
) (filtered []routing.ReachableNode, pool []routing.CandidateSelection) {
	targetHalfDistM := targetDistanceM / 2.0
	filtered = filterByDistanceBand(reachable, targetHalfDistM, 0.5, 1.5)
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

	// Hard limit to prevent O(n²) in BuildCandidatePool with huge pools.
	const maxVariantPool = 10000
	if len(filtered) > maxVariantPool {
		originalSize := len(filtered)
		filtered = filtered[:maxVariantPool]
		routingMetricCandidatePoolTruncated.Inc()
		slog.Warn("candidate pool truncated", "limit", maxVariantPool, "original_size", originalSize)
	}

	poolSize := maxVariants + 2
	if poolSize > 7 {
		poolSize = 7
	}

	minDist := routing.MinVariantDistanceM(targetDistanceM)
	pool = routing.BuildCandidatePool(filtered, startLat, startLon, targetDistanceM, poolSize, minDist)
	return filtered, pool
}

// deriveCandidateMetrics converts built RouteCandidates into CandidateMetrics,
// deriving nullable GreenScore (avg of forest/river/protected-area fractions)
// and CalmScore (1 - avg of clamped traffic + noise). Coverage-less candidates
// produce nil scores.
//
// Pure computation. No external effects.
func deriveCandidateMetrics(built []RouteCandidate) []routing.CandidateMetrics {
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
	return metrics
}

