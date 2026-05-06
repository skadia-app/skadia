package sport

import (
	"reflect"
	"testing"
)

func TestSegmentGroupBackwardsCompat(t *testing.T) {
	checks := map[string]string{
		"cycling": "bike",
		"running": "foot", "hiking": "foot",
		"walking":       "foot",
		"swimming":      "swim",
		"alpine_skiing": "ski", "cross_country_skiing": "ski",
		"backcountry_skiing": "ski", "snowboarding": "ski",
		"kayaking": "paddle", "stand_up_paddleboarding": "paddle", "rowing": "paddle",
		"generic": "generic", "unknown": "generic",
	}
	for st, expected := range checks {
		got := SegmentGroup(st)
		if got != expected {
			t.Errorf("SegmentGroup(%q) = %q, want %q", st, got, expected)
		}
	}
}

func TestMovingThresholdBackwardsCompat(t *testing.T) {
	thresholds := map[string]float64{
		"cycling": 0.5,
		"running": 0.5,
		"walking": 0.2, "hiking": 0.2,
		"swimming": 0.1,
		"rowing":   0.1, "kayaking": 0.1, "stand_up_paddleboarding": 0.1, "paddling": 0.1,
		"alpine_skiing": 0.3, "cross_country_skiing": 0.3, "snowboarding": 0.3,
		"strength_training": 0.0, "training": 0.0, "generic": 0.0,
	}
	for st, expected := range thresholds {
		got := MovingThreshold(st)
		if got != expected {
			t.Errorf("MovingThreshold(%q) = %v, want %v", st, got, expected)
		}
	}
}

func TestAutoTitleBackwardsCompat(t *testing.T) {
	titles := map[string]string{
		"cycling": "Radfahrt", "running": "Lauf", "hiking": "Wanderung",
		"walking": "Spaziergang", "swimming": "Schwimmen",
		"cross_country_skiing": "Langlauf", "paddling": "Wassersport-Tour",
		"rock_climbing": "Klettern", "stand_up_paddleboarding": "SUP-Tour",
		"mountaineering": "Bergtour",
		"inline_skating": "Inline-Skaten",
		"generic":        "Training",
	}
	for st, expected := range titles {
		got := AutoTitle(st, "")
		if got != expected {
			t.Errorf("AutoTitle(%q, \"\") = %q, want %q", st, got, expected)
		}
	}
}

func TestSubTypeLabels(t *testing.T) {
	tests := []struct {
		sport, sub, wantLabel string
	}{
		{"cycling", "mountain", "MTB"},
		{"running", "indoor_running", "Indoor"},
		{"yoga", "", "Yoga"},
		{"pilates", "", "Pilates"},
		{"strength_training", "", "Krafttraining"},
		{"training", "cardio_training", "Cardio"},
		{"training", "", "Training"},
		{"training", "generic", "Training"},
		{"fitness_equipment", "elliptical", "Crosstrainer"},
		{"fitness_equipment", "", "Fitnessgerät"},
	}
	for _, tc := range tests {
		got := SportLabel(tc.sport, tc.sub)
		if got != tc.wantLabel {
			t.Errorf("SportLabel(%q, %q) = %q, want %q", tc.sport, tc.sub, got, tc.wantLabel)
		}
	}
}

func TestSportDefHasNoSubTypePromotionField(t *testing.T) {
	if _, ok := reflect.TypeOf(SportDef{}).FieldByName("SubTypePromotion"); ok {
		t.Fatal("SportDef still has SubTypePromotion field")
	}
}

func TestFallbackUnknownSport(t *testing.T) {
	label := SportLabel("totally_new_sport", "")
	if label != "Totally New Sport" {
		t.Errorf("SportLabel(totally_new_sport) = %q, want 'Totally New Sport'", label)
	}
	sg := SegmentGroup("totally_new_sport")
	if sg != "generic" {
		t.Errorf("SegmentGroup(totally_new_sport) = %q, want 'generic'", sg)
	}
	mt := MovingThreshold("totally_new_sport")
	if mt != 0.0 {
		t.Errorf("MovingThreshold(totally_new_sport) = %v, want 0.0", mt)
	}
	at := AutoTitle("totally_new_sport", "")
	if at != "Training" {
		t.Errorf("AutoTitle(totally_new_sport) = %q, want 'Training'", at)
	}
	cat := CategoryForSport("totally_new_sport")
	if cat.Key != "other" {
		t.Errorf("CategoryForSport(totally_new_sport).Key = %q, want 'other'", cat.Key)
	}
}

