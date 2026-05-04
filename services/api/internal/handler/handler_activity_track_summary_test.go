package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
)

func TestBuildTrackSummary_JSONShapeNullFields(t *testing.T) {
	forestPct := 61.5

	summary, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{Category: "forest_pct", Percentage: forestPct, Ranges: []edgetags.CoordRange{{From: 2, To: 5}}},
		{Category: "surface", Type: "asphalt", Percentage: 75, LengthM: ptrFloat64(1500)},
		{Category: "highway", Type: "track", Percentage: 25, LengthM: ptrFloat64(500)},
	}, nil, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}

	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	jsonBody := string(body)

	for _, want := range []string{
		`"forest_pct":61.5`,
		`"protected_pct":null`,
		`"forest_ranges":[{"from":2,"to":5}]`,
		`"protected_ranges":null`,
		`"sac_ranges":null`,
		`"mtb_ranges":null`,
		`"vf_ranges":null`,
		`"vf_unknown_ranges":null`,
		`"surfaces":[{"type":"asphalt","percentage":75,"length_m":1500}]`,
		`"highways":[{"type":"track","percentage":25,"length_m":500}]`,
		`"summits":null`,
		`"waters":null`,
		`"settlements":null`,
	} {
		if !strings.Contains(jsonBody, want) {
			t.Fatalf("summary JSON missing %s in %s", want, jsonBody)
		}
	}
	// The deprecated `settings` envelope (and its `enable_difficulty_scales`
	// mirror) was removed — no consumer ever read it. Guard against accidental
	// reintroduction.
	for _, unwanted := range []string{
		`"user_id":`,
		`"enable_surface_detection":`,
		`"enable_difficulty_scales":`,
		`"settings":`,
	} {
		if strings.Contains(jsonBody, unwanted) {
			t.Fatalf("summary JSON unexpectedly contains %s in %s", unwanted, jsonBody)
		}
	}
}

func TestBuildTrackSummary_GroupsScalesAndGeoTags(t *testing.T) {
	trackPos := 0.42
	summary, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{Category: "sac_scale", Type: "T3", Percentage: 12, Ranges: []edgetags.CoordRange{{From: 1, To: 3}}},
		{Category: "mtb_scale", Type: "S1", Percentage: 8, Ranges: []edgetags.CoordRange{{From: 4, To: 7}}},
		{Category: "vf_scale", Type: "K2", Percentage: 4, Ranges: []edgetags.CoordRange{{From: 8, To: 9}}},
		{Category: "vf_unknown", Percentage: 2, Ranges: []edgetags.CoordRange{{From: 10, To: 11}}},
	}, []trackSummaryGeoTagRow{
		{Category: "summit", Name: "Peak", OSMID: ptrInt64(100), Lat: ptrFloat64(47.1), Lng: ptrFloat64(11.1), EleM: ptrFloat64(2000), DistM: ptrFloat64(18), TrackPos: &trackPos},
		{Category: "flowing_water", Name: "Creek", OSMID: ptrInt64(200)},
		{Category: "settlement", Name: "Village", OSMID: ptrInt64(300)},
	}, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}

	if got := summary.SacRanges["T3"]; len(got) != 1 || got[0] != (edgetags.CoordRange{From: 1, To: 3}) {
		t.Fatalf("unexpected sac ranges: %#v", summary.SacRanges)
	}
	if got := summary.MtbRanges["S1"]; len(got) != 1 || got[0] != (edgetags.CoordRange{From: 4, To: 7}) {
		t.Fatalf("unexpected mtb ranges: %#v", summary.MtbRanges)
	}
	if got := summary.VfRanges["K2"]; len(got) != 1 || got[0] != (edgetags.CoordRange{From: 8, To: 9}) {
		t.Fatalf("unexpected vf ranges: %#v", summary.VfRanges)
	}
	if len(summary.VfUnknownRanges) != 1 || summary.VfUnknownRanges[0] != (edgetags.CoordRange{From: 10, To: 11}) {
		t.Fatalf("unexpected vf unknown ranges: %#v", summary.VfUnknownRanges)
	}
	if len(summary.Summits) != 1 || summary.Summits[0].Name != "Peak" || summary.Summits[0].TrackPos == nil {
		t.Fatalf("unexpected summits: %#v", summary.Summits)
	}
	if len(summary.Waters) != 1 || summary.Waters[0].Kind != "flowing" {
		t.Fatalf("unexpected waters: %#v", summary.Waters)
	}
	if len(summary.Settlements) != 1 || summary.Settlements[0].Name != "Village" {
		t.Fatalf("unexpected settlements: %#v", summary.Settlements)
	}
}

func TestBuildTrackSummary_SurfaceAndHighwayRanges(t *testing.T) {
	summary, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{
			Category:   "surface",
			Type:       "asphalt",
			Percentage: 60,
			LengthM:    ptrFloat64(1200),
			Ranges:     []edgetags.CoordRange{{From: 0, To: 10}, {From: 20, To: 30}},
		},
		{
			Category:   "surface",
			Type:       "gravel",
			Percentage: 40,
			LengthM:    ptrFloat64(800),
			Ranges:     []edgetags.CoordRange{{From: 10, To: 20}},
		},
		{
			Category:   "highway",
			Type:       "path",
			Percentage: 75,
			LengthM:    ptrFloat64(1500),
			Ranges:     []edgetags.CoordRange{{From: 0, To: 25}},
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}

	// surface_ranges must contain asphalt and gravel entries.
	if summary.SurfaceRanges == nil {
		t.Fatal("SurfaceRanges is nil, want map")
	}
	if got := summary.SurfaceRanges["asphalt"]; len(got) != 2 || got[0] != (edgetags.CoordRange{From: 0, To: 10}) {
		t.Fatalf("unexpected SurfaceRanges[asphalt]: %#v", got)
	}
	if got := summary.SurfaceRanges["gravel"]; len(got) != 1 || got[0] != (edgetags.CoordRange{From: 10, To: 20}) {
		t.Fatalf("unexpected SurfaceRanges[gravel]: %#v", got)
	}

	// highway_ranges must contain the path entry.
	if summary.HighwayRanges == nil {
		t.Fatal("HighwayRanges is nil, want map")
	}
	if got := summary.HighwayRanges["path"]; len(got) != 1 || got[0] != (edgetags.CoordRange{From: 0, To: 25}) {
		t.Fatalf("unexpected HighwayRanges[path]: %#v", got)
	}

	// JSON shape must include the new fields.
	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	jsonBody := string(body)
	for _, want := range []string{
		`"surface_ranges":{"asphalt"`,
		`"highway_ranges":{"path"`,
	} {
		if !strings.Contains(jsonBody, want) {
			t.Fatalf("summary JSON missing %q in %s", want, jsonBody)
		}
	}
}

