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

func TestGeneratedBackendSegmentProjectionIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	activityFile, err := os.Open("../../../../config/sport-taxonomy/projections/activity_filters.yaml")
	if err != nil {
		t.Fatalf("open activity_filters.yaml: %v", err)
	}
	defer activityFile.Close()

	activityProjection, err := sporttaxgen.LoadActivityFilterProjections(activityFile, taxonomy)
	if err != nil {
		t.Fatalf("load activity filter projections: %v", err)
	}
	activityExpanded, err := sporttaxgen.ExpandActivityFilterPresets(taxonomy, activityProjection)
	if err != nil {
		t.Fatalf("expand activity filter projections: %v", err)
	}
	segmentFile, err := os.Open("../../../../config/sport-taxonomy/projections/segments.yaml")
	if err != nil {
		t.Fatalf("open segments.yaml: %v", err)
	}
	defer segmentFile.Close()
	segmentProjection, err := sporttaxgen.LoadSegmentProjection(segmentFile, taxonomy)
	if err != nil {
		t.Fatalf("load segment projection: %v", err)
	}
	segmentExpanded, err := sporttaxgen.ExpandSegmentProjection(taxonomy, segmentProjection)
	if err != nil {
		t.Fatalf("expand segment projection: %v", err)
	}
	got, err := sporttaxgen.RenderGoProjections(activityExpanded, segmentExpanded)
	if err != nil {
		t.Fatalf("render Go projections: %v", err)
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

func TestGeneratedPredicateValidationSQLIsCurrent(t *testing.T) {
	taxonomy := loadTestTaxonomy(t)
	got, err := sporttaxgen.RenderPredicateValidationSQL(taxonomy)
	if err != nil {
		t.Fatalf("render predicate validation SQL: %v", err)
	}
	want, err := os.ReadFile("predicate_validation_generated.sql")
	if err != nil {
		t.Fatalf("read predicate_validation_generated.sql: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("predicate_validation_generated.sql is stale; run scripts/generate-sport-taxonomy.sh")
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
