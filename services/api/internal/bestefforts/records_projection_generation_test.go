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
	modifierFile, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer modifierFile.Close()
	modifiers, err := sporttaxgen.LoadModifiers(modifierFile)
	if err != nil {
		t.Fatalf("load modifiers: %v", err)
	}

	projectionFile, err := os.Open("../../../../config/sport-taxonomy/projections/records.yaml")
	if err != nil {
		t.Fatalf("open records.yaml: %v", err)
	}
	defer projectionFile.Close()
	projection, err := sporttaxgen.LoadRecordsProjection(projectionFile, taxonomy, modifiers)
	if err != nil {
		t.Fatalf("load records projection: %v", err)
	}
	expanded, err := sporttaxgen.ExpandRecordsProjection(taxonomy, projection, modifiers)
	if err != nil {
		t.Fatalf("expand records projection: %v", err)
	}
	got, err := sporttaxgen.RenderGoRecordsProjection(expanded, modifiers)
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

func TestGeneratedFrontendRecordsProjectionIsCurrent(t *testing.T) {
	taxonomyFile, err := os.Open("../../../../config/sport-taxonomy/sports.yaml")
	if err != nil {
		t.Fatalf("open sports.yaml: %v", err)
	}
	defer taxonomyFile.Close()
	taxonomy, err := sporttaxgen.LoadTaxonomy(taxonomyFile)
	if err != nil {
		t.Fatalf("load taxonomy: %v", err)
	}
	modifierFile, err := os.Open("../../../../config/sport-taxonomy/modifiers.yaml")
	if err != nil {
		t.Fatalf("open modifiers.yaml: %v", err)
	}
	defer modifierFile.Close()
	modifiers, err := sporttaxgen.LoadModifiers(modifierFile)
	if err != nil {
		t.Fatalf("load modifiers: %v", err)
	}
	projectionFile, err := os.Open("../../../../config/sport-taxonomy/projections/records.yaml")
	if err != nil {
		t.Fatalf("open records.yaml: %v", err)
	}
	defer projectionFile.Close()
	projection, err := sporttaxgen.LoadRecordsProjection(projectionFile, taxonomy, modifiers)
	if err != nil {
		t.Fatalf("load records projection: %v", err)
	}
	expanded, err := sporttaxgen.ExpandRecordsProjection(taxonomy, projection, modifiers)
	if err != nil {
		t.Fatalf("expand records projection: %v", err)
	}
	got, err := sporttaxgen.RenderTSRecordsProjection(expanded, modifiers)
	if err != nil {
		t.Fatalf("render TypeScript records projection: %v", err)
	}
	want, err := os.ReadFile("../../../../frontend/src/lib/generatedRecordProjections.ts")
	if err != nil {
		t.Fatalf("read generatedRecordProjections.ts: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("generatedRecordProjections.ts is stale; run scripts/generate-sport-taxonomy.sh")
	}
}
