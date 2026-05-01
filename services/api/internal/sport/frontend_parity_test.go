package sport

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendTaxonomyContainsBackendLabels(t *testing.T) {
	data, err := os.ReadFile("../../../../frontend/src/lib/generatedSportTaxonomy.ts")
	if err != nil {
		t.Fatalf("read generated frontend taxonomy: %v", err)
	}
	src := string(data)

	for _, def := range AllSports() {
		if !strings.Contains(src, "'"+def.SportType+"'") {
			t.Errorf("frontend taxonomy missing sport_type %q", def.SportType)
		}
		if !strings.Contains(src, "'"+def.LabelDE+"'") {
			t.Errorf("frontend taxonomy missing label %q for sport_type %q", def.LabelDE, def.SportType)
		}
		for _, sub := range def.SubTypes {
			if !strings.Contains(src, "'"+sub.SubType+"'") {
				t.Errorf("frontend taxonomy missing sub_type %q for sport_type %q", sub.SubType, def.SportType)
			}
			if !strings.Contains(src, "'"+sub.LabelDE+"'") {
				t.Errorf("frontend taxonomy missing label %q for %q/%q", sub.LabelDE, def.SportType, sub.SubType)
			}
		}
	}
}
