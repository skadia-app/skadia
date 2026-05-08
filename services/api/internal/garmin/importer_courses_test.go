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
		{name: "course type is normalized before alias lookup", input: " Mountain_Biking ", wantSport: "cycling", wantSub: "mountain"},
		{name: "mountain biking", input: "mountain_biking", wantSport: "cycling", wantSub: "mountain"},
		{name: "gravel cycling", input: "gravel_cycling", wantSport: "cycling", wantSub: "gravel_cycling"},
		{name: "running", input: "running", wantSport: "running"},
		{name: "trail running", input: "trail_running", wantSport: "running", wantSub: "trail"},
		{name: "hiking", input: "hiking", wantSport: "hiking"},
		{name: "walking", input: "walking", wantSport: "walking"},
		{name: "swimming", input: "swimming", wantSport: "swimming"},
		{name: "skiing", input: "skiing", wantSport: "alpine_skiing"},
		{name: "cross country skiing", input: "cross_country_skiing", wantSport: "cross_country_skiing"},
		{name: "canonical direct type key", input: "archery", wantSport: "archery"},
		{name: "new FIT direct type key", input: "canoeing", wantSport: "canoeing"},
		{name: "wheelchair raw type key", input: "wheelchair_push_run", wantSport: "wheelchair_push", wantSub: "run_speed"},
		{name: "broad water bucket stays unknown", input: "water_sport", wantSport: "unknown"},
		{name: "unknown", input: "definitely_unknown", wantSport: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSport, gotSub, gotPatch := garminCourseTypeToSport(tt.input)
			if gotSport != tt.wantSport || ptrStringValue(gotSub) != tt.wantSub {
				t.Fatalf("garminCourseTypeToSport(%q) = %s/%s, want %s/%s",
					tt.input, gotSport, ptrStringValue(gotSub), tt.wantSport, tt.wantSub)
			}
			if gotPatch.IsElectric != nil {
				t.Fatalf("garminCourseTypeToSport(%q) electric patch = %#v, want nil", tt.input, gotPatch.IsElectric)
			}
		})
	}
}

func TestGarminCourseTypeToSportPreservesElectricPatch(t *testing.T) {
	gotSport, gotSub, gotPatch := garminCourseTypeToSport("e_biking")
	if gotSport != "cycling" || ptrStringValue(gotSub) != "" {
		t.Fatalf("garminCourseTypeToSport(e_biking) = %s/%s, want cycling/", gotSport, ptrStringValue(gotSub))
	}
	if gotPatch.IsElectric == nil || !*gotPatch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", gotPatch.IsElectric)
	}
}
