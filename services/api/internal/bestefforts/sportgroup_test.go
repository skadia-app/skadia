package bestefforts

import (
	"reflect"
	"testing"
)

func TestDetermineSportGroup(t *testing.T) {
	tests := []struct {
		name       string
		sportType  string
		subType    string
		isIndoor   bool
		isVirtual  bool
		isElectric bool
		want       string
	}{
		// ── Running ──
		{name: "running outdoor", sportType: "running", want: "running"},
		{name: "running with sub_type trail", sportType: "running", subType: "trail", want: "running_trail"},
		{name: "running sub_type trail indoor overrides", sportType: "running", subType: "trail", isIndoor: true, want: "running_indoor"},
		{name: "running sub_type trail virtual overrides", sportType: "running", subType: "trail", isVirtual: true, want: "running_virtual"},
		{name: "treadmill via isIndoor", sportType: "running", isIndoor: true, want: "running_indoor"},
		{name: "virtual run via isVirtual", sportType: "running", isVirtual: true, want: "running_virtual"},
		{name: "virtual run overrides indoor", sportType: "running", isIndoor: true, isVirtual: true, want: "running_virtual"},

		// hiking group
		{name: "hiking", sportType: "hiking", want: "hiking"},
		{name: "mountaineering", sportType: "mountaineering", want: "hiking"},
		{name: "snowshoeing", sportType: "snowshoeing", want: "hiking"},

		// no best efforts
		{name: "walking", sportType: "walking", want: ""},
		{name: "floor climbing", sportType: "floor_climbing", want: ""},
		{name: "wheelchair push", sportType: "wheelchair_push", want: ""},

		// ── Cycling ──
		{name: "cycling outdoor", sportType: "cycling", want: "cycling"},
		{name: "cycling parent-only generic subtype", sportType: "cycling", subType: "generic", want: "cycling"},
		{name: "mountain biking", sportType: "cycling", subType: "mountain", want: "cycling"},
		{name: "gravel cycling", sportType: "cycling", subType: "gravel_cycling", want: "cycling"},
		{name: "spin subtype uses taxonomy indoor default", sportType: "cycling", subType: "spin", want: "cycling_virtual"},
		{name: "cycling electric", sportType: "cycling", isElectric: true, want: "cycling_electric"},
		{name: "cycling virtual", sportType: "cycling", isVirtual: true, want: "cycling_virtual"},
		{name: "virtual overrides electric", sportType: "cycling", isVirtual: true, isElectric: true, want: "cycling_virtual"},
		{name: "indoor cycling is cycling_virtual", sportType: "cycling", isIndoor: true, want: "cycling_virtual"},
		{name: "indoor cycling electric is cycling_virtual", sportType: "cycling", isIndoor: true, isElectric: true, want: "cycling_virtual"}, // Indoor > Electric

		// ── Swimming ──
		{name: "swimming with lap subtype", sportType: "swimming", subType: "lap_swimming", want: "swim_pool"},
		{name: "swimming without lap subtype", sportType: "swimming", want: "swim_ow"},
		{name: "swimming open_water subtype", sportType: "swimming", subType: "open_water", want: "swim_ow"},

		// ── Unsupported sports ──
		{name: "alpine skiing", sportType: "alpine_skiing", want: ""},
		{name: "yoga", sportType: "yoga", want: ""},
		{name: "unknown sport", sportType: "totally_unknown", want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetermineSportGroup(tc.sportType, tc.subType, tc.isIndoor, tc.isVirtual, tc.isElectric)
			if got != tc.want {
				t.Errorf("DetermineSportGroup(%q, %q, indoor=%v, virtual=%v, electric=%v) = %q, want %q",
					tc.sportType, tc.subType, tc.isIndoor, tc.isVirtual, tc.isElectric, got, tc.want)
			}
		})
	}
}

func TestRecordGroupForActivityWithMissingModifierBooleansUsesTaxonomyDefaults(t *testing.T) {
	got, ok := recordGroupForActivityWithModifierPresence("cycling", "spin", false, false, false, false, false, false)
	if !ok {
		t.Fatal("recordGroupForActivityWithModifierPresence(cycling, spin) returned no group")
	}
	if got != "cycling_virtual" {
		t.Fatalf("recordGroupForActivityWithModifierPresence(cycling, spin) = %q, want cycling_virtual", got)
	}
}