func TestAllCategories(t *testing.T) {
	cats := AllCategories()
	if len(cats) != 10 {
		t.Errorf("AllCategories() returned %d, want 10", len(cats))
	}
}

func TestAllSports(t *testing.T) {
	all := AllSports()
	if len(all) < 60 {
		t.Errorf("AllSports() returned %d, want at least 60", len(all))
	}
	legacyAliases := map[string]bool{
		"mountain_biking":      true,
		"trail_running":        true,
		"gravel_cycling":       true,
		"indoor_cycling":       true,
		"treadmill_running":    true,
		"lap_swimming":         true,
		"open_water_swimming":  true,
		"wheelchair_push_walk": true,
		"wheelchair_push_run":  true,
		"strength":             true,
	}
	for _, d := range all {
		if legacyAliases[d.SportType] {
			t.Fatalf("AllSports() included legacy alias %q", d.SportType)
		}
	}
}

func TestAllSportsUsesTaxonomyOrder(t *testing.T) {
	all := AllSports()
	wantPrefix := []string{"running", "walking", "hiking", "mountaineering", "snowshoeing", "floor_climbing", "wheelchair_push", "cycling"}
	if len(all) < len(wantPrefix) {
		t.Fatalf("AllSports() returned %d sports, want at least %d", len(all), len(wantPrefix))
	}
	for i, want := range wantPrefix {
		if all[i].SportType != want {
			t.Fatalf("AllSports()[%d] = %q, want taxonomy order %q", i, all[i].SportType, want)
		}
	}
}

func TestSportsInCategory(t *testing.T) {
	bike := SportsInCategory("bike")
	if len(bike) == 0 {
		t.Error("SportsInCategory('bike') returned empty")
	}
	for _, s := range bike {
		if s.CategoryKey != "bike" {
			t.Errorf("SportsInCategory('bike') returned sport %q with category %q", s.SportType, s.CategoryKey)
		}
	}
}

func TestSportsInCategoryUsesTaxonomyOrder(t *testing.T) {
	run := SportsInCategory("run")
	want := []string{"running", "walking", "hiking", "mountaineering", "wheelchair_push"}
	if got := sportTypesInOrder(run); !reflect.DeepEqual(got, want) {
		t.Fatalf("SportsInCategory(run) = %#v, want taxonomy order %#v", got, want)
	}
}

func TestSportsInCategoryOrderMatchesAllSportsFiltering(t *testing.T) {
	for _, category := range AllCategories() {
		want := make([]string, 0)
		for _, def := range AllSports() {
			if def.CategoryKey == category.Key {
				want = append(want, def.SportType)
			}
		}
		if got := sportTypesInOrder(SportsInCategory(category.Key)); !reflect.DeepEqual(got, want) {
			t.Fatalf("SportsInCategory(%q) = %#v, want taxonomy order %#v", category.Key, got, want)
		}
	}
}

func TestGetSportDef(t *testing.T) {
	d, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef('cycling') returned false")
	}
	if d.LabelDE != "Radfahren" {
		t.Errorf("GetSportDef('cycling').LabelDE = %q, want 'Radfahren'", d.LabelDE)
	}
	_, ok = GetSportDef("nonexistent")
	if ok {
		t.Error("GetSportDef('nonexistent') should return false")
	}
}

func TestOrderedSubTypesUsesTaxonomyOrder(t *testing.T) {
	cycling, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef(cycling) returned false")
	}
	want := []string{
		"road",
		"mountain",
		"downhill",
		"recumbent",
		"cyclocross",
		"hand_cycling",
		"track_cycling",
		"indoor_cycling",
		"spin",
		"gravel_cycling",
		"unicycling",
		"mixed_surface",
		"bmx",
	}
	if got := subTypesInOrder(OrderedSubTypes(cycling)); !reflect.DeepEqual(got, want) {
		t.Fatalf("OrderedSubTypes(cycling) = %#v, want taxonomy order %#v", got, want)
	}
}

