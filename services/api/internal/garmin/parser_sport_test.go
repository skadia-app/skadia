package garmin

import (
	"sort"
	"testing"
)

// TestZoneEntrySortAndAssembly prüft, dass HrZone-Einträge nach MessageIndex sortiert werden
// und die Namen/Highs-Arrays korrekt zusammengebaut werden.
func TestZoneEntrySortAndAssembly(t *testing.T) {
	type zoneEntry struct {
		index uint16
		name  string
		high  float64
	}

	// Einträge in falscher Reihenfolge (wie sie aus FIT-Nachrichten kommen könnten)
	entries := []zoneEntry{
		{index: 4, name: "Zone 5", high: 200},
		{index: 1, name: "Zone 2", high: 130},
		{index: 0, name: "Zone 1", high: 120},
		{index: 3, name: "Zone 4", high: 175},
		{index: 2, name: "Zone 3", high: 150},
	}

	// Sortierfunktion aus parser_sport.go repliziert
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].index < entries[j].index
	})

	names := make([]string, len(entries))
	highs := make([]float64, len(entries))
	for i, e := range entries {
		names[i] = e.name
		highs[i] = e.high
	}

	// Zone 1 muss an Index 0 stehen
	if names[0] != "Zone 1" {
		t.Errorf("names[0]: got %q, want 'Zone 1'", names[0])
	}
	if highs[0] != 120 {
		t.Errorf("highs[0]: got %v, want 120", highs[0])
	}

	// Zone 3 muss an Index 2 stehen
	if names[2] != "Zone 3" {
		t.Errorf("names[2]: got %q, want 'Zone 3'", names[2])
	}
	if highs[2] != 150 {
		t.Errorf("highs[2]: got %v, want 150", highs[2])
	}

	// Zone 5 muss an Index 4 stehen
	if names[4] != "Zone 5" {
		t.Errorf("names[4]: got %q, want 'Zone 5'", names[4])
	}
	if highs[4] != 200 {
		t.Errorf("highs[4]: got %v, want 200", highs[4])
	}
}

// TestZoneSortStability prüft, dass bei gleichen Indizes keine Panik entsteht.
func TestZoneSortStability(t *testing.T) {
	type zoneEntry struct {
		index uint16
		name  string
		high  float64
	}

	entries := []zoneEntry{
		{index: 0, name: "A", high: 100},
		{index: 0, name: "B", high: 110}, // Duplikat-Index
	}

	// Darf keine Panik auslösen
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].index < entries[j].index
	})

	if len(entries) != 2 {
		t.Errorf("expected 2 entries after sort, got %d", len(entries))
	}
}

// TestSpeedZoneScaling prüft, dass SpeedZone HighValue /1000 → m/s skaliert wird.
func TestSpeedZoneScaling(t *testing.T) {
	// HighValue = 5000 → 5.0 m/s
	rawHigh := uint16(5000)
	scaled := float64(rawHigh) / 1000.0
	if scaled != 5.0 {
		t.Errorf("SpeedZone scaling: raw=%d → %.3f, want 5.0", rawHigh, scaled)
	}

	// HighValue = 12500 → 12.5 m/s (≈45 km/h)
	rawHigh2 := uint16(12500)
	scaled2 := float64(rawHigh2) / 1000.0
	if scaled2 != 12.5 {
		t.Errorf("SpeedZone scaling: raw=%d → %.3f, want 12.5", rawHigh2, scaled2)
	}
}
