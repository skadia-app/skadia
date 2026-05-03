package bestefforts

import (
	"bytes"
	"os"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sporttaxgen"
)

func TestGeneratedRecordsProjectionIsCurrent(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := sporttaxgen.LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}

	projectionFile, err := os.Open("../../../../config/sport-taxonomy/projections/records.yaml")
	if err != nil {
		t.Fatalf("open records.yaml: %v", err)
	}
	defer projectionFile.Close()
	projection, err := sporttaxgen.LoadRecordsProjection(projectionFile, taxonomy)
	if err != nil {
		t.Fatalf("load records projection: %v", err)
	}
	expanded, err := sporttaxgen.ExpandRecordsProjection(taxonomy, projection)
	if err != nil {
		t.Fatalf("expand records projection: %v", err)
	}
	got, err := sporttaxgen.RenderGoRecordsProjection(expanded)
	if err != nil {
		t.Fatalf("render records projection: %v", err)
	}
	want, err := os.ReadFile("records_projection_generated.go")
	if err != nil {
		t.Fatalf("read records_projection_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("records_projection_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}
