package database

import "testing"

func TestDeterminePMCSportGroupCategoryMovesPreserveMovementSemantics(t *testing.T) {
	if got := DeterminePMCSportGroup("snowshoeing"); got != "running" {
		t.Fatalf("snowshoeing PMC group = %q, want running", got)
	}
	if got := DeterminePMCSportGroup("floor_climbing"); got != "running" {
		t.Fatalf("floor_climbing PMC group = %q, want running", got)
	}
}
