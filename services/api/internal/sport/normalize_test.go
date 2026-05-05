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

func TestGearDefaultPredicateForSourcePreservesSubtypeAndModifiers(t *testing.T) {
	tests := []struct {
		raw          string
		wantSport    string
		wantSubType  string
		wantElectric *bool
		wantVirtual  *bool
	}{
		{raw: "Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain"},
		{raw: "E-Mountain Bike Ride", wantSport: "cycling", wantSubType: "mountain", wantElectric: boolPtr(true)},
		{raw: "Virtual Ride", wantSport: "cycling", wantVirtual: boolPtr(true)},
		{raw: "Mystery Sport", wantSport: "generic"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := GearDefaultPredicateForSource(SourceStrava, tt.raw)
			if !ok {
				t.Fatalf("GearDefaultPredicateForSource(SourceStrava, %q) returned ok=false", tt.raw)
			}
			if got.SportType != tt.wantSport {
				t.Fatalf("SportType = %q, want %q", got.SportType, tt.wantSport)
			}
			gotSubType := ""
			if got.SubType != nil {
				gotSubType = *got.SubType
			}
			if gotSubType != tt.wantSubType {
				t.Fatalf("SubType = %q, want %q", gotSubType, tt.wantSubType)
			}
			var gotElectric, gotVirtual *bool
			if got.Modifiers != nil {
				gotElectric = got.Modifiers.IsElectric
				gotVirtual = got.Modifiers.IsVirtual
			}
			assertBoolPatch(t, "is_electric", gotElectric, tt.wantElectric)
			assertBoolPatch(t, "is_virtual", gotVirtual, tt.wantVirtual)
		})
	}
}

func TestGearDefaultSportForSourcePreservesStravaGenericFallback(t *testing.T) {
	got, ok := GearDefaultSportForSource(SourceStrava, "Definitely Not A Known Sport")
	if !ok || got != "generic" {
		t.Fatalf("GearDefaultSportForSource unknown Strava = %q, %v; want generic, true", got, ok)
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

func TestNormalizeGearDefaultPredicates(t *testing.T) {
	subType := "mountain"
	got, err := NormalizeGearDefaultPredicates([]GearDefaultPredicate{
		{
			SportType: " cycling ",
			SubType:   &subType,
			Modifiers: &ModifierPatch{IsElectric: boolPtr(true)},
		},
		{
			SportType: "cycling",
			SubType:   &subType,
			Modifiers: &ModifierPatch{IsElectric: boolPtr(true)},
		},
		{SportType: "fitness_equipment"},
	})
	if err != nil {
		t.Fatalf("NormalizeGearDefaultPredicates() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2: %#v", len(got), got)
	}
	if got[0].SportType != "cycling" || got[0].SubType == nil || *got[0].SubType != "mountain" {
		t.Fatalf("first predicate = %#v, want cycling mountain", got[0])
	}
	if got[0].Modifiers == nil {
		t.Fatal("first predicate modifiers = nil, want electric modifier")
	}
	assertBoolPatch(t, "is_electric", got[0].Modifiers.IsElectric, boolPtr(true))
	if got[1].SportType != "fitness_equipment" || got[1].SubType != nil || got[1].Modifiers != nil {
		t.Fatalf("second predicate = %#v, want broad fitness_equipment", got[1])
	}
}

func TestNormalizeGearDefaultPredicatesRejectsUnknown(t *testing.T) {
	if _, err := NormalizeGearDefaultPredicates([]GearDefaultPredicate{{SportType: "not_a_sport"}}); err == nil {
		t.Fatal("NormalizeGearDefaultPredicates accepted unknown sport")
	}
}

func TestGearDefaultPredicateMatchesActivity(t *testing.T) {
	mountain := "mountain"
	predicate := GearDefaultPredicate{
		SportType: "cycling",
		SubType:   &mountain,
		Modifiers: &ModifierPatch{IsElectric: boolPtr(true)},
	}
	if !GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", ModifierValues{IsElectric: true}) {
		t.Fatal("predicate did not match electric mountain cycling")
	}
	if GearDefaultPredicateMatchesActivity(predicate, "cycling", "mountain", ModifierValues{IsElectric: false}) {
		t.Fatal("predicate matched non-electric mountain cycling")
	}
	if GearDefaultPredicateMatchesActivity(predicate, "cycling", "road", ModifierValues{IsElectric: true}) {
		t.Fatal("predicate matched road cycling")
	}
}

func TestNormalizeGearDefaultSports(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []string
		wantErr bool
	}{
		{name: "nil input", values: nil, want: []string{}},
		{name: "empty input", values: []string{}, want: []string{}},
		{name: "trims and dedupes", values: []string{" cycling ", "cycling", "running"}, want: []string{"cycling", "running"}},
		{name: "rejects tuple", values: []string{"cycling:mountain"}, wantErr: true},
		{name: "rejects unknown", values: []string{"unknown"}, wantErr: true},
		{name: "accepts broad fallback storage parent", values: []string{"fitness_equipment"}, want: []string{"fitness_equipment"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeGearDefaultSports(tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeGearDefaultSports() returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeGearDefaultSports() error = %v", err)
			}
			assertStringSlice(t, got, tt.want)
			if got == nil {
				t.Fatal("NormalizeGearDefaultSports() returned nil slice")
			}
		})
	}
}

func TestNormalizeManualGearDefaultSports(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		want    []string
		wantErr bool
	}{
		{name: "nil input", values: nil, want: []string{}},
		{name: "empty input", values: []string{}, want: []string{}},
		{name: "trims and dedupes", values: []string{" cycling ", "cycling", "running"}, want: []string{"cycling", "running"}},
		{name: "rejects tuple", values: []string{"cycling:mountain"}, wantErr: true},
		{name: "rejects unknown", values: []string{"unknown"}, wantErr: true},
		{name: "rejects broad fallback storage parent", values: []string{"fitness_equipment"}, wantErr: true},
		{name: "accepts selectable top-level sport", values: []string{"cycling"}, want: []string{"cycling"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeManualGearDefaultSports(tt.values)
			if tt.wantErr {
				if err == nil {
					t.Fatal("NormalizeManualGearDefaultSports() returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeManualGearDefaultSports() error = %v", err)
			}
			assertStringSlice(t, got, tt.want)
			if got == nil {
				t.Fatal("NormalizeManualGearDefaultSports() returned nil slice")
			}
		})
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
