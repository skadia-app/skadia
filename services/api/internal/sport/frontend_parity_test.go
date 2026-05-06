package sport

import (
	"bytes"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestGeneratedFrontendTaxonomyIsCurrent(t *testing.T) {
	source, err := os.ReadFile("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("read sports.yaml: %v", err)
	}
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()
	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load sports.yaml: %v", err)
	}
	got, err := sporttaxgen.RenderTS(taxonomy, source)
	if err != nil {
		t.Fatalf("render generated frontend taxonomy: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedSportTaxonomy.ts")
	if err != nil {
		t.Fatalf("read generatedSportTaxonomy.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedSportTaxonomy.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}
