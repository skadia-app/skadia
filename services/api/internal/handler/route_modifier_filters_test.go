package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
)

func TestRouteModifierFiltersUseGeneratedRouteFilterKeys(t *testing.T) {
	keys := sport.SportModifierKeysWithCapability("route_filter")
	if strings.Join(keys, ",") != "is_electric,is_indoor,is_virtual,is_commute" {
		t.Fatalf("route_filter keys = %v", keys)
	}

	req := httptest.NewRequest("GET", "/routes?is_electric=true&is_indoor=false&is_virtual=true&is_commute=false", nil)
	filters := parseRouteModifierFilters(req)
	assertRouteBoolFilter(t, "is_electric", filters.IsElectric, true)
	assertRouteBoolFilter(t, "is_indoor", filters.IsIndoor, false)
	assertRouteBoolFilter(t, "is_virtual", filters.IsVirtual, true)
	assertRouteBoolFilter(t, "is_commute", filters.IsCommute, false)
}

func assertRouteBoolFilter(t *testing.T, key string, got *bool, want bool) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s filter = nil, want %v", key, want)
	}
	if *got != want {
		t.Fatalf("%s filter = %v, want %v", key, *got, want)
	}
}
