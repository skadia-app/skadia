package gpximport

import (
	"strings"
	"testing"
)

func TestMapGPXTypeReturnsCanonicalSportTuple(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric *bool
	}{
		{"road_cycling", "cycling", "road", nil},
		{"roadcycling", "cycling", "road", nil},
		{"trail_running", "running", "trail", nil},
		{"mountain_biking", "cycling", "mountain", nil},
		{"gravel_ride", "cycling", "gravel_cycling", nil},
		{"17", "yoga", "", nil},
		{"canoeing", "canoeing", "", nil},
		{"bouldering", "rock_climbing", "bouldering", nil},
		{"geocaching", "geocaching", "", nil},
		{"does_not_exist", "unknown", "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapGPXType(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapGPXType(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
			assertBoolPatch(t, "IsElectric", got.Patch.IsElectric, tt.wantElectric)
		})
	}
}

func TestMapGPXTypeCoversAllConfiguredAliases(t *testing.T) {
	tests := []struct {
		input        string
		wantSport    string
		wantSubType  string
		wantElectric *bool
	}{
		{"1", "cycling", "", nil},
		{"2", "running", "", nil},
		{"3", "swimming", "", nil},
		{"4", "hiking", "", nil},
		{"5", "walking", "", nil},
		{"6", "cycling", "", boolPtr(true)},
		{"9", "alpine_skiing", "", nil},
		{"10", "cross_country_skiing", "", nil},
		{"11", "rock_climbing", "", nil},
		{"12", "rowing", "", nil},
		{"13", "stand_up_paddleboarding", "", nil},
		{"14", "kayaking", "", nil},
		{"15", "strength_training", "", nil},
		{"17", "yoga", "", nil},
		{"running", "running", "", nil},
		{"run", "running", "", nil},
		{"trail_running", "running", "trail", nil},
		{"trailrunning", "running", "trail", nil},
		{"trailrun", "running", "trail", nil},
		{"trail running", "running", "trail", nil},
		{"cycling", "cycling", "", nil},
		{"biking", "cycling", "", nil},
		{"bike", "cycling", "", nil},
		{"ride", "cycling", "", nil},
		{"road_cycling", "cycling", "road", nil},
		{"roadcycling", "cycling", "road", nil},
		{"mountainbikeride", "cycling", "mountain", nil},
		{"mountain_biking", "cycling", "mountain", nil},
		{"mountainbiking", "cycling", "mountain", nil},
		{"mtb", "cycling", "mountain", nil},
		{"ebikeride", "cycling", "", boolPtr(true)},
		{"e_bike_ride", "cycling", "", boolPtr(true)},
		{"ebike", "cycling", "", boolPtr(true)},
		{"e-bike", "cycling", "", boolPtr(true)},
		{"gravelride", "cycling", "gravel_cycling", nil},
		{"gravel_ride", "cycling", "gravel_cycling", nil},
		{"gravel", "cycling", "gravel_cycling", nil},
		{"hiking", "hiking", "", nil},
		{"hike", "hiking", "", nil},
		{"walking", "walking", "", nil},
		{"walk", "walking", "", nil},
		{"mountaineering", "mountaineering", "", nil},
		{"swimming", "swimming", "", nil},
		{"swim", "swimming", "", nil},
		{"open_water_swimming", "swimming", "open_water", nil},
		{"lap_swimming", "swimming", "lap_swimming", nil},
		{"skiing", "alpine_skiing", "", nil},
		{"ski", "alpine_skiing", "", nil},
		{"alpine_skiing", "alpine_skiing", "", nil},
		{"cross_country_skiing", "cross_country_skiing", "", nil},
		{"nordicski", "cross_country_skiing", "", nil},
		{"snowboarding", "snowboarding", "", nil},
		{"snowboard", "snowboarding", "", nil},
		{"snowshoeing", "snowshoeing", "", nil},
		{"ice_skating", "ice_skating", "", nil},
		{"rowing", "rowing", "", nil},
		{"row", "rowing", "", nil},
		{"kayaking", "kayaking", "", nil},
		{"canoeing", "canoeing", "", nil},
		{"paddling", "paddling", "", nil},
		{"paddle", "paddling", "", nil},
		{"stand_up_paddleboarding", "stand_up_paddleboarding", "", nil},
		{"sup", "stand_up_paddleboarding", "", nil},
		{"surfing", "surfing", "", nil},
		{"surf", "surfing", "", nil},
		{"sailing", "sailing", "", nil},
		{"sail", "sailing", "", nil},
		{"strength_training", "strength_training", "", nil},
		{"strength", "strength_training", "", nil},
		{"weighttraining", "strength_training", "", nil},
		{"training", "training", "", nil},
		{"yoga", "yoga", "", nil},
		{"pilates", "pilates", "", nil},
		{"hiit", "hiit", "", nil},
		{"rock_climbing", "rock_climbing", "", nil},
		{"climbing", "rock_climbing", "", nil},
		{"bouldering", "rock_climbing", "bouldering", nil},
		{"soccer", "soccer", "", nil},
		{"football", "soccer", "", nil},
		{"tennis", "tennis", "", nil},
		{"golf", "golf", "", nil},
		{"inline_skating", "inline_skating", "", nil},
		{"inlineskating", "inline_skating", "", nil},
		{"horseback_riding", "horseback_riding", "", nil},
		{"multisport", "multisport", "", nil},
	}
	wantIndoorByInput := map[string]*bool{
		"15":                boolPtr(true),
		"17":                boolPtr(true),
		"strength_training": boolPtr(true),
		"strength":          boolPtr(true),
		"weighttraining":    boolPtr(true),
		"training":          boolPtr(true),
		"yoga":              boolPtr(true),
		"pilates":           boolPtr(true),
		"hiit":              boolPtr(true),
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapGPXType(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapGPXType(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
			assertBoolPatch(t, "IsElectric", got.Patch.IsElectric, tt.wantElectric)
			assertBoolPatch(t, "IsIndoor", got.Patch.IsIndoor, wantIndoorByInput[tt.input])
			assertBoolPatch(t, "IsVirtual", got.Patch.IsVirtual, nil)
		})
	}
}

func TestParseGPXAppliesElectricPatch(t *testing.T) {
	data, err := ParseGPX(strings.NewReader(minimalGPX("6")), "electric.gpx")
	if err != nil {
		t.Fatalf("ParseGPX() error = %v", err)
	}
	if data.SportType != "cycling" {
		t.Fatalf("SportType = %q, want cycling", data.SportType)
	}
	if !data.IsElectric {
		t.Fatal("IsElectric = false, want true")
	}
}

func minimalGPX(trackType string) string {
	return `<gpx version="1.1" creator="test">
  <trk>
    <name>Test</name>
    <type>` + trackType + `</type>
    <trkseg>
      <trkpt lat="50.0" lon="8.0"><ele>100</ele><time>2026-01-01T00:00:00Z</time></trkpt>
      <trkpt lat="50.0001" lon="8.0001"><ele>101</ele><time>2026-01-01T00:01:00Z</time></trkpt>
    </trkseg>
  </trk>
</gpx>`
}

func boolPtr(v bool) *bool {
	return &v
}

func assertBoolPatch(t *testing.T, name string, got *bool, want *bool) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s patch = %#v, want nil", name, *got)
		}
		return
	}
	if got == nil || *got != *want {
		t.Fatalf("%s patch = %#v, want %v", name, got, *want)
	}
}
