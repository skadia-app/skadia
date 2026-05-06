package enrichment

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestAllGeoEnabled_SetsSummitRadius80(t *testing.T) {
	s := allGeoEnabled()
	if s.SummitDetectionRadiusM != 80.0 {
		t.Fatalf("SummitDetectionRadiusM = %v, want 80.0", s.SummitDetectionRadiusM)
	}
}

func TestGeoSettingsFromModel_CopiesSummitRadius(t *testing.T) {
	m := &model.EnrichmentSettings{SummitDetectionRadiusM: 42.0}
	got := geoSettingsFromModel(m)
	if got.SummitDetectionRadiusM != 42.0 {
		t.Fatalf("got %v, want 42.0", got.SummitDetectionRadiusM)
	}
}

func TestBorderCrossingAdminLevels_IncludesCountryAndRegionalBoundaries(t *testing.T) {
	got := borderCrossingAdminLevels()
	want := []int32{2, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("borderCrossingAdminLevels = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("borderCrossingAdminLevels = %v, want %v", got, want)
		}
	}
}
