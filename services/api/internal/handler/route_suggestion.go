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

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/httputil"
	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/sport"
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
	"hiking":          "hiking",
	"hiking-mountain": "hiking",
	"trekking":        "trekking",
	"mtb":             "mtb",
	"gravel":          "gravel",
	"road":            "road",
	"fastbike":        "road",
}

func routingProfileMatchesSuggestionRequest(profile database.RoutingProfile, req RouteSuggestionRequest) bool {
	return routingProfileMatchesSuggestionRequestWithFilters(profile, req, profile.ModifierFilters)
}

func routingProfileMatchesSuggestionRequestWithFilters(profile database.RoutingProfile, req RouteSuggestionRequest, filters sport.ModifierFilters) bool {
	preds := profile.Predicates
	if len(preds) == 0 {
		var err error
		preds, err = sport.PredicatesFromSportTypes(profile.SportTypes)
		if err != nil {
			return false
		}
	}

	if !predicatesMatchSuggestionRequest(preds, req) {
		return false
	}

	state := sport.ActivityModifierState{}
	if req.Modifiers != nil {
		state.IsElectric = boolPtrValue(req.Modifiers.IsElectric)
		state.IsIndoor = boolPtrValue(req.Modifiers.IsIndoor)
		state.IsVirtual = boolPtrValue(req.Modifiers.IsVirtual)
		state.IsCommute = boolPtrValue(req.Modifiers.IsCommute)
	}
	return sport.ModifiersMatch(filters, state)
}

func builtinProfileMatchesSuggestionRequest(profileName string, req RouteSuggestionRequest) bool {
	preds := routing.DefaultPredicates(profileName)
	if len(preds) == 0 {
		return true
	}
	return predicatesMatchSuggestionRequest(preds, req)
}

func predicatesMatchSuggestionRequest(preds []sport.Predicate, req RouteSuggestionRequest) bool {
	if req.SportType == "" {
		return true
	}

	requestSubType := ""
	if req.SubType != nil {
		requestSubType = *req.SubType
	}
	tuple, _, err := sport.NormalizeSportTuple(req.SportType, requestSubType)
	if err != nil {
		return false
	}
	return sport.PredicatesMatch(preds, tuple.SportType, &tuple.SubType)
}

func boolPtrValue(v *bool) bool {
	return v != nil && *v
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
	// SubType is the optional canonical FIT sport subtype.
	SubType *string `json:"sub_type,omitempty"`
	// Modifiers optionally constrain profile matching by activity flags.
	Modifiers *sport.ModifierFilters `json:"modifiers,omitempty"`
	// Profile is the routing profile (e.g. "hiking", "mtb", "gravel") or a custom profile name.
	Profile string `json:"profile"`
	// ProfileSportOverride bypasses profile-vs-sport tuple matching after an explicit frontend warning.
	ProfileSportOverride bool `json:"profile_sport_override"`
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

	// ElevationProfile is the authoritative top-level profile for the suggestion
	// (Spec §6.4 "key always present"). The same data is also exposed per-feature
	// in geojson.properties.elevation_profile for per-segment consumers. Both
	// paths intentionally carry the same data — frontend should prefer the
	// top-level field for the combined chart, per-feature for segment editing.
	// nil when no DEM data is available for the route.
	ElevationProfile *[]routing.ElevationPoint `json:"elevation_profile"`
}

// ResponsePassedSummit is a handler-internal DTO for database.PassedSummit.
// It avoids serialising DB types directly (DTO-layer isolation).
type ResponsePassedSummit struct {
	OsmID    int64    `json:"osm_id"`
	Name     string   `json:"name"`
	EleM     *float64 `json:"ele_m,omitempty"`
	Category string   `json:"category,omitempty"`
	DistM    float64  `json:"dist_m"`
	Lat      float64  `json:"lat"`
	Lng      float64  `json:"lng"`
}

