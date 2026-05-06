package handler

import (
	"testing"

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

func TestParseRecomputeScopeValuesRejectsUnknownScopes(t *testing.T) {
	if _, err := parseRecomputeScopeValues([]string{"bogus"}); err == nil {
		t.Fatal("parseRecomputeScopeValues accepted unknown scope")
	}
}

func TestGoNativeModulesAreCoveredByRecomputeScopes(t *testing.T) {
	for module := range enrichment.GoNativeModules {
		if !isValidRecomputeScope(module) {
			t.Fatalf("Go-native module %q is missing from recompute scope validation", module)
		}
	}
}
