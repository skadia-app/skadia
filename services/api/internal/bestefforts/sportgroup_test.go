package bestefforts

import "testing"

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
		{name: "trail running", sportType: "trail_running", want: "running"},
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
		{name: "wheelchair push walk", sportType: "wheelchair_push_walk", want: ""},
		{name: "wheelchair push run", sportType: "wheelchair_push_run", want: ""},

		// ── Cycling ──
		{name: "cycling outdoor", sportType: "cycling", want: "cycling"},
		{name: "mountain biking", sportType: "mountain_biking", want: "cycling"},
		{name: "gravel cycling", sportType: "gravel_cycling", want: "cycling"},
		{name: "indoor cycling", sportType: "indoor_cycling", want: "cycling"},
		{name: "cycling electric", sportType: "cycling", isElectric: true, want: "cycling_electric"},
		{name: "cycling virtual", sportType: "cycling", isVirtual: true, want: "cycling_virtual"},
		{name: "virtual overrides electric", sportType: "cycling", isVirtual: true, isElectric: true, want: "cycling_virtual"},

		// ── Swimming ──
		{name: "lap_swimming sport type", sportType: "lap_swimming", want: "swim_pool"},
		{name: "open_water_swimming sport type", sportType: "open_water_swimming", want: "swim_ow"},
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
		{
			sportGroup: "running", wantMinLen: 15 + 4 + 9 + 4,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: true, wantHasSuperlat: true,
		},
		{
			sportGroup: "running_indoor", wantMinLen: 15 + 9 + 4,
			wantDistKey: "dist_400m", wantHasPower: true, wantHasElev: false, wantHasSuperlat: true,
		},
		{
			sportGroup: "running_virtual", wantMinLen: 15 + 4,
			wantDistKey: "dist_400m", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{
			sportGroup: "cycling", wantMinLen: 11 + 4 + 9 + 4,
			wantDistKey: "dist_10km", wantHasPower: true, wantHasElev: true, wantHasSuperlat: true,
		},
		{
			sportGroup: "cycling_electric", wantMinLen: 11 + 4 + 4,
			wantDistKey: "dist_10km", wantHasPower: false, wantHasElev: true, wantHasSuperlat: true,
		},
		{
			sportGroup: "cycling_virtual", wantMinLen: 11 + 4,
			wantDistKey: "dist_10km", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{
			sportGroup: "hiking", wantMinLen: 5 + 4 + 4,
			wantDistKey: "dist_5km", wantHasPower: false, wantHasElev: true, wantHasSuperlat: true,
		},
		{
			sportGroup: "swim_pool", wantMinLen: 6 + 4,
			wantDistKey: "dist_50m", wantHasPower: false, wantHasElev: false, wantHasSuperlat: true,
		},
		{
			sportGroup: "swim_ow", wantMinLen: 3 + 4,
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
		{"dist_1mi", 1609, "1 Meile"},
		{"dist_hm", 21097, "Halbmarathon"},
		{"dist_marathon", 42195, "Marathon"},
		{"dist_10mi", 16093, "10 Meilen"},
	}
	for _, ch := range checks {
		c, ok := byKey[ch.key]
		if !ok {
			t.Errorf("running categories missing key %q", ch.key)
			continue
		}
		if c.DistanceM != ch.wantM {
			t.Errorf("running[%q].DistanceM = %.0f, want %.0f", ch.key, c.DistanceM, ch.wantM)
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

	checks := []struct{ key string; wantS int }{
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
