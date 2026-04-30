package importer

import (
	"strings"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestNormalizeImportedActivitySport(t *testing.T) {
	data := &model.ActivityData{SportType: "e_mountain_biking", SubType: "challenge"}
	if err := normalizeImportedActivitySport(data); err != nil {
		t.Fatalf("normalizeImportedActivitySport: %v", err)
	}
	if data.SportType != "cycling" || data.SubType != "mountain" || !data.IsElectric {
		t.Fatalf("data = %#v, want cycling/mountain electric", data)
	}
}

func TestNormalizeImportedActivitySportRejectsEmptySport(t *testing.T) {
	err := normalizeImportedActivitySport(&model.ActivityData{})
	if err == nil || !strings.Contains(err.Error(), "sport_type is required") {
		t.Fatalf("error = %v, want sport_type is required", err)
	}
}
