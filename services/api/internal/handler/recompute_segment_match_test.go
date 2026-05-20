package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestSegmentMatchSportTupleForReprocessUsesCurrentActivityTuple(t *testing.T) {
	mountain := "mountain"
	sportType, subType := segmentMatchSportTupleForReprocess(
		&model.ActivityData{SportType: "unknown", SubType: ""},
		&model.Activity{SportType: "cycling", SubType: &mountain},
	)
	if sportType != "cycling" || subType != "mountain" {
		t.Fatalf("match tuple = (%q, %q), want (cycling, mountain)", sportType, subType)
	}
}
