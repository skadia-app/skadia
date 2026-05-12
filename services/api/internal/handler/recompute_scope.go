package handler

import (
	"fmt"
	"sort"
)

type recomputeScopeFlags struct {
	allScope bool

	doStreams     bool
	doSensors     bool
	doMetrics     bool
	doSportType   bool
	doTags        bool
	doDetails     bool
	doSegments    bool
	doBestEfforts bool
	doRunningForm bool
	doSpeedSmooth bool

	doSummits     bool
	doSettlements bool
	doForests     bool
	doWater       bool
	doBorders     bool
	doSurfaces    bool
	doLifts       bool
	doGeo         bool

	doStops            bool
	doAscents          bool
	doIntervals        bool
	doStreamEnrichment bool

	doStage1    bool
	doAutoTitle bool
}

var validRecomputeScopes = map[string]struct{}{
	"streams":      {},
	"sensors":      {},
	"metrics":      {},
	"details":      {},
	"segments":     {},
	"sport_type":   {},
	"tags":         {},
	"best_efforts": {},
	"running_form": {},
	"speed_smooth": {},

	"geo":         {},
	"summits":     {},
	"settlements": {},
	"forests":     {},
	"water":       {},
	"borders":     {},
	"surfaces":    {},
	"lifts":       {},

	"stops":      {},
	"ascents":    {},
	"intervals":  {},
	"auto_title": {},
}

func isValidRecomputeScope(scope string) bool {
	_, ok := validRecomputeScopes[scope]
	return ok
}

func parseRecomputeScopeValues(values []string) (map[string]bool, error) {
	if len(values) == 0 {
		return nil, nil
	}

	scope := make(map[string]bool, len(values))
	for _, value := range values {
		if !isValidRecomputeScope(value) {
			return nil, fmt.Errorf("invalid scope: %s", value)
		}
		scope[value] = true
	}
	return scope, nil
}

func validRecomputeScopeList() []string {
	out := make([]string, 0, len(validRecomputeScopes))
	for scope := range validRecomputeScopes {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

func buildRecomputeScopeFlags(scope map[string]bool) recomputeScopeFlags {
	allScope := len(scope) == 0
	geoAll := scope["geo"]

	flags := recomputeScopeFlags{
		allScope: allScope,

		doStreams:     allScope || scope["streams"],
		doSensors:     allScope || scope["sensors"],
		doMetrics:     allScope || scope["metrics"],
		doSportType:   allScope || scope["sport_type"],
		doTags:        allScope || scope["tags"],
		doDetails:     allScope || scope["details"],
		doSegments:    allScope || scope["segments"],
		doBestEfforts: allScope || scope["best_efforts"],
		doRunningForm: allScope || scope["running_form"],
		doSpeedSmooth: scope["speed_smooth"],

		doSummits:     allScope || geoAll || scope["summits"],
		doSettlements: allScope || geoAll || scope["settlements"],
		doForests:     allScope || geoAll || scope["forests"],
		doWater:       allScope || geoAll || scope["water"],
		doBorders:     allScope || geoAll || scope["borders"],
		doSurfaces:    allScope || geoAll || scope["surfaces"],
		doLifts:       allScope || geoAll || scope["lifts"],

		doStops:     allScope || geoAll || scope["stops"],
		doAscents:   allScope || scope["ascents"],
		doIntervals: allScope || scope["intervals"],
		doAutoTitle: allScope || scope["auto_title"],
	}

	flags.doGeo = flags.doSummits || flags.doSettlements || flags.doForests ||
		flags.doWater || flags.doBorders || flags.doSurfaces || flags.doLifts
	// Lifts are derived in RunStreamEnrichment (Phase 4: NATS-Lift-Dispatch removed),
	// so doLifts must gate the stream-enrichment branch in scoped recompute. Without
	// this, scope=["lifts"] runs the cleanup batch (deletes activity_lift_candidates,
	// nulls segment_effort_id on rejected audits, deletes lift segment_efforts) but
	// the scoped enrichment branch in recompute.go is gated on doStreamEnrichment,
	// leaving the activity with empty lift state.
	flags.doStreamEnrichment = flags.doStops || flags.doAscents || flags.doIntervals || flags.doLifts
	flags.doStage1 = allScope || flags.doStreams || flags.doMetrics || flags.doSportType ||
		flags.doTags || flags.doDetails || flags.doSensors || flags.doGeo

	return flags
}

func (f recomputeScopeFlags) autoTitleOnly() bool {
	return f.doAutoTitle &&
		!f.allScope &&
		!f.doStreams &&
		!f.doSensors &&
		!f.doMetrics &&
		!f.doSportType &&
		!f.doTags &&
		!f.doDetails &&
		!f.doSegments &&
		!f.doBestEfforts &&
		!f.doRunningForm &&
		!f.doSpeedSmooth &&
		!f.doGeo &&
		!f.doStops &&
		!f.doAscents &&
		!f.doIntervals
}