// toResponseSummit maps a database.PassedSummit to a ResponsePassedSummit DTO.
func toResponseSummit(s database.PassedSummit) ResponsePassedSummit {
	return ResponsePassedSummit{
		OsmID:    s.OsmID,
		Name:     s.Name,
		EleM:     s.EleM, // *float64 pointer copy — no precision loss
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

type routeSuggestionError struct {
	Status int
	Title  string
	Detail string
	Code   string
}

func (e routeSuggestionError) Error() string {
	return e.Detail
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

	resp, err := h.SuggestRoutesForUser(r.Context(), userID, req)
	if err != nil {
		var suggestErr routeSuggestionError
		if errors.As(err, &suggestErr) {
			httputil.WriteProblem(w, suggestErr.Status, suggestErr.Title, suggestErr.Detail, suggestErr.Code, r.URL.Path)
			return
		}
		httputil.WriteError(w, http.StatusInternalServerError, "Routing Error", err.Error(), r.URL.Path)
		return
	}
	httputil.WriteJSON(w, http.StatusOK, resp)
}

// SuggestRoutesForUser runs the route suggestion engine without an HTTP request.
// It is used by the HTTP handler and by internal callers such as agent tools.
func (h *Handler) SuggestRoutesForUser(ctx context.Context, userID string, req RouteSuggestionRequest) (RouteSuggestionResponse, error) {
	if reason := h.featureUnavailableReason(capabilities.KeyRouteSuggestion); reason != "" {
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusServiceUnavailable, Title: "Feature Unavailable", Detail: "feature route_suggestion is unavailable: " + reason, Code: "feature_unavailable"}
	}
	// --- Input validation ---
	if req.Start[0] < -180 || req.Start[0] > 180 || req.Start[1] < -90 || req.Start[1] > 90 {
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "start coordinates out of range"}
	}
	if req.End != nil {
		if req.End[0] < -180 || req.End[0] > 180 || req.End[1] < -90 || req.End[1] > 90 {
			return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "end coordinates out of range"}
		}
	}
	if req.TargetDistanceM <= 0 {
		req.TargetDistanceM = 10000 // default 10 km
	}
	if req.SurfaceTendency != "" {
		validTendencies := map[string]bool{"trails": true, "forest_roads": true, "mixed": true, "paved": true}
		if !validTendencies[req.SurfaceTendency] {
			return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "surface_tendency must be one of: trails, forest_roads, mixed, paved"}
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
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusServiceUnavailable, Title: "Feature Unavailable", Detail: "feature route_suggestion is unavailable: " + capabilities.ReasonMissingArtifacts, Code: "feature_unavailable"}
	}
	router := h.GraphManager.UnifiedRouter()
	if router == nil {
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusServiceUnavailable, Title: "Feature Unavailable", Detail: "feature route_suggestion is unavailable: " + capabilities.ReasonMissingArtifacts, Code: "feature_unavailable"}
	}

	// --- Map profile ---
	mappedProfile := profileMap[req.Profile]
	profileCfg := routing.ProfileConfig{Profile: mappedProfile}
	if mappedProfile != "" && !req.ProfileSportOverride && !builtinProfileMatchesSuggestionRequest(mappedProfile, req) {
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "profile does not match requested sport tuple"}
	}

	// If profile not in system defaults, try loading custom profile from DB.
	if mappedProfile == "" {
		var customProfile *database.RoutingProfile
		var dbErr error
		if h.DB != nil {
			customProfile, dbErr = h.DB.GetRoutingProfileByNameOrGlobal(ctx, userID, req.Profile)
		}
		if dbErr == nil && customProfile != nil {
			if !req.ProfileSportOverride && !routingProfileMatchesSuggestionRequest(*customProfile, req) {
				return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "profile does not match requested sport tuple"}
			}
			_ = json.Unmarshal(customProfile.Config, &profileCfg)
			profileCfg.Profile = customProfile.BaseProfile
			mappedProfile = customProfile.BaseProfile
		} else {
			return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusBadRequest, Title: "Bad Request", Detail: "unknown routing profile"}
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
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusInternalServerError, Title: "Routing Error", Detail: "failed to build cost config"}
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
		rows, dbErr := h.DB.Pool.Query(ctx,
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
		ctx, startLat, startLon, cfg, halfBudgetCost, visitedWayIDs)
	if err != nil {
		slog.Error("suggest routes: FindReachableWithCoords failed", "error", err, "user_id", userID)
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusInternalServerError, Title: "Routing Error", Detail: "reachability query failed"}
	}

	coverageAvailable := h.CoverageReader != nil

	if len(reachable) == 0 {
		return RouteSuggestionResponse{
			Suggestions: []RouteSuggestion{}, CoverageAvailable: coverageAvailable, PartialSearch: truncated,
		}, nil
	}

	// Step 2: Load routing state (opts + ctx are request-scoped, not part of pool helper)
	opts := DefaultRoundTripOptions()
	maxVariants := *req.MaxVariants

	// Step 3: Build candidate pool (filter + sort + truncate + diversify)
	filtered, pool := buildCandidatePool(reachable, startLat, startLon, req.TargetDistanceM, maxVariants)
	routingMetricDiversityPoolSize.Observe(float64(len(pool)))
	if len(pool) == 0 {
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusNotFound, Title: "Not Found", Detail: "no candidate pool built"}
	}

	// Step 4: Build all candidates in parallel via errgroup.
	ctx60, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	candidates := make([]*RouteCandidate, len(pool))
	var eg errgroup.Group
	eg.SetLimit(h.GraphManager.MaxConcurrentRoutes())
	for i, sel := range pool {
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
		return RouteSuggestionResponse{}, routeSuggestionError{Status: http.StatusInternalServerError, Title: "Routing Error", Detail: "no routes built"}
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
	radiusM := 80.0 // Default fallback (matches Migration 000318 DEFAULT)
	if h.DB != nil && userID != "" {
		if settings, err := h.DB.GetEnrichmentSettings(ctx, userID); err == nil && settings != nil {
			radiusM = settings.SummitDetectionRadiusM
		} else if err != nil {
			slog.Warn("get enrichment settings failed; falling back to default radius",
				"user_id", userID, "error", err)
		}
	}

	// Step 10a: Pre-compute summit queries in parallel (regression case).
	// All variants' WKB encoding + PostGIS queries run concurrently before the
	// build loop so that 3×~40ms PostGIS RTTs overlap instead of stacking.
	// Per-variant isolation: errors are logged and produce nil (no bubble-up).
	summitsPerVariant := make([][]database.PassedSummit, len(labeled))
	if h.DB != nil && h.DB.Pool != nil && radiusM > 0 {
		var sgSummits errgroup.Group
		sgSummits.SetLimit(4) // cap at 4 concurrent PostGIS queries
		for i, lr := range labeled {
			merged := lr.Candidate.Merged
			if len(merged.Coordinates) < 2 {
				continue // approach-only or empty: skip, leave slot nil
			}
			sgSummits.Go(func() error {
				wkbBytes, wkbErr := coordsToLineStringWKB(merged.Coordinates)
				if wkbErr != nil {
					if !errors.Is(wkbErr, errTooFewCoords) {
						slog.Warn("summit-along-geom: WKB failed", "err", wkbErr, "variant", lr.VariantID)
					}
					return nil // per-variant isolation
				}
				summits, err := database.FindSummitsAlongGeom(ctx, h.DB.Pool, wkbBytes, radiusM)
				if err != nil {
					slog.Warn("summits along route failed", "variant", lr.VariantID, "error", err)
					return nil // per-variant isolation (Spec §4.1.5)
				}
				summitsPerVariant[i] = summits
				return nil
			})
		}
		_ = sgSummits.Wait() // errors impossible — all goroutines return nil
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

		// Range computation per variant — single fused pass over edge tags (regression case).
		// Use LastEdgeCoordEnd-1 when set to exclude the dst-VE air-line tail from
		// range highlights (F2: dst-VE Range-Bleed fix).
		lastCoordIdx := lastRealCoordIdx(merged)
		allRanges := computeAllRanges(merged.EdgeCoordStart, merged.EdgeTags1, merged.EdgeTags2, lastCoordIdx)
		forestRanges := allRanges.Forest
		protectedRanges := allRanges.Protected
		sacRanges := allRanges.Sac
		mtbRanges := allRanges.Mtb
		vfRanges := allRanges.Vf
		vfUnknownRanges := allRanges.VfUnknown

		// Consume pre-computed summit results (regression case: parallel pre-fetch above).
		summitsPassed := summitsPerVariant[i]
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
			ElevationProfile:       merged.ElevationProfile,
		})
	}

	// assignLabels already sorts by DistanceM; no redundant sort needed here.

	return RouteSuggestionResponse{
		Suggestions:       suggestions,
		CoverageAvailable: coverageAvailable,
		PartialSearch:     truncated,
	}, nil
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
	if len(pool) < poolSize && minDist > 0 {
		relaxed := routing.BuildCandidatePool(filtered, startLat, startLon, targetDistanceM, poolSize, 0)
		for _, sel := range relaxed {
			if candidateSelectionInPool(pool, sel) {
				continue
			}
			pool = append(pool, sel)
			if len(pool) >= poolSize {
				break
			}
		}
	}
	return filtered, pool
}

func candidateSelectionInPool(pool []routing.CandidateSelection, want routing.CandidateSelection) bool {
	for _, sel := range pool {
		if sel.NodeIdx == want.NodeIdx && sel.StateIdx == want.StateIdx {
			return true
		}
	}
	return false
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