func TestRecordProjectionKeepsExistingStoredGroupKeys(t *testing.T) {
	got := []string{}
	for _, group := range RecordGroups() {
		got = append(got, group.Key)
	}
	want := []string{"running", "running_trail", "running_indoor", "running_virtual", "cycling", "cycling_electric", "cycling_virtual", "hiking", "swim_pool", "swim_ow"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("record groups = %#v, want %#v", got, want)
	}
}

func TestCategoriesFor(t *testing.T) {
	tests := []struct {
		sportGroup      string
		wantNil         bool
		wantMinLen      int
		wantDistKey     string // first distance category key expected
		wantHasPower    bool
		wantHasElev     bool
		wantHasSuperlat bool
	}{
		{ // 15 dist + 4 elev + 9 power + 4 superlatives (with elevation)
			sportGroup: "running", wantMinLen: 15 + 4 + 9 + 4,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: true, wantHasSuperlat: true,
		},
		{ // 15 dist + 9 power + 2 superlatives (basic, no elevation indoors)
			sportGroup: "running_indoor", wantMinLen: 15 + 9 + 2,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: false, wantHasSuperlat: true,
		},
		{ // 15 dist + 9 power + 2 superlatives (basic, no elevation for virtual)
			sportGroup: "running_virtual", wantMinLen: 15 + 9 + 2,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: false, wantHasSuperlat: true,
		},
		{ // 15 dist + 4 elev + 9 power + 4 superlatives (with elevation)
			sportGroup: "running_trail", wantMinLen: 15 + 4 + 9 + 4,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: true, wantHasSuperlat: true,
		},
		{ // 11 dist + 4 elev + 9 power + 4 superlatives (with elevation)
			sportGroup: "cycling", wantMinLen: 11 + 4 + 9 + 4,
			wantDistKey: "dist_10km", wantHasPower: true, wantHasElev: true, wantHasSuperlat: true,
		},
		{ // 11 dist + 4 elev + 4 superlatives (with elevation, e-bike has real hills)
			sportGroup: "cycling_electric", wantMinLen: 11 + 4 + 4,
			wantDistKey: "dist_10km", wantHasPower: false, wantHasElev: true, wantHasSuperlat: true,
		},
		{ // 11 dist + 2 superlatives (basic, simulated elevation)
			sportGroup: "cycling_virtual", wantMinLen: 11 + 2,
			wantDistKey: "dist_10km", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{ // 5 dist + 4 elev + 4 superlatives (with elevation)
			sportGroup: "hiking", wantMinLen: 5 + 4 + 4,
			wantDistKey: "dist_5km", wantHasPower: false, wantHasElev: true, wantHasSuperlat: true,
		},
		{ // 6 dist + 2 superlatives (basic, no elevation in pool)
			sportGroup: "swim_pool", wantMinLen: 6 + 2,
			wantDistKey: "dist_50m", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{ // 3 dist + 2 superlatives (basic, no meaningful elevation)
			sportGroup: "swim_ow", wantMinLen: 3 + 2,
			wantDistKey: "dist_1km", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{sportGroup: "", wantNil: true},
		{sportGroup: "unknown_group", wantNil: true},
	}

	for _, tc := range tests {
		t.Run(tc.sportGroup, func(t *testing.T) {
			cats := CategoriesFor(tc.sportGroup)
			if tc.wantNil {
				if cats != nil {
					t.Errorf("CategoriesFor(%q) = %v, want nil", tc.sportGroup, cats)
				}
				return
			}
			if len(cats) < tc.wantMinLen {
				t.Errorf("CategoriesFor(%q) len=%d, want >= %d", tc.sportGroup, len(cats), tc.wantMinLen)
			}
			// first category is the expected distance
			if len(cats) > 0 && cats[0].Key != tc.wantDistKey {
				t.Errorf("CategoriesFor(%q)[0].Key = %q, want %q", tc.sportGroup, cats[0].Key, tc.wantDistKey)
			}
			hasPower, hasElev, hasSuperlat := false, false, false
			for _, c := range cats {
				switch c.Type {
				case TypeValueHigher:
					if c.DurationS > 0 {
						hasPower = true
					}
				case TypeTimeLower:
					if c.ElevationM > 0 {
						hasElev = true
					}
				case TypeValueSuperlative:
					hasSuperlat = true
				}
			}
			if hasPower != tc.wantHasPower {
				t.Errorf("CategoriesFor(%q) hasPower=%v, want %v", tc.sportGroup, hasPower, tc.wantHasPower)
			}
			if hasElev != tc.wantHasElev {
				t.Errorf("CategoriesFor(%q) hasElev=%v, want %v", tc.sportGroup, hasElev, tc.wantHasElev)
			}
			if hasSuperlat != tc.wantHasSuperlat {
				t.Errorf("CategoriesFor(%q) hasSuperlat=%v, want %v", tc.sportGroup, hasSuperlat, tc.wantHasSuperlat)
			}
		})
	}
}

func TestCategoriesForReturnsCopy(t *testing.T) {
	cats1 := CategoriesFor("running")
	cats2 := CategoriesFor("running")
	if len(cats1) == 0 {
		t.Fatal("CategoriesFor(running) returned empty slice")
	}
	// Mutating the returned slice must not affect subsequent calls.
	cats1[0].Key = "mutated"
	if cats2[0].Key == "mutated" {
		t.Error("CategoriesFor returned a shared slice; expected a defensive copy")
	}
}

func TestCategoriesForAllRecordGroupsExactKeys(t *testing.T) {
	cases := map[string][]string{
		"running":          {"dist_400m", "dist_800m", "dist_1km", "dist_1mi", "dist_2mi", "dist_5km", "dist_10km", "dist_15km", "dist_10mi", "dist_20km", "dist_hm", "dist_30km", "dist_marathon", "dist_50km", "dist_100km", "elev_500hm", "elev_1000hm", "elev_1500hm", "elev_2000hm", "power_5s", "power_30s", "power_1min", "power_5min", "power_10min", "power_20min", "power_30min", "power_1h", "power_2h", "sup_longest_distance", "sup_most_elevation", "sup_biggest_climb", "sup_longest_duration"},
		"running_trail":    {"dist_400m", "dist_800m", "dist_1km", "dist_1mi", "dist_2mi", "dist_5km", "dist_10km", "dist_15km", "dist_10mi", "dist_20km", "dist_hm", "dist_30km", "dist_marathon", "dist_50km", "dist_100km", "elev_500hm", "elev_1000hm", "elev_1500hm", "elev_2000hm", "power_5s", "power_30s", "power_1min", "power_5min", "power_10min", "power_20min", "power_30min", "power_1h", "power_2h", "sup_longest_distance", "sup_most_elevation", "sup_biggest_climb", "sup_longest_duration"},
		"running_indoor":   {"dist_400m", "dist_800m", "dist_1km", "dist_1mi", "dist_2mi", "dist_5km", "dist_10km", "dist_15km", "dist_10mi", "dist_20km", "dist_hm", "dist_30km", "dist_marathon", "dist_50km", "dist_100km", "power_5s", "power_30s", "power_1min", "power_5min", "power_10min", "power_20min", "power_30min", "power_1h", "power_2h", "sup_longest_distance", "sup_longest_duration"},
		"running_virtual":  {"dist_400m", "dist_800m", "dist_1km", "dist_1mi", "dist_2mi", "dist_5km", "dist_10km", "dist_15km", "dist_10mi", "dist_20km", "dist_hm", "dist_30km", "dist_marathon", "dist_50km", "dist_100km", "power_5s", "power_30s", "power_1min", "power_5min", "power_10min", "power_20min", "power_30min", "power_1h", "power_2h", "sup_longest_distance", "sup_longest_duration"},
		"cycling":          {"dist_10km", "dist_20km", "dist_25mi", "dist_40km", "dist_50km", "dist_50mi", "dist_100km", "dist_100mi", "dist_150km", "dist_200km", "dist_300km", "elev_500hm", "elev_1000hm", "elev_1500hm", "elev_2000hm", "power_5s", "power_30s", "power_1min", "power_5min", "power_10min", "power_20min", "power_30min", "power_1h", "power_2h", "sup_longest_distance", "sup_most_elevation", "sup_biggest_climb", "sup_longest_duration"},
		"cycling_electric": {"dist_10km", "dist_20km", "dist_25mi", "dist_40km", "dist_50km", "dist_50mi", "dist_100km", "dist_100mi", "dist_150km", "dist_200km", "dist_300km", "elev_500hm", "elev_1000hm", "elev_1500hm", "elev_2000hm", "sup_longest_distance", "sup_most_elevation", "sup_biggest_climb", "sup_longest_duration"},
		"cycling_virtual":  {"dist_10km", "dist_20km", "dist_25mi", "dist_40km", "dist_50km", "dist_50mi", "dist_100km", "dist_100mi", "dist_150km", "dist_200km", "dist_300km", "sup_longest_distance", "sup_longest_duration"},
		"hiking":           {"dist_5km", "dist_10km", "dist_20km", "dist_30km", "dist_50km", "elev_500hm", "elev_1000hm", "elev_1500hm", "elev_2000hm", "sup_longest_distance", "sup_most_elevation", "sup_biggest_climb", "sup_longest_duration"},
		"swim_pool":        {"dist_50m", "dist_100m", "dist_200m", "dist_400m", "dist_800m", "dist_1500m", "sup_longest_distance", "sup_longest_duration"},
		"swim_ow":          {"dist_1km", "dist_2km", "dist_5km", "sup_longest_distance", "sup_longest_duration"},
	}
	for group, want := range cases {
		if got := categoryKeys(CategoriesFor(group)); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s category keys = %#v, want %#v", group, got, want)
		}
	}
}

func TestRecordProjectionAccessorsReturnDeepCopies(t *testing.T) {
	groups := RecordGroups()
	groups[0].Categories[0].Key = "mutated"
	if again := RecordGroups(); again[0].Categories[0].Key == "mutated" {
		t.Fatal("RecordGroups returned shared category slice")
	}

	mainCategories := RecordMainCategories()
	mainCategories[0].RecordGroups[0] = "mutated"
	mainCategories[0].SubFilters[0].RecordGroups[0] = "mutated"
	mainCategories[0].PowerCurve.RecordGroups[0] = "mutated"
	again := RecordMainCategories()
	if again[0].RecordGroups[0] == "mutated" || again[0].SubFilters[0].RecordGroups[0] == "mutated" || again[0].PowerCurve.RecordGroups[0] == "mutated" {
		t.Fatal("RecordMainCategories returned shared nested slices")
	}
}

func categoryKeys(categories []Category) []string {
	keys := make([]string, 0, len(categories))
	for _, category := range categories {
		keys = append(keys, category.Key)
	}
	return keys
}

func TestDistanceCategoryValues(t *testing.T) {
	cats := CategoriesFor("running")
	byKey := make(map[string]Category, len(cats))
	for _, c := range cats {
		byKey[c.Key] = c
	}

	checks := []struct {
		key     string
		wantM   float64
		wantLbl string
	}{
		{"dist_1mi", 1609.34, "1 Meile"},
		{"dist_hm", 21097.5, "Halbmarathon"},
		{"dist_marathon", 42195, "Marathon"},
		{"dist_10mi", 16093.4, "10 Meilen"},
	}
	for _, ch := range checks {
		c, ok := byKey[ch.key]
		if !ok {
			t.Errorf("running categories missing key %q", ch.key)
			continue
		}
		if c.DistanceM != ch.wantM {
			t.Errorf("running[%q].DistanceM = %v, want %v", ch.key, c.DistanceM, ch.wantM)
		}
		if c.Label != ch.wantLbl {
			t.Errorf("running[%q].Label = %q, want %q", ch.key, c.Label, ch.wantLbl)
		}
	}
}

func TestPowerCategoryDurations(t *testing.T) {
	cats := CategoriesFor("cycling")
	byKey := make(map[string]Category, len(cats))
	for _, c := range cats {
		byKey[c.Key] = c
	}

	checks := []struct {
		key   string
		wantS int
	}{
		{"power_5s", 5},
		{"power_30s", 30},
		{"power_1min", 60},
		{"power_5min", 300},
		{"power_20min", 1200},
		{"power_1h", 3600},
		{"power_2h", 7200},
	}
	for _, ch := range checks {
		c, ok := byKey[ch.key]
		if !ok {
			t.Errorf("cycling categories missing key %q", ch.key)
			continue
		}
		if c.DurationS != ch.wantS {
			t.Errorf("cycling[%q].DurationS = %d, want %d", ch.key, c.DurationS, ch.wantS)
		}
	}
}
