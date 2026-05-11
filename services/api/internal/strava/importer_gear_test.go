package strava

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/gear"
)

func TestMapStravaDefaultSportPredicatesPreservesSubtypeAndModifiers(t *testing.T) {
	got := mapStravaDefaultSportPredicates("Ride|Mountain Bike Ride|E-Mountain Bike Ride|Virtual Ride")
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(got), got)
	}
	assertGearPredicate(t, got[0], "cycling", "", false, false)
	assertGearPredicate(t, got[1], "cycling", "mountain", false, false)
	assertGearPredicate(t, got[2], "cycling", "mountain", true, false)
	assertGearPredicate(t, got[3], "cycling", "", false, true)
}

func assertGearPredicate(t *testing.T, got gear.GearDefaultPredicate, sportType, subType string, electric, virtual bool) {
	t.Helper()
	if got.SportType != sportType {
		t.Fatalf("SportType = %q, want %q", got.SportType, sportType)
	}
	gotSubType := ""
	if got.SubType != nil {
		gotSubType = *got.SubType
	}
	if gotSubType != subType {
		t.Fatalf("SubType = %q, want %q", gotSubType, subType)
	}
	gotElectric := false
	gotVirtual := false
	if got.Modifiers != nil {
		if got.Modifiers.IsElectric != nil {
			gotElectric = *got.Modifiers.IsElectric
		}
		if got.Modifiers.IsVirtual != nil {
			gotVirtual = *got.Modifiers.IsVirtual
		}
	}
	if gotElectric != electric || gotVirtual != virtual {
		t.Fatalf("modifiers electric=%v virtual=%v, want electric=%v virtual=%v", gotElectric, gotVirtual, electric, virtual)
	}
}
