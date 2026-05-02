package gpximport

import (
	"strings"
	"testing"
)

func TestMapGPXTypeReturnsCanonicalSportTuple(t *testing.T) {
	tests := []struct {
		input       string
		wantSport   string
		wantSubType string
	}{
		{"road_cycling", "cycling", "road"},
		{"roadcycling", "cycling", "road"},
		{"trail_running", "running", "trail"},
		{"mountain_biking", "cycling", "mountain"},
		{"gravel_ride", "cycling", "gravel_cycling"},
		{"17", "training", "yoga"},
		{"canoeing", "canoeing", ""},
		{"bouldering", "rock_climbing", "bouldering"},
		{"does_not_exist", "unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := mapGPXType(tt.input)
			if got.Tuple.SportType != tt.wantSport || got.Tuple.SubType != tt.wantSubType {
				t.Fatalf("mapGPXType(%q): got (%q, %q), want (%q, %q)",
					tt.input, got.Tuple.SportType, got.Tuple.SubType, tt.wantSport, tt.wantSubType)
			}
		})
	}
}

func TestMapGPXTypeReturnsElectricPatch(t *testing.T) {
	got := mapGPXType("6")
	if got.Tuple.SportType != "cycling" {
		t.Fatalf("sport = %q, want cycling", got.Tuple.SportType)
	}
	if got.Patch.IsElectric == nil || !*got.Patch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", got.Patch.IsElectric)
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
