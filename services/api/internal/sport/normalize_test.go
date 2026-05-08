package sport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeSportTuple(t *testing.T) {
	trueValue := true

	tests := []struct {
		name         string
		sportType    string
		subType      string
		wantTuple    Tuple
		wantElectric *bool
		wantIndoor   *bool
		wantVirtual  *bool
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
			name:       "explicit diving apnea pool subtype sets indoor modifier",
			sportType:  "diving",
			subType:    "apnea_pool",
			wantTuple:  Tuple{SportType: "diving", SubType: "apnea_pool"},
			wantIndoor: &trueValue,
		},
		{
			name:       "explicit indoor climbing subtype sets indoor modifier",
			sportType:  "rock_climbing",
			subType:    "indoor_climbing",
			wantTuple:  Tuple{SportType: "rock_climbing", SubType: "indoor_climbing"},
			wantIndoor: &trueValue,
		},
		{
			name:      "explicit bouldering subtype does not set indoor modifier",
			sportType: "rock_climbing",
			subType:   "bouldering",
			wantTuple: Tuple{SportType: "rock_climbing", SubType: "bouldering"},
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
			name:      "wheelchair walk maps to speed subtype",
			sportType: "wheelchair_push_walk",
			wantTuple: Tuple{SportType: "wheelchair_push", SubType: "walk_speed"},
		},
		{
			name:      "wheelchair run maps to speed subtype",
			sportType: "wheelchair_push_run",
			wantTuple: Tuple{SportType: "wheelchair_push", SubType: "run_speed"},
		},
		{
			name:       "indoor wheelchair walk preserves speed and sets indoor",
			sportType:  "wheelchair_push_walk",
			subType:    "indoor_wheelchair_walk",
			wantTuple:  Tuple{SportType: "wheelchair_push", SubType: "walk_speed"},
			wantIndoor: &trueValue,
		},
		{
			name:       "indoor wheelchair run preserves speed and sets indoor",
			sportType:  "wheelchair_push_run",
			subType:    "indoor_wheelchair_run",
			wantTuple:  Tuple{SportType: "wheelchair_push", SubType: "run_speed"},
			wantIndoor: &trueValue,
		},
		{
			name:       "strength maps to indoor strength training",
			sportType:  "strength",
			wantTuple:  Tuple{SportType: "strength_training"},
			wantIndoor: &trueValue,
		},
		{
			name:      "canoeing remains canonical",
			sportType: "canoeing",
			wantTuple: Tuple{SportType: "canoeing"},
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
			assertBoolPatch(t, "IsVirtual", gotPatch.IsVirtual, tt.wantVirtual)
		})
	}
}

func TestNormalizeSportTupleUsesGeneratedLegacyAliases(t *testing.T) {
	gotTuple, gotPatch, err := NormalizeSportTuple("e_mountain_biking", "")
	if err != nil {
		t.Fatalf("NormalizeSportTuple() error = %v", err)
	}
	if gotTuple != (Tuple{SportType: "cycling", SubType: "mountain"}) {
		t.Fatalf("tuple = %#v, want cycling/mountain", gotTuple)
	}
	if gotPatch.IsElectric == nil || !*gotPatch.IsElectric {
		t.Fatalf("IsElectric patch = %#v, want true", gotPatch.IsElectric)
	}
	if gotPatch.IsVirtual != nil {
		t.Fatalf("IsVirtual patch = %#v, want nil for legacy alias", gotPatch.IsVirtual)
	}
}

func TestNormalizeSportTupleLegacyRawSubAliasSetsIndoor(t *testing.T) {
	gotTuple, gotPatch, err := NormalizeSportTuple("wheelchair_push_walk", "indoor_wheelchair_walk")
	if err != nil {
		t.Fatalf("NormalizeSportTuple() error = %v", err)
	}
	if gotTuple != (Tuple{SportType: "wheelchair_push", SubType: "walk_speed"}) {
		t.Fatalf("tuple = %#v, want wheelchair_push/walk_speed", gotTuple)
	}
	if gotPatch.IsIndoor == nil || !*gotPatch.IsIndoor {
		t.Fatalf("IsIndoor patch = %#v, want true", gotPatch.IsIndoor)
	}
}

func TestNormalizeSportTupleStillRejectsUnknownSport(t *testing.T) {
	if _, _, err := NormalizeSportTuple("totally_new_sport", ""); err == nil {
		t.Fatal("NormalizeSportTuple accepted unknown sport")
	}
}

func TestNormalizeSelectableSportTupleRejectsDisplayOnlyTuples(t *testing.T) {
	tests := []struct {
		name      string
		sportType string
		subType   string
	}{
		{name: "unknown sport", sportType: "unknown"},
		{name: "training fallback", sportType: "training"},
		{name: "transition subtype", sportType: "transition", subType: "swim_to_bike_transition"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := NormalizeSelectableSportTuple(tt.sportType, tt.subType); err == nil {
				t.Fatalf("NormalizeSelectableSportTuple(%q, %q) accepted display-only tuple", tt.sportType, tt.subType)
			}
		})
	}
}

func TestNormalizeSelectableSportTupleAcceptsSelectableCanonicalTuple(t *testing.T) {
	tuple, patch, err := NormalizeSelectableSportTuple("mountain_biking", "")
	if err != nil {
		t.Fatalf("NormalizeSelectableSportTuple() error = %v", err)
	}
	if tuple != (Tuple{SportType: "cycling", SubType: "mountain"}) {
		t.Fatalf("tuple = %#v, want cycling/mountain", tuple)
	}
	if patch.IsElectric != nil {
		t.Fatalf("patch.IsElectric = %v, want nil for non-electric legacy mountain bike", *patch.IsElectric)
	}
}