func TestReturnedSportDefsDoNotShareSubTypeOrder(t *testing.T) {
	cycling, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef(cycling) returned false")
	}
	cycling.SubTypeOrder[0] = "mutated"
	cyclingAgain, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef(cycling) second call returned false")
	}
	if cyclingAgain.SubTypeOrder[0] != "road" {
		t.Fatalf("GetSportDef returned shared SubTypeOrder backing array, got first subtype %q", cyclingAgain.SubTypeOrder[0])
	}

	all := AllSports()
	all[7].SubTypeOrder[0] = "mutated"
	allAgain := AllSports()
	if allAgain[7].SubTypeOrder[0] != "road" {
		t.Fatalf("AllSports returned shared SubTypeOrder backing array, got first cycling subtype %q", allAgain[7].SubTypeOrder[0])
	}
}

func TestReturnedSportDefsAreDeepCloned(t *testing.T) {
	t.Run("GetSportDef", func(t *testing.T) {
		cycling, ok := GetSportDef("cycling")
		if !ok {
			t.Fatal("GetSportDef(cycling) returned false")
		}
		mutateReturnedSportDef(cycling)
		assertCyclingDefUnchanged(t, func() (SportDef, bool) {
			return GetSportDef("cycling")
		})
	})

	t.Run("AllSports", func(t *testing.T) {
		all := AllSports()
		cycling, ok := findSportDef(all, "cycling")
		if !ok {
			t.Fatal("AllSports missing cycling")
		}
		mutateReturnedSportDef(cycling)
		assertCyclingDefUnchanged(t, func() (SportDef, bool) {
			allAgain := AllSports()
			return findSportDef(allAgain, "cycling")
		})
	})

	t.Run("SportsInCategory", func(t *testing.T) {
		bike := SportsInCategory("bike")
		cycling, ok := findSportDef(bike, "cycling")
		if !ok {
			t.Fatal("SportsInCategory(bike) missing cycling")
		}
		mutateReturnedSportDef(cycling)
		assertCyclingDefUnchanged(t, func() (SportDef, bool) {
			bikeAgain := SportsInCategory("bike")
			return findSportDef(bikeAgain, "cycling")
		})
	})
}

func TestReturnedSubTypeMovingThresholdsAreCloned(t *testing.T) {
	wheelchair, ok := GetSportDef("wheelchair_push")
	if !ok {
		t.Fatal("GetSportDef(wheelchair_push) returned false")
	}
	subType := wheelchair.SubTypes["walk_speed"]
	if subType.MovingThreshold == nil {
		t.Fatal("wheelchair_push/walk_speed missing moving threshold fixture")
	}
	*subType.MovingThreshold = 99
	wheelchair.SubTypes["walk_speed"] = subType

	wheelchairAgain, ok := GetSportDef("wheelchair_push")
	if !ok {
		t.Fatal("GetSportDef(wheelchair_push) second call returned false")
	}
	got := wheelchairAgain.SubTypes["walk_speed"].MovingThreshold
	if got == nil || *got != 0.2 {
		t.Fatalf("GetSportDef returned shared subtype moving threshold pointer, got %v", got)
	}
}

