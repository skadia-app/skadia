package sport

import (
	"bytes"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestGeneratedBackendTaxonomyIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
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

func TestGeneratedBackendActivityFilterProjectionsAreCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	file, err := os.Open("../../../../config/sport-taxonomy/projections/activity_filters.yaml")
	if err != nil {
		t.Fatalf("open activity_filters.yaml: %v", err)
	}
	defer file.Close()

	projections, err := sporttaxgen.LoadActivityFilterProjections(file, taxonomy)
	if err != nil {
		t.Fatalf("load activity filter projections: %v", err)
	}
	expanded, err := sporttaxgen.ExpandActivityFilterPresets(taxonomy, projections)
	if err != nil {
		t.Fatalf("expand activity filter projections: %v", err)
	}
	got, err := sporttaxgen.RenderGoActivityFilterProjections(expanded)
	if err != nil {
		t.Fatalf("render Go activity filter projections: %v", err)
	}
	want, err := os.ReadFile("projections_generated.go")
	if err != nil {
		t.Fatalf("read projections_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("projections_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestCategoryForSportIncludesGeneratedColor(t *testing.T) {
	cat := CategoryForSport("running")
	if cat.Color != "#EF4444" {
		t.Fatalf("running category color = %q, want #EF4444", cat.Color)
	}
}

func loadTestTaxonomy(t *testing.T) sporttaxgen.Taxonomy {
	t.Helper()
	file, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer file.Close()

	taxonomy, err := sporttaxgen.LoadTaxonomy(file)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	return taxonomy
}
