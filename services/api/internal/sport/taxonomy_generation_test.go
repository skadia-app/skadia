package sport

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestGeneratedBackendSourceAliasesAreCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	sources := loadTestSourceAliases(t, taxonomy)
	got, err := sporttaxgen.RenderGoSourceAliases(sources)
	if err != nil {
		t.Fatalf("render Go source aliases: %v", err)
	}
	want, err := os.ReadFile("aliases_generated.go")
	if err != nil {
		t.Fatalf("read aliases_generated.go: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("aliases_generated.go is stale; run scripts/generate-sport-taxonomy.sh")
	}
}

func TestCategoryForSportIncludesGeneratedColor(t *testing.T) {
	cat := CategoryForSport("running")
	if cat.Color != "#EF4444" {
		t.Fatalf("running category color = %q, want #EF4444", cat.Color)
	}
}

func loadTestSourceAliases(t *testing.T, taxonomy sporttaxgen.Taxonomy) []sporttaxgen.SourceAliases {
	t.Helper()
	matches, err := filepath.Glob("../../../../config/sport-taxonomy/aliases/*.yaml")
	if err != nil {
		t.Fatalf("glob aliases: %v", err)
	}
	files := make(map[string]io.Reader, len(matches))
	opened := make([]*os.File, 0, len(matches))
	defer func() {
		for _, file := range opened {
			_ = file.Close()
		}
	}()
	for _, match := range matches {
		file, err := os.Open(match)
		if err != nil {
			t.Fatalf("open alias file %s: %v", match, err)
		}
		opened = append(opened, file)
		stem := strings.TrimSuffix(filepath.Base(match), filepath.Ext(match))
		files[stem] = file
	}
	sources, err := sporttaxgen.LoadSourceAliasFiles(files, taxonomy)
	if err != nil {
		t.Fatalf("load source aliases: %v", err)
	}
	return sources
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
