package sport

import (
	"os"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestSportsYAMLMatchesCurrentBackendRegistry(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}

	categories := AllCategories()
	if len(taxonomy.Categories) != len(categories) {
		t.Fatalf("category count = %d, want %d", len(taxonomy.Categories), len(categories))
	}
	for i, got := range taxonomy.Categories {
		want := categories[i]
		if got.Key != want.Key || got.Labels["de"] != want.LabelDE {
			t.Fatalf("category[%d] = %s/%s, want %s/%s", i, got.Key, got.Labels["de"], want.Key, want.LabelDE)
		}
	}

	gotSports := taxonomy.Sports
	wantSports := AllSports()
	if len(gotSports) != len(wantSports) {
		t.Fatalf("sport count = %d, want %d", len(gotSports), len(wantSports))
	}
	for i, got := range gotSports {
		want := wantSports[i]
		if got.SportType != want.SportType ||
			got.CategoryKey != want.CategoryKey ||
			got.Labels["de"] != want.LabelDE ||
			got.AutoTitles["de"] != want.AutoTitleDE ||
			got.SegmentGroup != want.SegmentGroup ||
			got.TypicallyHasGPS != want.TypicallyHasGPS ||
			got.MovingThreshold != want.MovingThreshold ||
			got.IsIndoor != want.IsIndoor ||
			got.Lifecycle != want.Lifecycle ||
			got.Selectable == nil ||
			*got.Selectable != want.Selectable ||
			!sameTraits(got.Traits, want.Traits) {
			t.Fatalf("sport[%d] %q does not match backend registry: got %#v want %#v", i, got.SportType, got, want)
		}
		if len(got.SubTypes) != len(want.SubTypes) {
			t.Fatalf("sport %q subtype count = %d, want %d", got.SportType, len(got.SubTypes), len(want.SubTypes))
		}
		wantSubTypes := OrderedSubTypes(want)
		for j, gotSub := range got.SubTypes {
			wantSub := wantSubTypes[j]
			if gotSub.SubType != wantSub.SubType ||
				gotSub.Labels["de"] != wantSub.LabelDE ||
				gotSub.Details["de"] != wantSub.DetailDE ||
				gotSub.AutoTitles["de"] != wantSub.AutoTitleDE ||
				gotSub.Selectable == nil ||
				*gotSub.Selectable != wantSub.Selectable ||
				!sameOptionalFloat64(gotSub.MovingThreshold, wantSub.MovingThreshold) ||
				!sameTraits(sporttaxgen.EffectiveSubTypeTraits(got, gotSub), wantSub.Traits) {
				t.Fatalf("sport %q subtype[%d] = %#v, want %#v", got.SportType, j, gotSub, wantSub)
			}
		}
	}
}

func sameOptionalFloat64(got, want *float64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func sameTraits(got sporttaxgen.Traits, want SportTraits) bool {
	return reflect.DeepEqual(got.Environments, want.Environments) &&
		reflect.DeepEqual(got.MovementFamilies, want.MovementFamilies) &&
		reflect.DeepEqual(got.EquipmentFamilies, want.EquipmentFamilies) &&
		got.ActivityForm == want.ActivityForm &&
		reflect.DeepEqual(got.PropulsionModes, want.PropulsionModes)
}
