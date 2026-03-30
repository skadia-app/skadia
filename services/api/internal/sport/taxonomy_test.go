package sport

import (
	"testing"
)

func TestSegmentGroupBackwardsCompat(t *testing.T) {
	checks := map[string]string{
		"cycling": "bike", "mountain_biking": "bike",
		"gravel_cycling": "bike", "indoor_cycling": "bike",
		"running": "foot", "trail_running": "foot", "hiking": "foot",
		"walking": "foot", "treadmill_running": "foot",
		"swimming": "swim", "open_water_swimming": "swim", "lap_swimming": "swim",
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
		"running": 0.5, "trail_running": 0.5,
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
		"trail_running": "Trailrun", "mountaineering": "Bergtour",
		"inline_skating": "Inline-Skaten", "open_water_swimming": "Freiwasserschwimmen",
		"mountain_biking": "MTB-Tour", "gravel_cycling": "Gravel-Tour",
		"generic": "Training",
	}
	for st, expected := range titles {
		got := AutoTitle(st, "")
		if got != expected {
			t.Errorf("AutoTitle(%q, \"\") = %q, want %q", st, got, expected)
		}
	}
}

func TestSubTypePromotion(t *testing.T) {
	tests := []struct {
		sport, sub, wantLabel string
	}{
		{"training", "yoga", "Yoga"},
		{"training", "strength_training", "Krafttraining"},
		{"training", "cardio_training", "Cardio"},
		{"training", "", "Training"},
		{"training", "generic", "Training"},
		{"fitness_equipment", "elliptical", "Crosstrainer"},
		{"fitness_equipment", "treadmill", "Laufband"},
		{"fitness_equipment", "", "Fitnessgerät"},
	}
	for _, tc := range tests {
		got := SportLabel(tc.sport, tc.sub)
		if got != tc.wantLabel {
			t.Errorf("SportLabel(%q, %q) = %q, want %q", tc.sport, tc.sub, got, tc.wantLabel)
		}
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

func TestSportDetailLabel(t *testing.T) {
	tests := []struct {
		sport, sub, want string
	}{
		{"cycling", "mountain", "Radfahren (MTB)"},
		{"cycling", "", "Radfahren"},
		{"cycling", "generic", "Radfahren"},
		{"training", "yoga", "Yoga"},
		{"training", "", "Training"},
	}
	for _, tc := range tests {
		got := SportDetailLabel(tc.sport, tc.sub)
		if got != tc.want {
			t.Errorf("SportDetailLabel(%q, %q) = %q, want %q", tc.sport, tc.sub, got, tc.want)
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
		{"indoor_cycling", "", true},
		{"lap_swimming", "", true},
		{"yoga", "", true},
		{"pilates", "", true},
		{"treadmill_running", "", true},
		{"hiit", "", true},
		{"dance", "", true},
		{"jump_rope", "", true},
		{"meditation", "", true},
		{"boxing", "", true},
		{"mixed_martial_arts", "", true},
		{"floor_climbing", "", true},
		{"strength", "", true}, // alias

		// Sport-level IsIndoor: false
		{"running", "", false},
		{"cycling", "", false},
		{"hiking", "", false},
		{"diving", "", false},
		{"swimming", "", false},
		{"rock_climbing", "", false},
		{"open_water_swimming", "", false},

		// SubType patterns
		{"running", "indoor_running", true},
		{"running", "treadmill", true},
		{"cycling", "indoor_cycling", true},
		{"cycling", "spin", true},
		{"swimming", "lap_swimming", true},
		{"rowing", "indoor_rowing", true},
		{"alpine_skiing", "indoor_skiing", true},
		{"walking", "indoor_walking", true},
		{"rock_climbing", "indoor_climbing", true},

		// SubType: not indoor
		{"running", "street", false},
		{"running", "trail", false},
		{"cycling", "road", false},
		{"swimming", "open_water", false},

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
