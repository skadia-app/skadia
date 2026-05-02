package fitimport

import (
	"testing"
	"time"

	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
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

func TestNormalizeParsedActivitySportPreservesFitParserEvidence(t *testing.T) {
	data := &model.ActivityData{
		SportType:  "cycling",
		SubType:    "commuting",
		IsElectric: true,
		IsIndoor:   true,
		IsVirtual:  true,
	}
	if err := normalizeParsedActivitySport(data); err != nil {
		t.Fatalf("normalizeParsedActivitySport: %v", err)
	}
	if data.SportType != "cycling" || data.SubType != "" {
		t.Fatalf("tuple = %s/%s, want broad cycling", data.SportType, data.SubType)
	}
	if !data.IsElectric || !data.IsIndoor || !data.IsVirtual {
		t.Fatalf("parser flags = electric:%v indoor:%v virtual:%v, want all true", data.IsElectric, data.IsIndoor, data.IsVirtual)
	}
}

func TestIndoorFITSubSportEvidence(t *testing.T) {
	if !isIndoorFITSubSport(int16(typedef.SubSportIndoorCycling)) {
		t.Fatalf("SubSportIndoorCycling should be indoor")
	}
	if isIndoorFITSubSport(int16(typedef.SubSportGravelCycling)) {
		t.Fatalf("SubSportGravelCycling should not be indoor")
	}
}

func TestNormalizeParsedActivitySportBeforeStreamMovingTimeKeepsWheelchairRunThreshold(t *testing.T) {
	speed := 0.3
	data := &model.ActivityData{
		SportType:   "wheelchair_push_run",
		MovingTimeS: 20,
		Streams: []model.StreamPoint{
			{Time: time.Unix(0, 0), SpeedMS: &speed},
			{Time: time.Unix(10, 0), SpeedMS: &speed},
		},
	}
	if err := normalizeParsedActivitySport(data); err != nil {
		t.Fatalf("normalizeParsedActivitySport: %v", err)
	}
	streamMoving := activityutil.CalcMovingTimeForTuple(data.Streams, data.SportType, data.SubType)
	if streamMoving != 0 {
		t.Fatalf("stream moving time = %d, want 0 with wheelchair run-speed threshold", streamMoving)
	}
}
