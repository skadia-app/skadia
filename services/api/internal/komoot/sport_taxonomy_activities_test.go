package komoot

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestApplyKomootSportMappingAdoptsSameParentSubtype(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling"}
	mapping := SportMapping{SportType: "cycling", SubType: "mountain"}

	applyKomootSportMapping(data, mapping)

	if data.SportType != "cycling" || data.SubType != "mountain" {
		t.Fatalf("activity tuple = %s/%s, want cycling/mountain", data.SportType, data.SubType)
	}
}

func TestApplyKomootSportMappingKeepsSpecificFileSubtype(t *testing.T) {
	data := &model.ActivityData{SportType: "cycling", SubType: "gravel_cycling"}
	mapping := SportMapping{SportType: "cycling", SubType: "mountain"}

	applyKomootSportMapping(data, mapping)

	if data.SubType != "gravel_cycling" {
		t.Fatalf("SubType = %q, want existing gravel_cycling", data.SubType)
	}
}
