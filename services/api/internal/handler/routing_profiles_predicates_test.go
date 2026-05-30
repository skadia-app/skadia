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

func TestBRouterProfilePredicatesUseOfficialProfileNames(t *testing.T) {
	tests := []struct {
		name     string
		sport    string
		subType  string
		hasMatch bool
	}{
		{name: "fastbike", sport: "cycling", subType: "road", hasMatch: true},
		{name: "mtb", sport: "cycling", subType: "mountain", hasMatch: true},
		{name: "gravel", sport: "cycling", subType: "gravel_cycling", hasMatch: true},
		{name: "all", hasMatch: false},
		{name: "softaccess", hasMatch: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preds := brouterProfilePredicates(tt.name)
			if !tt.hasMatch {
				if len(preds) != 0 {
					t.Fatalf("brouterProfilePredicates(%q) = %#v, want empty", tt.name, preds)
				}
				return
			}
			if len(preds) == 0 {
				t.Fatalf("brouterProfilePredicates(%q) returned no predicates", tt.name)
			}
			found := false
			for _, pred := range preds {
				if pred.Sport == tt.sport && pred.SubType != nil && *pred.SubType == tt.subType {
					found = true
				}
			}
			if !found {
				t.Fatalf("brouterProfilePredicates(%q) = %#v, want %s/%s", tt.name, preds, tt.sport, tt.subType)
			}
		})
	}
}

func TestVisibleBRouterProfileExcludesBuildHelperProfiles(t *testing.T) {
	for _, name := range []string{"all", "softaccess", "cycling-gravel", "cycling-mountain", "cycling-road", "hiking-sac2", "running", "walking", "bad name"} {
		if isVisibleBRouterProfile(name) {
			t.Fatalf("isVisibleBRouterProfile(%q) = true, want false", name)
		}
	}
	for _, name := range []string{"fastbike", "gravel", "hiking-mountain", "mtb", "shortest", "trekking", "custom-trail"} {
		if !isVisibleBRouterProfile(name) {
			t.Fatalf("isVisibleBRouterProfile(%q) = false, want true", name)
		}
	}
}
