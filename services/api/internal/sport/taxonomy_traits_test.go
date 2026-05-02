package sport

import (
	"reflect"
	"sort"
	"testing"
)

func TestGeneratedSportTraits(t *testing.T) {
	running := mustSportDef(t, "running")
	assertTraits(t, running.Traits, SportTraits{
		Environments:      []string{"land"},
		MovementFamilies:  []string{"run"},
		EquipmentFamilies: []string{},
		ActivityForm:      "single",
		PropulsionModes:   []string{"human"},
	})

	paddling := mustSportDef(t, "paddling")
	if paddling.Traits.ActivityForm != "fallback" {
		t.Fatalf("paddling activity_form = %q, want fallback", paddling.Traits.ActivityForm)
	}

	racket := mustSportDef(t, "racket")
	if racket.Lifecycle != "canonical" || !racket.Selectable || racket.Traits.ActivityForm != "family" {
		t.Fatalf("racket lifecycle/selectable/activity_form = %s/%t/%s, want canonical/true/family", racket.Lifecycle, racket.Selectable, racket.Traits.ActivityForm)
	}
	if got := racket.SubTypes["squash"].Traits.ActivityForm; got != "single" {
		t.Fatalf("racket/squash activity_form = %q, want single", got)
	}
	assertTraits(t, racket.SubTypes["badminton"].Traits, SportTraits{
		Environments:      []string{"land"},
		MovementFamilies:  []string{"strike"},
		EquipmentFamilies: []string{"racket"},
		ActivityForm:      "single",
		PropulsionModes:   []string{"human"},
	})

	generic := mustSportDef(t, "generic")
	assertTraits(t, generic.Traits, SportTraits{
		Environments:      []string{"generic"},
		MovementFamilies:  []string{"generic"},
		EquipmentFamilies: []string{"generic"},
		ActivityForm:      "system",
		PropulsionModes:   []string{"generic"},
	})

	sup := mustSportDef(t, "stand_up_paddleboarding")
	if !reflect.DeepEqual(sup.Traits.EquipmentFamilies, []string{"board", "paddle"}) {
		t.Fatalf("stand_up_paddleboarding equipment = %#v, want board+paddle", sup.Traits.EquipmentFamilies)
	}

	treadmill := running.SubTypes["treadmill"].Traits
	if !reflect.DeepEqual(treadmill.Environments, []string{"indoor"}) || !reflect.DeepEqual(treadmill.EquipmentFamilies, []string{"machine"}) {
		t.Fatalf("running/treadmill traits = %#v, want indoor machine", treadmill)
	}

	fitnessEquipment := mustSportDef(t, "fitness_equipment")
	if got := fitnessEquipment.SubTypes["elliptical"].Traits.ActivityForm; got != "single" {
		t.Fatalf("fitness_equipment/elliptical activity_form = %q, want single", got)
	}
	if got := fitnessEquipment.SubTypes["pilates"].Traits.MovementFamilies; !reflect.DeepEqual(got, []string{"mind_body"}) {
		t.Fatalf("fitness_equipment/pilates movement = %#v, want mind_body", got)
	}

	diving := mustSportDef(t, "diving")
	if got := diving.SubTypes["apnea_hunting"].Traits.EquipmentFamilies; !reflect.DeepEqual(got, []string{"fishing_gear"}) {
		t.Fatalf("diving/apnea_hunting equipment = %#v, want fishing_gear", got)
	}

	flying := mustSportDef(t, "flying")
	if got := flying.SubTypes["fly_paramotor"].Traits.PropulsionModes; !reflect.DeepEqual(got, []string{"motor"}) {
		t.Fatalf("flying/fly_paramotor propulsion = %#v, want motor", got)
	}
	if got := flying.SubTypes["fly_vfr"].Traits; got.ActivityForm != "mode" || !reflect.DeepEqual(got.PropulsionModes, []string{"generic"}) {
		t.Fatalf("flying/fly_vfr traits = %#v, want mode/generic", got)
	}
}

