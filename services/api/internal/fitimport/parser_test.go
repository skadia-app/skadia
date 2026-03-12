package fitimport

import (
	"testing"

	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestAllSameSport_True — alle gleiche Sportart
func TestAllSameSport_True(t *testing.T) {
	sports := []typedef.Sport{typedef.SportCycling, typedef.SportCycling}
	if !allSameSport(sports) {
		t.Error("expected true for two cycling sessions")
	}
}

// TestAllSameSport_False — unterschiedliche Sportarten
func TestAllSameSport_False(t *testing.T) {
	sports := []typedef.Sport{typedef.SportCycling, typedef.SportRunning}
	if allSameSport(sports) {
		t.Error("expected false for cycling + running")
	}
}

// TestAllSameSport_Empty — leere Liste → false
func TestAllSameSport_Empty(t *testing.T) {
	if allSameSport(nil) {
		t.Error("expected false for empty slice")
	}
}

// TestAllSameSport_Single — Einzelelement → true
func TestAllSameSport_Single(t *testing.T) {
	sports := []typedef.Sport{typedef.SportRunning}
	if !allSameSport(sports) {
		t.Error("expected true for single element")
	}
}

// TestAllSameSport_TriathlonSports — Triathlon-Mix → false
func TestAllSameSport_TriathlonSports(t *testing.T) {
	sports := []typedef.Sport{
		typedef.SportSwimming,
		typedef.SportTransition,
		typedef.SportCycling,
		typedef.SportTransition,
		typedef.SportRunning,
	}
	if allSameSport(sports) {
		t.Error("expected false for triathlon mix")
	}
}

// TestMergeSessionTotals_TwoSessions summiert Distanz, Zeit, Höhenmeter
// über zwei same-sport Sessions hinweg (GOTOES-Szenario).
func TestMergeSessionTotals_TwoSessions(t *testing.T) {
	// Session 0: 5000m, 1800s elapsed, 1600s timer, 200m ascent, 150m descent
	// Session 1: 3000m, 1200s elapsed, 1000s timer, 100m ascent, 80m descent
	// Expected:  8000m, 3000s elapsed, 2600s timer, 300m ascent, 230m descent

	s0 := &mesgdef.Session{}
	s0.Sport = typedef.SportCycling
	s0.TotalDistance = 500000 // 5000m * 100
	s0.TotalElapsedTime = 1800000
	s0.TotalTimerTime = 1600000
	s0.TotalAscent = 200
	s0.TotalDescent = 150
	s0.MaxHeartRate = 170

	s1 := &mesgdef.Session{}
	s1.Sport = typedef.SportCycling
	s1.TotalDistance = 300000 // 3000m * 100
	s1.TotalElapsedTime = 1200000
	s1.TotalTimerTime = 1000000
	s1.TotalAscent = 100
	s1.TotalDescent = 80
	s1.MaxHeartRate = 185

	msgs := []proto.Message{
		s0.ToMesg(nil),
		s1.ToMesg(nil),
	}

	// ad starts with sessions[0] values (as buildActivityData would set them)
	ad := &model.ActivityData{
		DistanceM:      5000.0,
		ElapsedTimeS:   1800,
		MovingTimeS:    1600,
		ElevationGainM: 200.0,
		ElevationLossM: 150.0,
		MaxHeartRate:   170,
	}

	mergeSessionTotals(ad, msgs)

	if ad.DistanceM != 8000.0 {
		t.Errorf("DistanceM = %f, want 8000.0", ad.DistanceM)
	}
	if ad.ElapsedTimeS != 3000 {
		t.Errorf("ElapsedTimeS = %d, want 3000", ad.ElapsedTimeS)
	}
	if ad.MovingTimeS != 2600 {
		t.Errorf("MovingTimeS = %d, want 2600", ad.MovingTimeS)
	}
	if ad.ElevationGainM != 300.0 {
		t.Errorf("ElevationGainM = %f, want 300.0", ad.ElevationGainM)
	}
	if ad.ElevationLossM != 230.0 {
		t.Errorf("ElevationLossM = %f, want 230.0", ad.ElevationLossM)
	}
	if ad.MaxHeartRate != 185 {
		t.Errorf("MaxHeartRate = %d, want 185 (max of both sessions)", ad.MaxHeartRate)
	}
	// AvgSpeed recalculated: 8000m / 2600s ≈ 3.077 m/s
	wantAvg := 8000.0 / 2600.0
	if ad.AvgSpeedMS < wantAvg-0.01 || ad.AvgSpeedMS > wantAvg+0.01 {
		t.Errorf("AvgSpeedMS = %f, want ~%f", ad.AvgSpeedMS, wantAvg)
	}
}

// TestMergeSessionTotals_SingleSession — keine Änderung bei nur einer Session
func TestMergeSessionTotals_SingleSession(t *testing.T) {
	s0 := &mesgdef.Session{}
	s0.Sport = typedef.SportCycling
	s0.TotalDistance = 500000

	msgs := []proto.Message{s0.ToMesg(nil)}

	ad := &model.ActivityData{DistanceM: 5000.0}
	mergeSessionTotals(ad, msgs)

	if ad.DistanceM != 5000.0 {
		t.Errorf("DistanceM = %f, want 5000.0 (unchanged)", ad.DistanceM)
	}
}
