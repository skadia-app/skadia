package handler

import "testing"

func TestSportTypeToProfileUsesCanonicalSubType(t *testing.T) {
	mountain := "mountain"
	road := "road"

	tests := []struct {
		name      string
		sportType string
		subType   *string
		want      string
	}{
		{name: "canonical MTB", sportType: "cycling", subType: &mountain, want: "cycling-mountain"},
		{name: "canonical road", sportType: "cycling", subType: &road, want: "cycling-road"},
		{name: "trail running", sportType: "running", want: "running"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sportTypeToProfile(tt.sportType, tt.subType); got != tt.want {
				t.Fatalf("sportTypeToProfile(%q, %v) = %q, want %q", tt.sportType, tt.subType, got, tt.want)
			}
		})
	}
}
