package fitimport

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestNormalizeParsedActivitySportDropsFitModeSubType(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling", SubType: "track_me"}
	if err := normalizeParsedActivitySport(data); err != nil {
		t.Fatalf("normalizeParsedActivitySport: %v", err)
	}
	if data.SportType != "cycling" || data.SubType != "" {
		t.Fatalf("data = %#v, want broad cycling", data)
	}
}
