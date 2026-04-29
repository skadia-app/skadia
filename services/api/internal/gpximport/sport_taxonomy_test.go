package gpximport

import "testing"

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
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotSport, gotSubType := mapGPXType(tt.input)
			if gotSport != tt.wantSport || gotSubType != tt.wantSubType {
				t.Fatalf("mapGPXType(%q): got (%q, %q), want (%q, %q)",
					tt.input, gotSport, gotSubType, tt.wantSport, tt.wantSubType)
			}
		})
	}
}