func TestNormalizeSportTupleAcceptsCaseInsensitiveCanonicalLegacyIDs(t *testing.T) {
	gotTuple, gotPatch, err := NormalizeSportTuple(" Cycling ", " Generic ")
	if err != nil {
		t.Fatalf("NormalizeSportTuple() error = %v", err)
	}
	if gotTuple != (Tuple{SportType: "cycling"}) {
		t.Fatalf("tuple = %#v, want broad cycling", gotTuple)
	}
	if gotPatch != (ModifierPatch{}) {
		t.Fatalf("patch = %#v, want empty", gotPatch)
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
	if gotPatch.IsElectric != nil || gotPatch.IsIndoor != nil || gotPatch.IsVirtual != nil {
		t.Fatalf("patch = %#v, want empty", gotPatch)
	}
}

func TestNormalizeSportTupleForImportStillDropsDeviceModeSubTypes(t *testing.T) {
	gotTuple, _, err := NormalizeSportTupleForImport("cycling", "commuting")
	if err != nil {
		t.Fatalf("NormalizeSportTupleForImport() error = %v", err)
	}
	if gotTuple != (Tuple{SportType: "cycling"}) {
		t.Fatalf("tuple = %#v, want broad cycling", gotTuple)
	}
}

func TestGearDefaultProjectionIncludesBroadFallbackParentsForStorageOnly(t *testing.T) {
	storage := gearDefaultOptionsBySportType(GearDefaultStorageSportOptions())
	manual := gearDefaultOptionsBySportType(GearDefaultManualSportOptions())
	for _, want := range []string{"fitness_equipment", "generic"} {
		if _, ok := storage[want]; !ok {
			t.Fatalf("storage options missing %q", want)
		}
	}
	if _, ok := storage["unknown"]; ok {
		t.Fatal("storage options include unknown")
	}
	if _, ok := manual["cycling"]; !ok {
		t.Fatal("manual options missing cycling")
	}
	if _, ok := manual["fitness_equipment"]; ok {
		t.Fatal("manual options include fitness_equipment")
	}
}

func TestGearDefaultSportForSourceUsesGeneratedProjection(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{raw: "Mountain Bike Ride", want: "cycling"},
		{raw: "Badminton", want: "racket"},
		{raw: "Elliptical", want: "fitness_equipment"},
		{raw: "Virtual Ride", want: "cycling"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := GearDefaultSportForSource(SourceStrava, tt.raw)
			if !ok || got != tt.want {
				t.Fatalf("GearDefaultSportForSource(SourceStrava, %q) = %q, %v; want %q, true", tt.raw, got, ok, tt.want)
			}
		})
	}
}

func TestGearDefaultSportForSourceSkipsUnknownStravaSource(t *testing.T) {
	got, ok := GearDefaultSportForSource(SourceStrava, "Definitely Not A Known Sport")
	if ok || got != "" {
		t.Fatalf("GearDefaultSportForSource unknown Strava = %q, %v; want empty, false", got, ok)
	}
}

func TestModifierPatchJSONUsesSnakeCaseKeys(t *testing.T) {
	var patch ModifierPatch
	data := []byte(`{"is_electric":true,"is_indoor":false,"is_virtual":true,"is_commute":false}`)
	if err := json.Unmarshal(data, &patch); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	assertBoolPatch(t, "is_electric", patch.IsElectric, boolPtr(true))
	assertBoolPatch(t, "is_indoor", patch.IsIndoor, boolPtr(false))
	assertBoolPatch(t, "is_virtual", patch.IsVirtual, boolPtr(true))
	assertBoolPatch(t, "is_commute", patch.IsCommute, boolPtr(false))

	rendered, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, want := range []string{`"is_electric":true`, `"is_indoor":false`, `"is_virtual":true`, `"is_commute":false`} {
		if !strings.Contains(string(rendered), want) {
			t.Fatalf("rendered JSON %s missing %s", rendered, want)
		}
	}
}

func TestApplyModifierPatchHandlesCommute(t *testing.T) {
	got := ApplyModifierPatch(ModifierValues{IsCommute: true}, ModifierPatch{IsCommute: boolPtr(false)})
	if got.IsCommute {
		t.Fatal("ApplyModifierPatch did not apply is_commute=false")
	}
	if ModifierPatchIsEmpty(&ModifierPatch{IsCommute: boolPtr(false)}) {
		t.Fatal("ModifierPatchIsEmpty treated is_commute patch as empty")
	}
}

func TestGearDefaultSportForSourceDoesNotFallbackUnknownNonStravaSource(t *testing.T) {
	for _, source := range []Source{SourceGPXFile, SourceKomoot} {
		got, ok := GearDefaultSportForSource(source, "Definitely Not A Known Sport")
		if ok || got != "" {
			t.Fatalf("GearDefaultSportForSource(%q, unknown) = %q, %v; want empty, false", source, got, ok)
		}
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

func gearDefaultOptionsBySportType(options []GearDefaultSportOption) map[string]GearDefaultSportOption {
	out := make(map[string]GearDefaultSportOption, len(options))
	for _, option := range options {
		out[option.SportType] = option
	}
	return out
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("slice length = %d, want %d; got %#v want %#v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("slice[%d] = %q, want %q; got %#v want %#v", i, got[i], want[i], got, want)
		}
	}
}
