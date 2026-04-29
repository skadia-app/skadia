package garmin

import "testing"

func TestGarminCourseTypeToSportTuple(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSport string
		wantSub   string
	}{
		{name: "road cycling", input: "road_cycling", wantSport: "cycling", wantSub: "road"},
		{name: "mountain biking", input: "mountain_biking", wantSport: "cycling", wantSub: "mountain"},
		{name: "trail running", input: "trail_running", wantSport: "running", wantSub: "trail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSport, gotSub := garminCourseTypeToSport(tt.input)
			if gotSport != tt.wantSport || ptrStringValue(gotSub) != tt.wantSub {
				t.Fatalf("garminCourseTypeToSport(%q) = %s/%s, want %s/%s",
					tt.input, gotSport, ptrStringValue(gotSub), tt.wantSport, tt.wantSub)
			}
		})
	}
}
