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

func TestNormalizeParsedActivitySportHandlesFITProfileV21201Sports(t *testing.T) {
	tests := []struct {
		name      string
		sport     typedef.Sport
		subSport  typedef.SubSport
		wantSport string
		wantSub   string
	}{
		{name: "ccr diving stays ccr diving", sport: typedef.SportDiving, subSport: typedef.SubSportCcrDiving, wantSport: "diving", wantSub: "ccr_diving"},
		{name: "canoeing is canonical", sport: typedef.SportCanoeing, subSport: typedef.SubSportGeneric, wantSport: "canoeing"},
		{name: "pool apnea maps to diving apnea pool", sport: typedef.SportPoolApnea, subSport: typedef.SubSportGeneric, wantSport: "diving", wantSub: "apnea_pool"},
		{name: "winter sport broad bucket stays unknown", sport: typedef.SportWinterSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "water sport broad bucket stays unknown", sport: typedef.SportWaterSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "team sport broad bucket stays unknown", sport: typedef.SportTeamSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "motor sports broad bucket stays unknown", sport: typedef.SportMotorSports, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "para sport broad bucket stays unknown", sport: typedef.SportParaSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "unsupported concrete FIT sport stays unknown", sport: typedef.SportArchery, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "sailing grinder stays unknown until taxonomy decision", sport: typedef.SportGrinding, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &model.ActivityData{
				SportType: SportToString(tt.sport),
				SubType:   SubSportToString(tt.subSport),
			}
			if err := normalizeParsedActivitySport(data); err != nil {
				t.Fatalf("normalizeParsedActivitySport: %v", err)
			}
			if data.SportType != tt.wantSport || data.SubType != tt.wantSub {
				t.Fatalf("tuple = %s/%s, want %s/%s", data.SportType, data.SubType, tt.wantSport, tt.wantSub)
			}
		})
	}
}

func TestNormalizeParsedActivitySportKeepsFITEBikeScopedToCycling(t *testing.T) {
	tests := []struct {
		name      string
		subSport  typedef.SubSport
		wantSport string
		wantSub   string
	}{
		{name: "e-bike mountain", subSport: typedef.SubSportEBikeMountain, wantSport: "cycling", wantSub: "mountain"},
		{name: "e-bike enduro falls back to broad cycling", subSport: typedef.SubSportEBikeEnduro, wantSport: "cycling"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := &model.ActivityData{
				SportType: "cycling",
				SubType:   SubSportToString(tt.subSport),
			}
			if err := normalizeParsedActivitySport(data); err != nil {
				t.Fatalf("normalizeParsedActivitySport: %v", err)
			}
			if data.SportType != tt.wantSport || data.SubType != tt.wantSub {
				t.Fatalf("tuple = %s/%s, want %s/%s", data.SportType, data.SubType, tt.wantSport, tt.wantSub)
			}
			if !data.IsElectric {
				t.Fatalf("IsElectric = false, want true")
			}
		})
	}
}

func TestSubSportToStringUsesFITProfileV21201CcrDiving(t *testing.T) {
	if got := SubSportToString(typedef.SubSportCcrDiving); got != "ccr_diving" {
		t.Fatalf("SubSportToString(SubSportCcrDiving) = %q, want ccr_diving", got)
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
