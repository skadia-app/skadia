package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestRoutingProfileMatchesSuggestionRequest(t *testing.T) {
	profile := database.RoutingProfile{
		Name:       "MTB",
		SportTypes: []string{"mountain_biking"},
	}

	if routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{SportType: "cycling"}) {
		t.Fatal("expected broad cycling request not to match exact MTB profile")
	}
	if !routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{SportType: "cycling", SubType: strPtr("mountain")}) {
		t.Fatal("expected cycling/mountain request to match MTB profile")
	}
	if routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{SportType: "cycling", SubType: strPtr("road")}) {
		t.Fatal("expected cycling/road request not to match MTB profile")
	}
}

func TestRoutingProfileMatchesSuggestionModifiers(t *testing.T) {
	trueValue := true
	falseValue := false
	profile := database.RoutingProfile{
		Name:       "eBike",
		SportTypes: []string{"e_bike"},
	}
	filters := sport.ModifierFilters{IsElectric: &trueValue}

	if !routingProfileMatchesSuggestionRequestWithFilters(profile, RouteSuggestionRequest{
		SportType: "cycling",
		Modifiers: &filters,
	}, sport.ModifierFilters{IsElectric: &trueValue}) {
		t.Fatal("expected matching electric modifier to match")
	}
	if routingProfileMatchesSuggestionRequestWithFilters(profile, RouteSuggestionRequest{
		SportType: "cycling",
		Modifiers: &filters,
	}, sport.ModifierFilters{IsElectric: &falseValue}) {
		t.Fatal("expected nonmatching electric modifier to reject")
	}
}

func TestRoutingProfileMatchesStoredPredicatesAndModifierFilters(t *testing.T) {
	trueValue := true
	falseValue := false
	profile := database.RoutingProfile{
		Name:            "Stored MTB",
		Predicates:      []sport.Predicate{{Sport: "cycling", SubType: strPtr("mountain")}},
		ModifierFilters: sport.ModifierFilters{IsElectric: &trueValue},
	}

	if !routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{
		SportType: "cycling",
		SubType:   strPtr("mountain"),
		Modifiers: &sport.ModifierFilters{IsElectric: &trueValue},
	}) {
		t.Fatal("expected stored cycling/mountain + electric filter to match")
	}
	if routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{
		SportType: "cycling",
		SubType:   strPtr("road"),
		Modifiers: &sport.ModifierFilters{IsElectric: &trueValue},
	}) {
		t.Fatal("expected cycling/road request to reject stored MTB predicate")
	}
	if routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{
		SportType: "cycling",
		Modifiers: &sport.ModifierFilters{IsElectric: &trueValue},
	}) {
		t.Fatal("expected broad cycling request to reject exact stored MTB predicate")
	}
	if routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{
		SportType: "cycling",
		SubType:   strPtr("mountain"),
		Modifiers: &sport.ModifierFilters{IsElectric: &falseValue},
	}) {
		t.Fatal("expected nonmatching stored electric filter to reject")
	}
}

func TestBuiltinProfileMatchesSuggestionRequest(t *testing.T) {
	if !builtinProfileMatchesSuggestionRequest("trekking", RouteSuggestionRequest{SportType: "cycling"}) {
		t.Fatal("expected broad cycling route to match trekking")
	}
	if builtinProfileMatchesSuggestionRequest("trekking", RouteSuggestionRequest{SportType: "cycling", SubType: strPtr("mountain")}) {
		t.Fatal("expected cycling/mountain route not to match trekking")
	}
	if !builtinProfileMatchesSuggestionRequest("mtb", RouteSuggestionRequest{SportType: "cycling", SubType: strPtr("mountain")}) {
		t.Fatal("expected cycling/mountain route to match mtb")
	}
	if builtinProfileMatchesSuggestionRequest("mtb", RouteSuggestionRequest{SportType: "cycling"}) {
		t.Fatal("expected broad cycling route not to match exact mtb")
	}
}
