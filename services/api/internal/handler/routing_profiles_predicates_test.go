package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestRoutingProfileListItemIncludesComputedPredicates(t *testing.T) {
	trueValue := true
	profile := database.RoutingProfile{
		ID:              "profile-1",
		Name:            "MTB",
		BaseProfile:     "mtb",
		SportTypes:      []string{"cycling"},
		Predicates:      []sport.Predicate{{Sport: "cycling", SubType: strPtr("mountain")}},
		ModifierFilters: sport.ModifierFilters{IsElectric: &trueValue},
		Source:          "system",
	}

	item := profileListItemFromDBProfile(profile, "user-1")
	if len(item.Predicates) != 1 || item.Predicates[0].Sport != "cycling" ||
		item.Predicates[0].SubType == nil || *item.Predicates[0].SubType != "mountain" {
		t.Fatalf("computed predicates = %#v", item.Predicates)
	}
	if item.ModifierFilters.IsElectric == nil || !*item.ModifierFilters.IsElectric {
		t.Fatalf("modifier filters = %#v, want stored is_electric=true", item.ModifierFilters)
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

func TestProfileStorageFieldsPreferPredicatesOverLegacySportTypes(t *testing.T) {
	preds := []sport.Predicate{{Sport: "cycling"}}
	filters := sport.ModifierFilters{IsElectric: boolPtr(true)}

	gotPreds, gotFilters, gotSportTypes, err := normalizeProfileStorageFields(&preds, &filters, []string{"cycling"})
	if err != nil {
		t.Fatalf("normalizeProfileStorageFields() error = %v", err)
	}
	if len(gotPreds) != 1 || gotPreds[0].Sport != "cycling" || gotPreds[0].SubType != nil {
		t.Fatalf("predicates = %#v, want explicit cycling wildcard from predicates field", gotPreds)
	}
	if gotFilters.IsElectric == nil || !*gotFilters.IsElectric {
		t.Fatalf("modifier filters = %#v, want is_electric=true", gotFilters)
	}
	if len(gotSportTypes) != 1 || gotSportTypes[0] != "cycling" {
		t.Fatalf("sport_types = %#v, want canonical [cycling]", gotSportTypes)
	}
}

func TestValidateLegacySportTypesForProfile(t *testing.T) {
	got, err := validateLegacySportTypesForProfile([]string{"cycling"})
	if err != nil {
		t.Fatalf("validateLegacySportTypesForProfile() error = %v", err)
	}
	if len(got) != 1 || got[0].Sport != "cycling" || got[0].SubType != nil {
		t.Fatalf("predicates = %#v", got)
	}

	if _, err := validateLegacySportTypesForProfile(nil); err == nil {
		t.Fatal("expected empty sport_types rejection")
	}
}
