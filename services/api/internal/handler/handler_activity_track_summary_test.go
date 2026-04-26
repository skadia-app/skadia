package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/edgetags"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestBuildTrackSummary_JSONShapeNullFieldsAndSettingsSnapshot(t *testing.T) {
	forestPct := 61.5
	settings := &model.EnrichmentSettings{
		UserID:                 "user-1",
		EnableSurfaceDetection: true,
		EnableDifficultyScales: false,
	}

	summary, err := buildTrackSummary(settings, []trackSummaryEnrichmentRow{
		{Category: "forest_pct", Percentage: forestPct, Ranges: []edgetags.CoordRange{{From: 2, To: 5}}},
		{Category: "surface", Type: "asphalt", Percentage: 75, LengthM: ptrFloat64(1500)},
		{Category: "highway", Type: "track", Percentage: 25, LengthM: ptrFloat64(500)},
	}, nil)
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
		`"enable_difficulty_scales":false`,
	} {
		if !strings.Contains(jsonBody, want) {
			t.Fatalf("summary JSON missing %s in %s", want, jsonBody)
		}
	}
	for _, unwanted := range []string{
		`"user_id":`,
		`"enable_surface_detection":`,
	} {
		if strings.Contains(jsonBody, unwanted) {
			t.Fatalf("summary JSON unexpectedly contains %s in %s", unwanted, jsonBody)
		}
	}
}

func TestBuildTrackSummary_GroupsScalesAndGeoTags(t *testing.T) {
	trackPos := 0.42
	summary, err := buildTrackSummary(&model.EnrichmentSettings{}, []trackSummaryEnrichmentRow{
		{Category: "sac_scale", Type: "T3", Percentage: 12, Ranges: []edgetags.CoordRange{{From: 1, To: 3}}},
		{Category: "mtb_scale", Type: "S1", Percentage: 8, Ranges: []edgetags.CoordRange{{From: 4, To: 7}}},
		{Category: "vf_scale", Type: "K2", Percentage: 4, Ranges: []edgetags.CoordRange{{From: 8, To: 9}}},
		{Category: "vf_unknown", Percentage: 2, Ranges: []edgetags.CoordRange{{From: 10, To: 11}}},
	}, []trackSummaryGeoTagRow{
		{Category: "summit", Name: "Peak", OSMID: ptrInt64(100), Lat: ptrFloat64(47.1), Lng: ptrFloat64(11.1), EleM: ptrFloat64(2000), DistM: ptrFloat64(18), TrackPos: &trackPos},
		{Category: "flowing_water", Name: "Creek", OSMID: ptrInt64(200)},
		{Category: "settlement", Name: "Village", OSMID: ptrInt64(300)},
	})
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

func ptrFloat64(v float64) *float64 { return &v }
func ptrInt64(v int64) *int64       { return &v }
