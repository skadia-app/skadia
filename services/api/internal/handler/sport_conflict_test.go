package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/database"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestSportConflictResolutionChangeIncludesModifiers(t *testing.T) {
	activity := &model.Activity{SportType: "cycling", IsElectric: false, IsIndoor: false, IsVirtual: false}
	resolution := &database.SportConflictResolution{SportType: "cycling", IsElectric: boolPtr(true)}

	if !sportConflictResolutionChangesActivity(activity, resolution) {
		t.Fatal("modifier-only sport conflict resolution should trigger recompute")
	}
}
