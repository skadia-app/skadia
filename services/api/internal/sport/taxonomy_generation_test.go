package sport

import (
	"bytes"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestGeneratedBackendTaxonomyIsCurrent(t *testing.T) {
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	got, err := sporttaxgen.RenderGo(taxonomy)
	if err != nil {
		t.Fatalf("render Go taxonomy: %v", err)
	}
	want, err := os.ReadFile("taxonomy_generated.go")
	if err != nil {
		t.Fatalf("read taxonomy_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("taxonomy_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}
