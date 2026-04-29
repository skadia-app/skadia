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

	if !routingProfileMatchesSuggestionRequest(profile, RouteSuggestionRequest{SportType: "cycling"}) {
		t.Fatal("expected sport-only cycling request to match MTB profile for backward compatibility")
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
