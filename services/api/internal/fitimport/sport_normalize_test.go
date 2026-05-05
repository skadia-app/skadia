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
		{name: "dynamic apnea stays dynamic apnea", sport: typedef.SportDiving, subSport: typedef.SubSportDynamicApnea, wantSport: "diving", wantSub: "dynamic_apnea"},
		{name: "canoeing is canonical", sport: typedef.SportCanoeing, subSport: typedef.SubSportGeneric, wantSport: "canoeing"},
		{name: "pool apnea maps to diving apnea pool", sport: typedef.SportPoolApnea, subSport: typedef.SubSportGeneric, wantSport: "diving", wantSub: "apnea_pool"},
		{name: "mobility is canonical", sport: typedef.SportMobility, subSport: typedef.SubSportGeneric, wantSport: "mobility"},
		{name: "archery is canonical", sport: typedef.SportArchery, subSport: typedef.SubSportGeneric, wantSport: "archery"},
		{name: "shooting is canonical", sport: typedef.SportShooting, subSport: typedef.SubSportGeneric, wantSport: "shooting"},
		{name: "geocaching is canonical", sport: typedef.SportGeocaching, subSport: typedef.SubSportGeneric, wantSport: "geocaching"},
		{name: "rucking maps to hiking rucking", sport: typedef.SportHiking, subSport: typedef.SubSportRucking, wantSport: "hiking", wantSub: "rucking"},
		{name: "winter sport broad bucket stays unknown", sport: typedef.SportWinterSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "water sport broad bucket stays unknown", sport: typedef.SportWaterSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "team sport broad bucket stays unknown", sport: typedef.SportTeamSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "motor sports broad bucket stays unknown", sport: typedef.SportMotorSports, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
		{name: "para sport broad bucket stays unknown", sport: typedef.SportParaSport, subSport: typedef.SubSportGeneric, wantSport: "unknown"},
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
		{name: "enduro is mountain biking", subSport: typedef.SubSportEnduro, wantSport: "cycling", wantSub: "mountain"},
		{name: "e-bike enduro is electric mountain biking", subSport: typedef.SubSportEBikeEnduro, wantSport: "cycling", wantSub: "mountain"},
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
			wantElectric := tt.subSport == typedef.SubSportEBikeMountain || tt.subSport == typedef.SubSportEBikeEnduro
			if data.IsElectric != wantElectric {
				t.Fatalf("IsElectric = %v, want %v", data.IsElectric, wantElectric)
			}
		})
	}
}

func TestSubSportToStringUsesFITProfileV21201CcrDiving(t *testing.T) {
	if got := SubSportToString(typedef.SubSportCcrDiving); got != "ccr_diving" {
		t.Fatalf("SubSportToString(SubSportCcrDiving) = %q, want ccr_diving", got)
	}
}

func TestApplyDiveGasDerivedSportNormalizesCCRSubType(t *testing.T) {
	data := &model.ActivityData{
		SportType:  "diving",
		SubType:    "generic",
		IsElectric: true,
		DiveGases: []model.DiveGas{
			{Mode: "open_circuit"},
			{Mode: "closed_circuit_diluent"},
		},
	}

	if err := applyDiveGasDerivedSport(data); err != nil {
		t.Fatalf("applyDiveGasDerivedSport: %v", err)
	}
	if data.SportType != "diving" || data.SubType != "ccr_diving" {
		t.Fatalf("tuple = %s/%s, want diving/ccr_diving", data.SportType, data.SubType)
	}
	if !data.IsElectric {
		t.Fatalf("IsElectric = false, want existing parser evidence preserved")
	}
}

func TestApplyDiveGasDerivedSportClearsStalePoolIndoorDefault(t *testing.T) {
	data := &model.ActivityData{
		SportType: "diving",
		SubType:   "apnea_pool",
		IsIndoor:  true,
		DiveGases: []model.DiveGas{
			{Mode: "closed_circuit_diluent"},
		},
	}

	if err := applyDiveGasDerivedSport(data); err != nil {
		t.Fatalf("applyDiveGasDerivedSport: %v", err)
	}
	if data.SportType != "diving" || data.SubType != "ccr_diving" {
		t.Fatalf("tuple = %s/%s, want diving/ccr_diving", data.SportType, data.SubType)
	}
	if data.IsIndoor {
		t.Fatalf("IsIndoor = true, want false for CCR diving without independent indoor evidence")
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
	if data.SportType != "wheelchair_push" || data.SubType != "run_speed" {
		t.Fatalf("tuple = %s/%s, want wheelchair_push/run_speed", data.SportType, data.SubType)
	}
	streamMoving := activityutil.CalcMovingTimeForTuple(data.Streams, data.SportType, data.SubType)
	if streamMoving != 0 {
		t.Fatalf("stream moving time = %d, want 0 with wheelchair run-speed threshold", streamMoving)
	}
}
