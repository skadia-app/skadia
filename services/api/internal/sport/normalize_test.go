package sport

import "testing"

func TestNormalizeSportTuple(t *testing.T) {
	trueValue := true

	tests := []struct {
		name         string
		sportType    string
		subType      string
		wantTuple    Tuple
		wantElectric *bool
		wantIndoor   *bool
		wantErr      bool
	}{
		{
			name:      "mountain biking maps to cycling mountain",
			sportType: "mountain_biking",
			wantTuple: Tuple{SportType: "cycling", SubType: "mountain"},
		},
		{
			name:         "electric mountain biking maps to modifier plus cycling mountain",
			sportType:    "e_mountain_biking",
			wantTuple:    Tuple{SportType: "cycling", SubType: "mountain"},
			wantElectric: &trueValue,
		},
		{
			name:       "treadmill running maps to treadmill modifier",
			sportType:  "treadmill_running",
			wantTuple:  Tuple{SportType: "running", SubType: "treadmill"},
			wantIndoor: &trueValue,
		},
		{
			name:       "explicit treadmill running subtype stays precise",
			sportType:  "running",
			subType:    "treadmill",
			wantTuple:  Tuple{SportType: "running", SubType: "treadmill"},
			wantIndoor: &trueValue,
		},
		{
			name:       "explicit indoor cycling subtype sets indoor modifier",
			sportType:  "cycling",
			subType:    "indoor_cycling",
			wantTuple:  Tuple{SportType: "cycling", SubType: "indoor_cycling"},
			wantIndoor: &trueValue,
		},
		{
			name:      "generic subtype clears to no subtype",
			sportType: "cycling",
			subType:   "generic",
			wantTuple: Tuple{SportType: "cycling"},
		},
		{
			name:         "electric generic bike maps to broad electric cycling",
			sportType:    "e_bike",
			wantTuple:    Tuple{SportType: "cycling"},
			wantElectric: &trueValue,
		},
		{
			name:      "road cycling maps to cycling road",
			sportType: "road_cycling",
			wantTuple: Tuple{SportType: "cycling", SubType: "road"},
		},
		{
			name:      "legacy bike maps to cycling",
			sportType: "bike",
			wantTuple: Tuple{SportType: "cycling"},
		},
		{
			name:      "paddling remains canonical",
			sportType: "paddling",
			wantTuple: Tuple{SportType: "paddling"},
		},
		{
			name:      "unknown sport returns error",
			sportType: "totally_new_sport",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTuple, gotPatch, err := NormalizeSportTuple(tt.sportType, tt.subType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeSportTuple(%q, %q) returned nil error", tt.sportType, tt.subType)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeSportTuple(%q, %q) returned error: %v", tt.sportType, tt.subType, err)
			}
			if gotTuple != tt.wantTuple {
				t.Fatalf("NormalizeSportTuple(%q, %q) tuple = %#v, want %#v", tt.sportType, tt.subType, gotTuple, tt.wantTuple)
			}
			assertBoolPatch(t, "IsElectric", gotPatch.IsElectric, tt.wantElectric)
			assertBoolPatch(t, "IsIndoor", gotPatch.IsIndoor, tt.wantIndoor)
		})
	}
}

func TestNormalizeSportTupleForImportDropsDeviceModeSubTypes(t *testing.T) {
	gotTuple, gotPatch, err := NormalizeSportTupleForImport("cycling", "commuting")
	if err != nil {
		t.Fatalf("NormalizeSportTupleForImport: %v", err)
	}
	if gotTuple != (Tuple{SportType: "cycling"}) {
		t.Fatalf("tuple = %#v, want broad cycling", gotTuple)
	}
	if gotPatch.IsElectric != nil || gotPatch.IsIndoor != nil {
		t.Fatalf("patch = %#v, want empty", gotPatch)
	}
}

func assertBoolPatch(t *testing.T, field string, got, want *bool) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s patch = %v, want nil", field, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s patch = nil, want %v", field, *want)
	}
	if *got != *want {
		t.Fatalf("%s patch = %v, want %v", field, *got, *want)
	}
}
