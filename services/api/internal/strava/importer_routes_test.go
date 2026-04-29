package strava

import "testing"

func TestStravaRouteTypeToSportTuple(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantSport string
		wantSub   string
	}{
		{name: "trail run", input: "trail run", wantSport: "running", wantSub: "trail"},
		{name: "ride", input: "ride", wantSport: "cycling", wantSub: ""},
		{name: "hiking", input: "hike", wantSport: "hiking", wantSub: ""},
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