func TestGeneratedTraitChangeDoesNotChangeSportLists(t *testing.T) {
	wantAllSports := []string{
		"alpine_skiing", "american_football", "backcountry_skiing", "baseball", "basketball", "boating", "boxing", "canoeing", "cricket", "cross_country_skiing", "cycling", "dance", "disc_golf", "diving", "driving", "fishing", "fitness_equipment", "floor_climbing", "flying", "generic", "golf", "hang_gliding", "hiit", "hiking", "hockey", "horseback_riding", "hunting", "ice_skating", "inline_skating", "jump_rope", "jumpmaster", "kayaking", "kitesurfing", "lacrosse", "meditation", "mixed_martial_arts", "motorcycling", "mountaineering", "multisport", "paddling", "pilates", "racket", "rafting", "rock_climbing", "rowing", "rugby", "running", "sailing", "skiing", "sky_diving", "sledding", "snorkeling", "snowboarding", "snowmobiling", "snowshoeing", "soccer", "stand_up_paddleboarding", "strength_training", "surfing", "swimming", "tactical", "tennis", "training", "transition", "unknown", "volleyball", "wakeboarding", "wakesurfing", "walking", "water_skiing", "water_tubing", "wheelchair_push", "windsurfing", "yoga",
	}
	if got := sportTypes(AllSports()); !reflect.DeepEqual(got, wantAllSports) {
		t.Fatalf("AllSports() = %#v, want %#v", got, wantAllSports)
	}

	wantByCategory := map[string][]string{
		"bike":    {"cycling"},
		"climb":   {"rock_climbing"},
		"fitness": {"boxing", "dance", "fitness_equipment", "floor_climbing", "hiit", "jump_rope", "meditation", "mixed_martial_arts", "pilates", "strength_training", "training", "yoga"},
		"game":    {"american_football", "baseball", "basketball", "cricket", "disc_golf", "golf", "hockey", "lacrosse", "racket", "rugby", "soccer", "tennis", "volleyball"},
		"multi":   {"multisport", "transition"},
		"other":   {"generic", "unknown"},
		"outdoor": {"driving", "flying", "hang_gliding", "horseback_riding", "hunting", "inline_skating", "jumpmaster", "motorcycling", "sky_diving", "tactical"},
		"run":     {"hiking", "mountaineering", "running", "walking", "wheelchair_push"},
		"water":   {"boating", "canoeing", "diving", "fishing", "kayaking", "kitesurfing", "paddling", "rafting", "rowing", "sailing", "snorkeling", "stand_up_paddleboarding", "surfing", "swimming", "wakeboarding", "wakesurfing", "water_skiing", "water_tubing", "windsurfing"},
		"winter":  {"alpine_skiing", "backcountry_skiing", "cross_country_skiing", "ice_skating", "skiing", "sledding", "snowboarding", "snowmobiling", "snowshoeing"},
	}
	for category, want := range wantByCategory {
		if got := sportTypes(SportsInCategory(category)); !reflect.DeepEqual(got, want) {
			t.Fatalf("SportsInCategory(%q) = %#v, want %#v", category, got, want)
		}
	}

	if got := CategoryForSport("swimming").Key; got != "water" {
		t.Fatalf("CategoryForSport(swimming) = %q, want water", got)
	}
	if got := CategoryForSport("generic").Key; got != "other" {
		t.Fatalf("CategoryForSport(generic) = %q, want other", got)
	}
}

func mustSportDef(t *testing.T, sportType string) SportDef {
	t.Helper()
	def, ok := GetSportDef(sportType)
	if !ok {
		t.Fatalf("missing sport %q", sportType)
	}
	return def
}

func assertTraits(t *testing.T, got, want SportTraits) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("traits = %#v, want %#v", got, want)
	}
}

func sportTypes(defs []SportDef) []string {
	result := make([]string, 0, len(defs))
	for _, def := range defs {
		result = append(result, def.SportType)
	}
	sort.Strings(result)
	return result
}
