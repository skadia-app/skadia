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
