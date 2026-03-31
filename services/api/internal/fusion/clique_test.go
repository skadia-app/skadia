package fusion

import (
	"slices"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestBuildCliques_ThreeDevices: 3 vollständig verbundene Geräte → 1 Clique mit 3 Mitgliedern
func TestBuildCliques_ThreeDevices(t *testing.T) {
	pairs := []model.PairData{
		{ActivityIDA: "A", ActivityIDB: "B", OverlapPercent: 95.0},
		{ActivityIDA: "B", ActivityIDB: "C", OverlapPercent: 90.0},
		{ActivityIDA: "A", ActivityIDB: "C", OverlapPercent: 88.0},
	}

	cliques := BuildCliques(pairs, 80.0)

	if len(cliques) != 1 {
		t.Fatalf("expected 1 clique, got %d", len(cliques))
	}
	if len(cliques[0]) != 3 {
		t.Fatalf("expected 3 members in clique, got %d: %v", len(cliques[0]), cliques[0])
	}
	for _, id := range []string{"A", "B", "C"} {
		if !slices.Contains(cliques[0], id) {
			t.Errorf("expected %q in clique, but not found: %v", id, cliques[0])
		}
	}
}

// TestBuildCliques_NoTransitiveChaining: A↔B 90%, B↔C 80%, A↔C 15% → A und C dürfen NICHT in derselben Clique sein
func TestBuildCliques_NoTransitiveChaining(t *testing.T) {
	pairs := []model.PairData{
		{ActivityIDA: "A", ActivityIDB: "B", OverlapPercent: 90.0},
		{ActivityIDA: "B", ActivityIDB: "C", OverlapPercent: 80.0},
		{ActivityIDA: "A", ActivityIDB: "C", OverlapPercent: 15.0},
	}

	cliques := BuildCliques(pairs, 70.0)

	// A und C müssen in unterschiedlichen Cliquen sein (oder C komplett ausgelassen)
	for _, cl := range cliques {
		hasA := slices.Contains(cl, "A")
		hasC := slices.Contains(cl, "C")
		if hasA && hasC {
			t.Errorf("A and C must NOT be in the same clique (no transitive chaining), but got: %v", cl)
		}
	}
}

// TestBuildCliques_SixDevices: 6 vollständig verbundene Geräte → 1 Clique mit 6 Mitgliedern
func TestBuildCliques_SixDevices(t *testing.T) {
	ids := []string{"A", "B", "C", "D", "E", "F"}
	var pairs []model.PairData
	for i := range len(ids) {
		for j := i + 1; j < len(ids); j++ {
			pairs = append(pairs, model.PairData{
				ActivityIDA:    ids[i],
				ActivityIDB:    ids[j],
				OverlapPercent: 85.0,
			})
		}
	}

	cliques := BuildCliques(pairs, 80.0)

	if len(cliques) != 1 {
		t.Fatalf("expected 1 clique, got %d: %v", len(cliques), cliques)
	}
	if len(cliques[0]) != 6 {
		t.Fatalf("expected 6 members in clique, got %d: %v", len(cliques[0]), cliques[0])
	}
	for _, id := range ids {
		if !slices.Contains(cliques[0], id) {
			t.Errorf("expected %q in clique, but not found: %v", id, cliques[0])
		}
	}
}

// TestBestPrimary: gibt die ID mit dem höchsten FieldRichness zurück
func TestBestPrimary(t *testing.T) {
	pairs := []model.PairData{
		{ActivityIDA: "A", ActivityIDB: "B", OverlapPercent: 90.0, FieldRichnessA: 5, FieldRichnessB: 12},
		{ActivityIDA: "B", ActivityIDB: "C", OverlapPercent: 88.0, FieldRichnessA: 12, FieldRichnessB: 3},
		{ActivityIDA: "A", ActivityIDB: "C", OverlapPercent: 85.0, FieldRichnessA: 5, FieldRichnessB: 3},
	}
	clique := Clique{"A", "B", "C"}

	best := BestPrimary(pairs, clique)
	if best != "B" {
		t.Errorf("expected best primary to be %q (richness 12), got %q", "B", best)
	}
}