func TestReturnedSportModifierDefaultsAreCloned(t *testing.T) {
	floorClimbing, ok := GetSportDef("floor_climbing")
	if !ok {
		t.Fatal("GetSportDef(floor_climbing) returned false")
	}
	if floorClimbing.ModifierDefaults.IsIndoor == nil {
		t.Fatal("floor_climbing missing modifier default fixture")
	}
	*floorClimbing.ModifierDefaults.IsIndoor = false
	floorClimbingAgain, ok := GetSportDef("floor_climbing")
	if !ok {
		t.Fatal("GetSportDef(floor_climbing) second call returned false")
	}
	if floorClimbingAgain.ModifierDefaults.IsIndoor == nil || !*floorClimbingAgain.ModifierDefaults.IsIndoor {
		t.Fatalf("GetSportDef returned shared modifier default pointer, got %#v", floorClimbingAgain.ModifierDefaults)
	}

	cycling, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef(cycling) returned false")
	}
	indoorCycling := cycling.SubTypes["indoor_cycling"]
	if indoorCycling.ModifierDefaults.IsIndoor == nil {
		t.Fatal("cycling/indoor_cycling missing modifier default fixture")
	}
	*indoorCycling.ModifierDefaults.IsIndoor = false
	cycling.SubTypes["indoor_cycling"] = indoorCycling
	cyclingAgain, ok := GetSportDef("cycling")
	if !ok {
		t.Fatal("GetSportDef(cycling) second call returned false")
	}
	indoorCyclingAgain := cyclingAgain.SubTypes["indoor_cycling"]
	if indoorCyclingAgain.ModifierDefaults.IsIndoor == nil || !*indoorCyclingAgain.ModifierDefaults.IsIndoor {
		t.Fatalf("GetSportDef returned shared subtype modifier default pointer, got %#v", indoorCyclingAgain.ModifierDefaults)
	}
}

func TestEffectiveTraitsForTupleReturnsClone(t *testing.T) {
	traits, ok := EffectiveTraitsForTuple("cycling", "mountain")
	if !ok {
		t.Fatal("EffectiveTraitsForTuple(cycling, mountain) returned false")
	}
	traits.Environments[0] = "mutated"
	traits.MovementFamilies[0] = "mutated"
	traits.EquipmentFamilies[0] = "mutated"
	traits.PropulsionModes[0] = "mutated"

	traitsAgain, ok := EffectiveTraitsForTuple("cycling", "mountain")
	if !ok {
		t.Fatal("EffectiveTraitsForTuple(cycling, mountain) second call returned false")
	}
	if got, want := traitsAgain.Environments[0], "land"; got != want {
		t.Fatalf("EffectiveTraitsForTuple returned shared Environments backing array, got %q", got)
	}
	if got, want := traitsAgain.MovementFamilies[0], "pedal"; got != want {
		t.Fatalf("EffectiveTraitsForTuple returned shared MovementFamilies backing array, got %q", got)
	}
	if got, want := traitsAgain.EquipmentFamilies[0], "bike"; got != want {
		t.Fatalf("EffectiveTraitsForTuple returned shared EquipmentFamilies backing array, got %q", got)
	}
	if got, want := traitsAgain.PropulsionModes[0], "human"; got != want {
		t.Fatalf("EffectiveTraitsForTuple returned shared PropulsionModes backing array, got %q", got)
	}
}

func TestSportDefLifecycleDefaults(t *testing.T) {
	d, ok := GetSportDef("running")
	if !ok {
		t.Fatal("GetSportDef(running) returned false")
	}
	if d.Lifecycle != "canonical" {
		t.Fatalf("running lifecycle = %q, want canonical", d.Lifecycle)
	}
	if !d.Selectable {
		t.Fatal("running selectable = false, want true")
	}
}

func TestIsSelectableTupleHonorsParentAndSubtypeFlags(t *testing.T) {
	if !IsSelectableTuple("cycling", "") {
		t.Fatal("cycling parent tuple should be selectable")
	}
	if !IsSelectableTuple("fitness_equipment", "elliptical") {
		t.Fatal("fitness_equipment/elliptical subtype should be selectable")
	}
	if IsSelectableTuple("fitness_equipment", "") {
		t.Fatal("fitness_equipment parent tuple should not be selectable")
	}
	if IsSelectableTuple("unknown", "") {
		t.Fatal("unknown should not be selectable for user-authored activity creation")
	}
	if IsSelectableTuple("cycling", "does_not_exist") {
		t.Fatal("unknown subtype should not be selectable")
	}
}

func TestLegacyAliasesAreNotRuntimeSports(t *testing.T) {
	for _, sportType := range []string{
		"mountain_biking",
		"trail_running",
		"gravel_cycling",
		"indoor_cycling",
		"treadmill_running",
		"lap_swimming",
		"open_water_swimming",
		"wheelchair_push_walk",
		"wheelchair_push_run",
		"strength",
	} {
		if _, ok := GetSportDef(sportType); ok {
			t.Fatalf("GetSportDef(%q) returned true after compatibility cleanup", sportType)
		}
	}
}

