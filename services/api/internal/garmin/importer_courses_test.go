package garmin

import "testing"

func TestGarminCourseTypeToSportTuple(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSport string
		wantSub   string
	}{
		{name: "cycling", input: "cycling", wantSport: "cycling"},
		{name: "road cycling", input: "road_cycling", wantSport: "cycling", wantSub: "road"},
		{name: "mountain biking", input: "mountain_biking", wantSport: "cycling", wantSub: "mountain"},
		{name: "gravel cycling", input: "gravel_cycling", wantSport: "cycling", wantSub: "gravel_cycling"},
		{name: "running", input: "running", wantSport: "running"},
		{name: "trail running", input: "trail_running", wantSport: "running", wantSub: "trail"},
		{name: "hiking", input: "hiking", wantSport: "hiking"},
		{name: "walking", input: "walking", wantSport: "walking"},
		{name: "swimming", input: "swimming", wantSport: "swimming"},
		{name: "skiing", input: "skiing", wantSport: "alpine_skiing"},
		{name: "cross country skiing", input: "cross_country_skiing", wantSport: "cross_country_skiing"},
		{name: "unknown", input: "definitely_unknown", wantSport: "generic"},
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