func TestBuildTrackSummary_SurfaceRangesNullWhenAbsent(t *testing.T) {
	// Surface rows without ranges → surface_ranges must be null in JSON.
	summary, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{Category: "surface", Type: "asphalt", Percentage: 100, LengthM: ptrFloat64(2000)},
	}, nil, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}
	if summary.SurfaceRanges != nil {
		t.Fatalf("SurfaceRanges should be nil when no ranges stored, got %#v", summary.SurfaceRanges)
	}
	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if !strings.Contains(string(body), `"surface_ranges":null`) {
		t.Fatalf("expected surface_ranges:null in JSON, got %s", string(body))
	}
}

// TestBuildTrackSummary_GraphSnapUsedFromMeta verifies that graph_snap_used is
// populated from the persisted enrichment path, not from the heuristic.
func TestBuildTrackSummary_GraphSnapUsedFromMeta(t *testing.T) {
	// No surface rows but path='graph' → graph_snap_used must be true.
	pathGraph := "graph"
	summary, err := buildTrackSummary(nil, nil, &pathGraph)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}
	if summary.GraphSnapUsed == nil {
		t.Fatal("GraphSnapUsed is nil, want *true")
	}
	if !*summary.GraphSnapUsed {
		t.Fatalf("GraphSnapUsed = %v, want true", *summary.GraphSnapUsed)
	}

	// path='sql' → graph_snap_used must be false, even if surface rows with ranges exist.
	pathSQL := "sql"
	summarySQL, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{Category: "surface", Type: "asphalt", Percentage: 100, LengthM: ptrFloat64(1000),
			Ranges: []edgetags.CoordRange{{From: 0, To: 50}}},
	}, nil, &pathSQL)
	if err != nil {
		t.Fatalf("buildTrackSummary sql: %v", err)
	}
	if summarySQL.GraphSnapUsed == nil {
		t.Fatal("GraphSnapUsed is nil for sql path, want *false")
	}
	if *summarySQL.GraphSnapUsed {
		t.Fatal("GraphSnapUsed = true for sql path, want false")
	}

	// nil enrichmentPath (not yet enriched) → graph_snap_used must be nil.
	summaryNil, err := buildTrackSummary([]trackSummaryEnrichmentRow{
		{Category: "surface", Type: "asphalt", Percentage: 100, LengthM: ptrFloat64(1000),
			Ranges: []edgetags.CoordRange{{From: 0, To: 50}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary nil path: %v", err)
	}
	if summaryNil.GraphSnapUsed != nil {
		t.Fatalf("GraphSnapUsed = %v, want nil for un-enriched activity", *summaryNil.GraphSnapUsed)
	}
}

// TestBuildTrackSummary_ResampleSpacingMPresent verifies that resample_spacing_m
// is always present in the JSON response.
func TestBuildTrackSummary_ResampleSpacingMPresent(t *testing.T) {
	summary, err := buildTrackSummary(nil, nil, nil)
	if err != nil {
		t.Fatalf("buildTrackSummary: %v", err)
	}
	if summary.ResampleSpacingM <= 0 {
		t.Fatalf("ResampleSpacingM = %v, want > 0", summary.ResampleSpacingM)
	}
	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(body), `"resample_spacing_m":`) {
		t.Fatalf("JSON missing resample_spacing_m: %s", body)
	}
}

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt64(v int64) *int64       { return &v }

// TestTrackSummaryCacheControl_RegressionGuard pins the Cache-Control header
// for the track-summary endpoint.
//
// Background: an earlier `private, max-age=86400` setting let the browser
// serve a stale empty-summits response for 24h after enrichment added
// summits to activity_geo_tags. Hard refresh did not bypass it (sub-fetch
// requests use the HTTP cache). The fix is `no-cache, must-revalidate`,
// which forces ETag-based revalidation each request — the unchanged-payload
// path returns 304 (no body), so the cost is one round-trip per request.
//
// If you change this constant, make sure that:
//
//  1. Cached responses are still invalidated when activity_geo_tags rows are
//     written (e.g. via SSE-driven React Query invalidation), AND
//  2. Either the new value still allows revalidation (no long max-age), OR
//     the cache is invalidated server-side on enrichment completion.
//
// Otherwise the original bug returns: pill says "no summits", map shows them
// (the geo-tags endpoint has no caching), users hit hard-refresh, nothing
// changes.
func TestTrackSummaryCacheControl_RegressionGuard(t *testing.T) {
	const want = "private, no-cache, must-revalidate"
	if trackSummaryCacheControl != want {
		t.Errorf("trackSummaryCacheControl = %q, want %q — see test comment for context",
			trackSummaryCacheControl, want)
	}
}
