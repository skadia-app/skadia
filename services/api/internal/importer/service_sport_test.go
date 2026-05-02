package importer

import (
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestNormalizeImportedActivitySport(t *testing.T) {
	data := &model.ActivityData{SportType: "e_mountain_biking", SubType: "challenge"}
	if err := normalizeImportedActivitySport(data); err != nil {
		t.Fatalf("normalizeImportedActivitySport: %v", err)
	}
	if data.SportType != "cycling" || data.SubType != "mountain" || !data.IsElectric {
		t.Fatalf("data = %#v, want cycling/mountain electric", data)
	}
}

func TestNormalizeImportedActivitySportDropsDeviceSubType(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling", SubType: "commuting"}
	if err := normalizeImportedActivitySport(data); err != nil {
		t.Fatalf("normalizeImportedActivitySport: %v", err)
	}
	if data.SportType != "cycling" || data.SubType != "" {
		t.Fatalf("data = %#v, want broad cycling", data)
	}
}

func TestResolverVirtualPatchAppliesThroughSharedHelper(t *testing.T) {
	resolved, err := sport.ResolveSport(sport.ResolveInput{Source: sport.SourceStrava, RawSport: "Virtual Ride"})
	if err != nil {
		t.Fatalf("ResolveSport: %v", err)
	}
	if !resolved.Resolved || !resolved.AliasMatched {
		t.Fatalf("resolved = %#v, want Strava virtual alias", resolved)
	}

	values := sport.ApplyModifierPatch(sport.ModifierValues{}, resolved.Patch)
	if !values.IsVirtual {
		t.Fatalf("IsVirtual = false, want resolver patch to apply")
	}
}

func TestSharedModifierPatchAppliesExplicitIndoorFalse(t *testing.T) {
	falseValue := false
	values := sport.ApplyModifierPatch(
		sport.ModifierValues{IsIndoor: true},
		sport.ModifierPatch{IsIndoor: &falseValue},
	)
	if values.IsIndoor {
		t.Fatalf("IsIndoor = true, want explicit false patch to win")
	}
}

func TestNormalizeImportedActivitySportRejectsEmptySport(t *testing.T) {
	err := normalizeImportedActivitySport(&model.ActivityData{})
	if err == nil || !strings.Contains(err.Error(), "sport_type is required") {
		t.Fatalf("error = %v, want sport_type is required", err)
	}
}
