package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/sport"
	"github.com/go-chi/chi/v5"
)

func TestGetSportsTaxonomyUsesTaxonomyOrder(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	r.Get("/api/v1/sports/taxonomy", h.GetSportsTaxonomy)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sports/taxonomy", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var got taxonomyResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantCategories := make([]string, 0)
	for _, category := range sport.AllCategories() {
		wantCategories = append(wantCategories, category.Key)
	}
	if gotCategories := responseCategoryKeys(got.Categories); !reflect.DeepEqual(gotCategories, wantCategories) {
		t.Fatalf("category order = %#v, want %#v", gotCategories, wantCategories)
	}

	runCategory := findCategoryResponse(t, got.Categories, "run")
	wantRunSports := []string{"running", "walking", "hiking", "mountaineering", "wheelchair_push"}
	if gotRunSports := responseSportTypes(runCategory.Sports); !reflect.DeepEqual(gotRunSports, wantRunSports) {
		t.Fatalf("run category sport order = %#v, want %#v", gotRunSports, wantRunSports)
	}

	bikeCategory := findCategoryResponse(t, got.Categories, "bike")
	cycling := findSportResponse(t, bikeCategory.Sports, "cycling")
	wantCyclingSubTypes := []string{"road", "mountain", "downhill", "recumbent", "cyclocross", "hand_cycling", "track_cycling", "indoor_cycling", "spin", "gravel_cycling", "unicycling", "mixed_surface", "bmx"}
	if gotCyclingSubTypes := responseSubTypes(cycling.SubTypes); !reflect.DeepEqual(gotCyclingSubTypes, wantCyclingSubTypes) {
		t.Fatalf("cycling subtype order = %#v, want %#v", gotCyclingSubTypes, wantCyclingSubTypes)
	}
}

func responseCategoryKeys(categories []categoryResponse) []string {
	keys := make([]string, 0, len(categories))
	for _, category := range categories {
		keys = append(keys, category.Key)
	}
	return keys
}

func responseSportTypes(sports []sportResponse) []string {
	types := make([]string, 0, len(sports))
	for _, sport := range sports {
		types = append(types, sport.SportType)
	}
	return types
}

func responseSubTypes(subTypes []subTypeResponse) []string {
	types := make([]string, 0, len(subTypes))
	for _, subType := range subTypes {
		types = append(types, subType.SubType)
	}
	return types
}

func findCategoryResponse(t *testing.T, categories []categoryResponse, key string) categoryResponse {
	t.Helper()
	for _, category := range categories {
		if category.Key == key {
			return category
		}
	}
	t.Fatalf("response missing category %q", key)
	return categoryResponse{}
}

func findSportResponse(t *testing.T, sports []sportResponse, sportType string) sportResponse {
	t.Helper()
	for _, sport := range sports {
		if sport.SportType == sportType {
			return sport
		}
	}
	t.Fatalf("response missing sport %q", sportType)
	return sportResponse{}
}
