package fusion

import (
	"sort"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// Clique ist eine Gruppe von Aktivitäten, die paarweise einen Overlap ≥ threshold haben.
type Clique []string

// pairKey gibt einen deterministischen Schlüssel für ein Paar zurück (sortiert).
func pairKey(a, b string) string {
	if a <= b {
		return a + "\x00" + b
	}
	return b + "\x00" + a
}

// BuildCliques gruppiert Aktivitäten in Cliquen basierend auf paarweisem Overlap.
//
// Algorithmus (Clique-based, NICHT transitiv/Union-Find):
// Jedes Mitglied einer Gruppe muss paarweisen Overlap ≥ threshold mit ALLEN anderen Mitgliedern haben.
//
//  1. Overlap-Lookup-Map aufbauen: sortierter Paar-Key → Overlap-Prozent
//  2. Paare nach Overlap DESC sortieren
//  3. Für jedes Paar über threshold:
//     - Beide bereits in verschiedenen Cliquen → überspringen (keine Cliquen-Verschmelzung)
//     - Eines in Clique → prüfen ob das andere passt (Overlap mit ALLEN Mitgliedern ≥ threshold) → hinzufügen oder überspringen
//     - Keines in Clique → neue Clique {A, B} erstellen
//  4. Liste der Cliquen zurückgeben
func BuildCliques(pairs []model.PairData, threshold float64) []Clique {
	// 1. Overlap-Lookup aufbauen
	overlapMap := make(map[string]float64, len(pairs))
	for _, p := range pairs {
		overlapMap[pairKey(p.ActivityIDA, p.ActivityIDB)] = p.OverlapPercent
	}

	// 2. Paare nach Overlap DESC sortieren
	sorted := make([]model.PairData, len(pairs))
	copy(sorted, pairs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].OverlapPercent > sorted[j].OverlapPercent
	})

	// Zuordnung: Aktivitäts-ID → Clique-Index
	assignment := make(map[string]int)
	var cliques []Clique

	// overlapWith gibt den Overlap zwischen zwei IDs zurück (0 wenn nicht vorhanden).
	overlapWith := func(a, b string) float64 {
		return overlapMap[pairKey(a, b)]
	}

	// fitsInClique prüft ob candidate mit ALLEN Mitgliedern der Clique Overlap ≥ threshold hat.
	fitsInClique := func(candidate string, cliqueIdx int) bool {
		for _, member := range cliques[cliqueIdx] {
			if member == candidate {
				continue
			}
			if overlapWith(candidate, member) < threshold {
				return false
			}
		}
		return true
	}

	// 3. Paare verarbeiten
	for _, p := range sorted {
		if p.OverlapPercent < threshold {
			break // Liste ist sortiert, alle folgenden liegen darunter
		}

		idxA, inA := assignment[p.ActivityIDA]
		idxB, inB := assignment[p.ActivityIDB]

		switch {
		case inA && inB:
			// Beide bereits zugewiesen
			if idxA != idxB {
				// Verschiedene Cliquen → nicht verschmelzen
				continue
			}
			// Gleiche Clique → nichts zu tun

		case inA && !inB:
			// A hat eine Clique, B noch nicht → B hinzufügen wenn es passt
			if fitsInClique(p.ActivityIDB, idxA) {
				cliques[idxA] = append(cliques[idxA], p.ActivityIDB)
				assignment[p.ActivityIDB] = idxA
			}

		case !inA && inB:
			// B hat eine Clique, A noch nicht → A hinzufügen wenn es passt
			if fitsInClique(p.ActivityIDA, idxB) {
				cliques[idxB] = append(cliques[idxB], p.ActivityIDA)
				assignment[p.ActivityIDA] = idxB
			}

		default:
			// Keines in einer Clique → neue Clique erstellen
			newIdx := len(cliques)
			cliques = append(cliques, Clique{p.ActivityIDA, p.ActivityIDB})
			assignment[p.ActivityIDA] = newIdx
			assignment[p.ActivityIDB] = newIdx
		}
	}

	return cliques
}

// BestPrimary gibt die Aktivitäts-ID mit dem höchsten FieldRichness in der Clique zurück.
// Bei Gleichstand gewinnt die erste in der Paarliste.
func BestPrimary(pairs []model.PairData, clique Clique) string {
	if len(clique) == 0 {
		return ""
	}

	// FieldRichness je ID aus den Paaren ermitteln
	richness := make(map[string]int, len(clique))
	for _, p := range pairs {
		if r, exists := richness[p.ActivityIDA]; !exists || p.FieldRichnessA > r {
			richness[p.ActivityIDA] = p.FieldRichnessA
		}
		if r, exists := richness[p.ActivityIDB]; !exists || p.FieldRichnessB > r {
			richness[p.ActivityIDB] = p.FieldRichnessB
		}
	}

	best := clique[0]
	for _, id := range clique[1:] {
		if richness[id] > richness[best] {
			best = id
		}
	}
	return best
}
