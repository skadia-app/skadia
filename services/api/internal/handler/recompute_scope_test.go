package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/capabilities"
	"github.com/skadia-app/skadia/services/api/internal/enrichment"
)

func TestBuildRecomputeScopeFlagsStreamModulesAreStandalone(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"ascents", "stops", "intervals"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}

	flags := buildRecomputeScopeFlags(scope)
	if !flags.doAscents || !flags.doStops || !flags.doIntervals {
		t.Fatalf("stream flags not enabled: %+v", flags)
	}
	if !flags.doStreamEnrichment {
		t.Fatalf("doStreamEnrichment = false, want true")
	}
	if flags.doGeo {
		t.Fatalf("stream-only scopes must not enable geo: %+v", flags)
	}
	if flags.doStage1 {
		t.Fatalf("stream-only scopes must not trigger Stage1: %+v", flags)
	}
}

func TestBuildRecomputeScopeFlagsSettlementsAndBordersAreSeparate(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"settlements"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags := buildRecomputeScopeFlags(scope)
	if !flags.doSettlements {
		t.Fatalf("doSettlements = false, want true")
	}
	if flags.doBorders {
		t.Fatalf("doBorders = true, want false for settlements-only")
	}

	scope, err = parseRecomputeScopeValues([]string{"borders"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags = buildRecomputeScopeFlags(scope)
	if flags.doSettlements {
		t.Fatalf("doSettlements = true, want false for borders-only")
	}
	if !flags.doBorders {
		t.Fatalf("doBorders = false, want true")
	}
}

func TestBuildRecomputeScopeFlagsGeoKeepsLegacyCoverage(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"geo"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags := buildRecomputeScopeFlags(scope)

	if !flags.doGeo || !flags.doSummits || !flags.doSettlements || !flags.doForests ||
		!flags.doWater || !flags.doBorders || !flags.doSurfaces || !flags.doLifts {
		t.Fatalf("legacy geo did not enable every geo submodule: %+v", flags)
	}
	if !flags.doStops || !flags.doStreamEnrichment {
		t.Fatalf("legacy geo must still recompute stops: %+v", flags)
	}
}

// scope=["lifts"] must enable doStreamEnrichment so the scoped branch in
// recompute.go runs RunStreamEnrichment with EnableLiftDetection=true after
// the cleanup batch wipes activity_lift_candidates / lift segment_efforts.
// Without this, the cleanup runs but detection never re-runs, leaving the
// activity in a silent data-loss state.
func TestBuildRecomputeScopeFlagsLiftsOnlyTriggersStreamEnrichment(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"lifts"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags := buildRecomputeScopeFlags(scope)
	if !flags.doLifts {
		t.Fatalf("doLifts = false, want true")
	}
	if !flags.doStreamEnrichment {
		t.Fatalf("doStreamEnrichment = false, want true (lifts run via RunStreamEnrichment)")
	}
	if flags.doStops || flags.doAscents || flags.doIntervals {
		t.Fatalf("lifts-only must not enable other stream modules: %+v", flags)
	}
}

// scope=["lifts","stops"] must run stream enrichment exactly once (the gate
// is doStreamEnrichment, not the count). Both module flags must be set so
// RunStreamEnrichment processes both in the same pass.
func TestBuildRecomputeScopeFlagsLiftsAndStopsCombined(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"lifts", "stops"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags := buildRecomputeScopeFlags(scope)
	if !flags.doLifts || !flags.doStops {
		t.Fatalf("doLifts/doStops not both enabled: %+v", flags)
	}
	if !flags.doStreamEnrichment {
		t.Fatalf("doStreamEnrichment = false, want true")
	}
}

func TestBuildRecomputeScopeFlagsAutoTitleOnlyDoesNotRequestReparseWork(t *testing.T) {
	scope, err := parseRecomputeScopeValues([]string{"auto_title"})
	if err != nil {
		t.Fatalf("parseRecomputeScopeValues: %v", err)
	}
	flags := buildRecomputeScopeFlags(scope)
	if !flags.doAutoTitle {
		t.Fatalf("doAutoTitle = false, want true")
	}
	if flags.doStage1 || flags.doStreams || flags.doMetrics || flags.doGeo || flags.doTags {
		t.Fatalf("auto_title-only must not request file-backed recompute work: %+v", flags)
	}
	if !flags.autoTitleOnly() {
		t.Fatalf("autoTitleOnly = false, want true")
	}
}

func TestBuildRecomputeScopeFlagsAllScopeIncludesAutoTitle(t *testing.T) {
	flags := buildRecomputeScopeFlags(nil)
	if !flags.allScope || !flags.doAutoTitle {
		t.Fatalf("all-scope must include auto-title recompute: %+v", flags)
	}
	if flags.autoTitleOnly() {
		t.Fatalf("all-scope must not be treated as auto-title-only: %+v", flags)
	}
}

func TestParseRecomputeScopeValuesRejectsUnknownScopes(t *testing.T) {
	if _, err := parseRecomputeScopeValues([]string{"bogus"}); err == nil {
		t.Fatal("parseRecomputeScopeValues accepted unknown scope")
	}
}

func TestRecomputeScopeUnavailableReason_CoreBlocksFullOnlyScopes(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}

	for _, tc := range []struct {
		name  string
		scope []string
		want  string
	}{
		{name: "all", scope: nil, want: capabilities.KeyGeoPOIEnrichment},
		{name: "geo", scope: []string{"geo"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "summits", scope: []string{"summits"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "forests", scope: []string{"forests"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "water", scope: []string{"water"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "settlements", scope: []string{"settlements"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "borders", scope: []string{"borders"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "stops", scope: []string{"stops"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "surfaces", scope: []string{"surfaces"}, want: capabilities.KeySurfaceEnrichment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, err := parseRecomputeScopeValues(tc.scope)
			if err != nil {
				t.Fatalf("parseRecomputeScopeValues: %v", err)
			}
			key, reason := h.recomputeScopeUnavailableReason(t.Context(), scope)
			if key != tc.want || reason != capabilities.ReasonCoreProfile {
				t.Fatalf("unavailable = (%q,%q), want (%q,%q)", key, reason, tc.want, capabilities.ReasonCoreProfile)
			}
		})
	}
}

func TestRecomputeScopeUnavailableReason_CoreAllowsSafeScopes(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "core"}}

	for _, scopeValues := range [][]string{
		{"auto_title"},
		{"ascents"},
		{"intervals"},
		{"lifts"},
		{"ascents", "intervals", "lifts"},
	} {
		scope, err := parseRecomputeScopeValues(scopeValues)
		if err != nil {
			t.Fatalf("parseRecomputeScopeValues(%v): %v", scopeValues, err)
		}
		if key, reason := h.recomputeScopeUnavailableReason(t.Context(), scope); key != "" || reason != "" {
			t.Fatalf("scope %v unavailable = (%q,%q), want allowed", scopeValues, key, reason)
		}
	}
}

func TestRecomputeScopeUnavailableReason_FullBlocksUnavailableGeodataScopes(t *testing.T) {
	h := &Handler{Config: &HandlerConfig{DeploymentProfile: "full"}}

	for _, tc := range []struct {
		name  string
		scope []string
		want  string
	}{
		{name: "all", scope: nil, want: capabilities.KeyGeoPOIEnrichment},
		{name: "geo", scope: []string{"geo"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "stops", scope: []string{"stops"}, want: capabilities.KeyGeoPOIEnrichment},
		{name: "surfaces", scope: []string{"surfaces"}, want: capabilities.KeySurfaceEnrichment},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, err := parseRecomputeScopeValues(tc.scope)
			if err != nil {
				t.Fatalf("parseRecomputeScopeValues: %v", err)
			}
			key, reason := h.recomputeScopeUnavailableReason(t.Context(), scope)
			if key != tc.want || reason != capabilities.ReasonMissingArtifacts {
				t.Fatalf("unavailable = (%q,%q), want (%q,%q)", key, reason, tc.want, capabilities.ReasonMissingArtifacts)
			}
		})
	}
}

func TestGoNativeModulesAreCoveredByRecomputeScopes(t *testing.T) {
	for module := range enrichment.GoNativeModules {
		if !isValidRecomputeScope(module) {
			t.Fatalf("Go-native module %q is missing from recompute scope validation", module)
		}
	}
}
