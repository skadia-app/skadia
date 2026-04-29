package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestRoutingProfileListItemIncludesComputedPredicates(t *testing.T) {
	profile := database.RoutingProfile{
		ID:          "profile-1",
		Name:        "MTB",
		BaseProfile: "mtb",
		SportTypes:  []string{"mountain_biking"},
		Source:      "system",
	}

	item := profileListItemFromDBProfile(profile, "user-1")
	if len(item.Predicates) != 1 || item.Predicates[0].Sport != "cycling" ||
		item.Predicates[0].SubType == nil || *item.Predicates[0].SubType != "mountain" {
		t.Fatalf("computed predicates = %#v", item.Predicates)
	}
	if item.ModifierFilters.IsElectric != nil {
		t.Fatalf("modifier filters should be empty before DB storage exists: %#v", item.ModifierFilters)
	}
	if len(item.SportTypes) != 1 || item.SportTypes[0] != "cycling" {
		t.Fatalf("compat sport_types = %#v, want canonical [cycling]", item.SportTypes)
	}
}

func TestRoutingProfileListItemJSONIncludesPredicateFields(t *testing.T) {
	item := profileListItem{
		Name:            "MTB",
		Engine:          "badrouter",
		BaseProfile:     "mtb",
		SportTypes:      []string{"cycling"},
		Predicates:      []sport.Predicate{{Sport: "cycling", SubType: strPtr("mountain")}},
		ModifierFilters: sport.ModifierFilters{},
		Source:          "system",
		Editable:        false,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	raw := string(data)
	if !strings.Contains(raw, `"predicates"`) || !strings.Contains(raw, `"modifier_filters"`) || !strings.Contains(raw, `"sport_types"`) {
		t.Fatalf("profile JSON missing compatibility fields: %s", raw)
	}
}

func TestRejectsPredicateStorageFieldsBeforeMigration(t *testing.T) {
	preds := []sport.Predicate{{Sport: "cycling"}}
	filters := sport.ModifierFilters{IsElectric: boolPtr(true)}

	if err := rejectPredicateStorageFieldsBeforeMigration(&preds, nil); err == nil {
		t.Fatal("expected predicates field rejection")
	}
	if err := rejectPredicateStorageFieldsBeforeMigration(nil, &filters); err == nil {
		t.Fatal("expected modifier_filters field rejection")
	}
	if err := rejectPredicateStorageFieldsBeforeMigration(nil, nil); err != nil {
		t.Fatalf("unexpected rejection without predicate storage fields: %v", err)
	}
}

func TestValidateLegacySportTypesForProfile(t *testing.T) {
	got, err := validateLegacySportTypesForProfile([]string{"mountain_biking"})
	if err != nil {
		t.Fatalf("validateLegacySportTypesForProfile() error = %v", err)
	}
	if len(got) != 1 || got[0].Sport != "cycling" || got[0].SubType == nil || *got[0].SubType != "mountain" {
		t.Fatalf("predicates = %#v", got)
	}

	if _, err := validateLegacySportTypesForProfile(nil); err == nil {
		t.Fatal("expected empty sport_types rejection")
	}
}
