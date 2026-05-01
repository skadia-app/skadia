package sport

import (
	"os"
	"sort"
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

	gotSports := append([]sporttaxgen.Sport(nil), taxonomy.Sports...)
	sort.Slice(gotSports, func(i, j int) bool { return gotSports[i].SportType < gotSports[j].SportType })
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
			*got.Selectable != want.Selectable {
			t.Fatalf("sport[%d] %q does not match backend registry: got %#v want %#v", i, got.SportType, got, want)
		}
		if len(got.SubTypes) != len(want.SubTypes) {
			t.Fatalf("sport %q subtype count = %d, want %d", got.SportType, len(got.SubTypes), len(want.SubTypes))
		}
		gotSubTypes := append([]sporttaxgen.SubType(nil), got.SubTypes...)
		sort.Slice(gotSubTypes, func(i, j int) bool { return gotSubTypes[i].SubType < gotSubTypes[j].SubType })
		wantKeys := make([]string, 0, len(want.SubTypes))
		for key := range want.SubTypes {
			wantKeys = append(wantKeys, key)
		}
		sort.Strings(wantKeys)
		for j, gotSub := range gotSubTypes {
			wantSub := want.SubTypes[wantKeys[j]]
			if gotSub.SubType != wantSub.SubType ||
				gotSub.Labels["de"] != wantSub.LabelDE ||
				gotSub.Details["de"] != wantSub.DetailDE ||
				gotSub.AutoTitles["de"] != wantSub.AutoTitleDE ||
				gotSub.Selectable == nil ||
				*gotSub.Selectable != wantSub.Selectable ||
				!sameOptionalFloat64(gotSub.MovingThreshold, wantSub.MovingThreshold) {
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