func TestMovingThresholdForTupleUsesSubTypeOverride(t *testing.T) {
	if got := MovingThresholdForTuple("wheelchair_push", "walk_speed"); got != 0.2 {
		t.Fatalf("walk threshold = %v, want 0.2", got)
	}
	if got := MovingThresholdForTuple("wheelchair_push", "run_speed"); got != 0.5 {
		t.Fatalf("run threshold = %v, want 0.5", got)
	}
	if got := MovingThreshold("wheelchair_push"); got != 0.2 {
		t.Fatalf("parent threshold = %v, want 0.2", got)
	}
}

func TestSportDetailLabel(t *testing.T) {
	tests := []struct {
		sport, sub, want string
	}{
		{"cycling", "mountain", "Radfahren (MTB)"},
		{"cycling", "", "Radfahren"},
		{"cycling", "generic", "Radfahren"},
		{"running", "indoor_running", "Laufen (Indoor)"},
		{"yoga", "", "Yoga"},
		{"pilates", "", "Pilates"},
		{"training", "", "Training"},
		{"paddling", "", "Paddeln (allgemein)"},
	}
	for _, tc := range tests {
		got := SportDetailLabel(tc.sport, tc.sub)
		if got != tc.want {
			t.Errorf("SportDetailLabel(%q, %q) = %q, want %q", tc.sport, tc.sub, got, tc.want)
		}
	}
}

func TestEffectiveTraitsForTupleUsesParentForParentOnlyAndUnknownSubTypes(t *testing.T) {
	parent, ok := EffectiveTraitsForTuple("cycling", "")
	if !ok {
		t.Fatal("EffectiveTraitsForTuple(cycling, \"\") returned false")
	}
	for _, subType := range []string{"generic", " Generic ", "device_private_mode"} {
		got, ok := EffectiveTraitsForTuple("cycling", subType)
		if !ok {
			t.Fatalf("EffectiveTraitsForTuple(cycling, %q) returned false", subType)
		}
		if !reflect.DeepEqual(got, parent) {
			t.Fatalf("EffectiveTraitsForTuple(cycling, %q) = %#v, want parent %#v", subType, got, parent)
		}
	}
}

func TestIsIndoor(t *testing.T) {
	tests := []struct {
		sport, sub string
		want       bool
	}{
		// Sport-level IsIndoor: true
		{"training", "", true},
		{"fitness_equipment", "", true},
		{"strength_training", "", true},
		{"yoga", "", true},
		{"pilates", "", true},
		{"hiit", "", true},
		{"dance", "", true},
		{"jump_rope", "", true},
		{"meditation", "", true},
		{"boxing", "", true},
		{"mixed_martial_arts", "", true},
		{"floor_climbing", "", true},

		// Sport-level IsIndoor: false
		{"running", "", false},
		{"cycling", "", false},
		{"hiking", "", false},
		{"diving", "", false},
		{"swimming", "", false},
		{"rock_climbing", "", false},

		// SubType patterns
		{"running", "indoor_running", true},
		{"running", "treadmill", true},
		{"cycling", "indoor_cycling", true},
		{"cycling", "spin", true},
		{"swimming", "lap_swimming", false},
		{"rowing", "indoor_rowing", true},
		{"alpine_skiing", "indoor_skiing", true},
		{"walking", "indoor_walking", true},
		{"rock_climbing", "indoor_climbing", true},
		{"diving", "apnea_pool", true},

		// SubType: not indoor
		{"running", "street", false},
		{"running", "trail", false},
		{"cycling", "road", false},
		{"swimming", "open_water", false},
		{"rock_climbing", "bouldering", false},

		// Unknown sport
		{"nonexistent_sport", "", false},
	}
	for _, tt := range tests {
		got := IsIndoor(tt.sport, tt.sub)
		if got != tt.want {
			t.Errorf("IsIndoor(%q, %q) = %v, want %v", tt.sport, tt.sub, got, tt.want)
		}
	}
}

