package enrichment

import (
	"testing"
)

func TestAggregateEdgesToSegmentEnrichment_AllSurface(t *testing.T) {
	// 10 edges, all asphalt (surface-id=1), sac/mtb unset.
	edges := make([]edgeSample, 10)
	for i := range edges {
		edges[i] = edgeSample{surfaceID: 1, sacID: 0, mtbID: 0, lengthM: 100}
	}
	resp := aggregateEdgesToSegmentEnrichment(edges, 1.0)
	if resp.Surface == nil || resp.Surface.Dominant != "Asphalt" {
		t.Fatalf("Surface.Dominant expected 'Asphalt', got %+v", resp.Surface)
	}
	if resp.Surface.Mixed {
		t.Fatalf("Mixed expected false for homogeneous asphalt")
	}
	if resp.SacScale != nil || resp.MtbScale != nil {
		t.Fatalf("sac/mtb expected nil when no edges tagged")
	}
}

func TestAggregateEdgesToSegmentEnrichment_Mixed(t *testing.T) {
	// 5 asphalt, 5 gravel → mixed.
	edges := []edgeSample{
		{surfaceID: 1, lengthM: 100}, {surfaceID: 1, lengthM: 100},
		{surfaceID: 1, lengthM: 100}, {surfaceID: 1, lengthM: 100},
		{surfaceID: 1, lengthM: 100},
		{surfaceID: 10, lengthM: 100}, {surfaceID: 10, lengthM: 100},
		{surfaceID: 10, lengthM: 100}, {surfaceID: 10, lengthM: 100},
		{surfaceID: 10, lengthM: 100},
	}
	resp := aggregateEdgesToSegmentEnrichment(edges, 1.0)
	if resp.Surface == nil || !resp.Surface.Mixed {
		t.Fatalf("Mixed expected true for 50/50 split, got %+v", resp.Surface)
	}
	if len(resp.Surface.Distribution) != 2 {
		t.Fatalf("Distribution expected 2 keys, got %d", len(resp.Surface.Distribution))
	}
}

func TestAggregateEdgesToSegmentEnrichment_LowCoverage(t *testing.T) {
	// 2 edges matched out of 10 total → coverage 0.2 < 0.30.
	edges := []edgeSample{{surfaceID: 1, lengthM: 100}, {surfaceID: 1, lengthM: 100}}
	resp := aggregateEdgesToSegmentEnrichment(edges, 0.20)
	if resp.Surface != nil || resp.SacScale != nil || resp.MtbScale != nil {
		t.Fatalf("expected all-nil on low coverage, got %+v", resp)
	}
	if resp.Reason != "low_coverage" {
		t.Fatalf("Reason expected 'low_coverage', got %q", resp.Reason)
	}
}

func TestAggregateEdgesToSegmentEnrichment_SacMax(t *testing.T) {
	// Majority T2, one T3 — dominant=T2, max=T3.
	edges := []edgeSample{
		{sacID: 2, lengthM: 100}, {sacID: 2, lengthM: 100},
		{sacID: 2, lengthM: 100}, {sacID: 3, lengthM: 100},
	}
	resp := aggregateEdgesToSegmentEnrichment(edges, 1.0)
	if resp.SacScale == nil || resp.SacScale.Dominant != "T2" || resp.SacScale.Max != "T3" {
		t.Fatalf("dominant=T2 max=T3 expected, got %+v", resp.SacScale)
	}
}

// At a true tie (two groups with identical total length) the dominant label
// MUST be empty (spec §4.7) — picking one arbitrarily would depend on map
// iteration order and produce non-deterministic output across runs.
func TestBuildSurfaceSuggestion_TieLeavesDominantEmpty(t *testing.T) {
	totals := map[string]float64{"Asphalt": 100, "Schotter (grob)": 100}
	s := buildSurfaceSuggestion(totals, 200)
	if s.Dominant != "" {
		t.Fatalf("expected empty Dominant at 50/50 tie, got %q", s.Dominant)
	}
	if !s.Mixed {
		t.Fatalf("50/50 tie must be marked Mixed")
	}
}

func TestBuildSurfaceSuggestion_NoTieKeepsDominant(t *testing.T) {
	totals := map[string]float64{"Asphalt": 100, "Schotter (grob)": 50}
	s := buildSurfaceSuggestion(totals, 150)
	if s.Dominant != "Asphalt" {
		t.Fatalf("expected Asphalt, got %q", s.Dominant)
	}
}

// buildScaleSuggestion must also leave dominant empty on a share-tie, but the
// `max` label remains well-defined as the numerically-highest scale-number.
func TestBuildScaleSuggestion_TieLeavesDominantEmptyButMaxIntact(t *testing.T) {
	totals := map[string]float64{"T2": 100, "T4": 100}
	s := buildScaleSuggestion(totals, 200, "T")
	if s.Dominant != "" {
		t.Fatalf("expected empty Dominant at tie, got %q", s.Dominant)
	}
	if s.Max != "T4" {
		t.Fatalf("expected Max=T4 (numerically highest), got %q", s.Max)
	}
}
