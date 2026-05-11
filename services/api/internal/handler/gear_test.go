package handler

import (
	"testing"

	gearpkg "github.com/skadia-app/skadia/services/api/internal/gear"
)

func TestNormalizeGearDefaultSportsForStorageAllowsImportedDefaults(t *testing.T) {
	got, err := gearpkg.NormalizeGearDefaultSports([]string{"fitness_equipment"})
	if err != nil {
		t.Fatalf("NormalizeGearDefaultSports() error = %v", err)
	}
	if len(got) != 1 || got[0] != "fitness_equipment" {
		t.Fatalf("normalized sports = %#v, want []string{fitness_equipment}", got)
	}
}