func TestModifierDefaultsForTuple(t *testing.T) {
	defaults := ModifierDefaultsForTuple("diving", "apnea_pool")
	if defaults.IsIndoor == nil || !*defaults.IsIndoor {
		t.Fatalf("diving/apnea_pool modifier defaults = %#v, want is_indoor true", defaults)
	}

	if got := ModifierDefaultsForTuple("rock_climbing", "bouldering"); got.IsIndoor != nil {
		t.Fatalf("rock_climbing/bouldering defaults = %#v, want no indoor default", got)
	}
	if got := ModifierDefaultsForTuple("running", "treadmill"); got.IsIndoor == nil || !*got.IsIndoor {
		t.Fatalf("running/treadmill defaults = %#v, want is_indoor true", got)
	}
	if got := ModifierDefaultsForTuple("yoga", ""); got.IsIndoor == nil || !*got.IsIndoor {
		t.Fatalf("yoga defaults = %#v, want is_indoor true", got)
	}
	if got := ModifierDefaultsForTuple("fitness_equipment", "indoor_rowing"); got.IsIndoor == nil || !*got.IsIndoor {
		t.Fatalf("fitness_equipment/indoor_rowing defaults = %#v, want inherited is_indoor true", got)
	}
	if got := ModifierDefaultsForTuple("fitness_equipment", "device_private_mode"); got.IsIndoor == nil || !*got.IsIndoor {
		t.Fatalf("fitness_equipment unknown subtype defaults = %#v, want inherited is_indoor true", got)
	}
	if got := ModifierDefaultsForTuple("nonexistent_sport", ""); got.IsIndoor != nil {
		t.Fatalf("unknown sport defaults = %#v, want zero defaults", got)
	}
}

func TestModifierDefaultsForTupleReturnsCopy(t *testing.T) {
	first := ModifierDefaultsForTuple("running", "treadmill")
	if first.IsIndoor == nil || !*first.IsIndoor {
		t.Fatalf("running/treadmill defaults = %#v, want is_indoor true", first)
	}
	*first.IsIndoor = false

	second := ModifierDefaultsForTuple("running", "treadmill")
	if second.IsIndoor == nil || !*second.IsIndoor {
		t.Fatalf("mutating returned defaults changed registry: %#v", second)
	}
}

func sportTypesInOrder(defs []SportDef) []string {
	result := make([]string, 0, len(defs))
	for _, def := range defs {
		result = append(result, def.SportType)
	}
	return result
}

func subTypesInOrder(defs []SubTypeDef) []string {
	result := make([]string, 0, len(defs))
	for _, def := range defs {
		result = append(result, def.SubType)
	}
	return result
}

func findSportDef(defs []SportDef, sportType string) (SportDef, bool) {
	for _, def := range defs {
		if def.SportType == sportType {
			return def, true
		}
	}
	return SportDef{}, false
}

func mutateReturnedSportDef(def SportDef) {
	def.Traits.Environments[0] = "mutated"
	def.ModifierDefaults.IsIndoor = ptrBool(true)
	subType := def.SubTypes["mountain"]
	subType.Traits.EquipmentFamilies[0] = "mutated"
	subType.ModifierDefaults.IsIndoor = ptrBool(true)
	def.SubTypes["mountain"] = subType
}

func assertCyclingDefUnchanged(t *testing.T, load func() (SportDef, bool)) {
	t.Helper()
	def, ok := load()
	if !ok {
		t.Fatal("fresh cycling lookup failed")
	}
	if got, want := def.Traits.Environments[0], "land"; got != want {
		t.Fatalf("cycling traits environment = %q, want %q", got, want)
	}
	if def.ModifierDefaults.IsIndoor != nil {
		t.Fatalf("cycling modifier default IsIndoor = %v, want nil", *def.ModifierDefaults.IsIndoor)
	}
	subType := def.SubTypes["mountain"]
	if got, want := subType.Traits.EquipmentFamilies[0], "bike"; got != want {
		t.Fatalf("cycling/mountain equipment family = %q, want %q", got, want)
	}
	if subType.ModifierDefaults.IsIndoor != nil {
		t.Fatalf("cycling/mountain modifier default IsIndoor = %v, want nil", *subType.ModifierDefaults.IsIndoor)
	}
}
