package strava

import "testing"

func TestStravaRouteTypeToSportTuple(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSport string
		wantSub   string
	}{
		{name: "run", input: "run", wantSport: "running", wantSub: ""},
		{name: "running", input: "running", wantSport: "running", wantSub: ""},
		{name: "trail run", input: "trail run", wantSport: "running", wantSub: "trail"},
		{name: "trail running", input: "trail running", wantSport: "running", wantSub: "trail"},
		{name: "trailrun", input: "trailrun", wantSport: "running", wantSub: "trail"},
		{name: "ride", input: "ride", wantSport: "cycling", wantSub: ""},
		{name: "cycling", input: "cycling", wantSport: "cycling", wantSub: ""},
		{name: "bike", input: "bike", wantSport: "cycling", wantSub: ""},
		{name: "hike", input: "hike", wantSport: "hiking", wantSub: ""},
		{name: "hiking", input: "hiking", wantSport: "hiking", wantSub: ""},
		{name: "walk", input: "walk", wantSport: "walking", wantSub: ""},
		{name: "walking", input: "walking", wantSport: "walking", wantSub: ""},
		{name: "alpine ski", input: "alpine ski", wantSport: "alpine_skiing", wantSub: ""},
		{name: "skiing", input: "skiing", wantSport: "alpine_skiing", wantSub: ""},
		{name: "mixed case and whitespace", input: " Trail Run ", wantSport: "running", wantSub: "trail"},
		{name: "unknown", input: "pickleball", wantSport: "unknown", wantSub: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSport, gotSub := stravaRouteTypeToSport(tt.input)
			if gotSport != tt.wantSport || ptrStringValue(gotSub) != tt.wantSub {
				t.Fatalf("stravaRouteTypeToSport(%q) = %s/%s, want %s/%s",
					tt.input, gotSport, ptrStringValue(gotSub), tt.wantSport, tt.wantSub)
			}
		})
	}
}

func ptrStringValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
