package activityutil

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func TestCalcMovingTimeForTupleUsesSportThreshold(t *testing.T) {
	slow := 0.3
	streams := []model.StreamPoint{
		{Time: time.Unix(0, 0), SpeedMS: &slow},
		{Time: time.Unix(10, 0), SpeedMS: &slow},
	}
	if got := CalcMovingTimeForTuple(streams, "walking", ""); got != 10 {
		t.Fatalf("walking moving time = %d, want 10", got)
	}
	if got := CalcMovingTimeForTuple(streams, "running", ""); got != 0 {
		t.Fatalf("running moving time = %d, want 0", got)
	}
}
