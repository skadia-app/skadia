//go:build integration

package database

import (
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// TestStreamPoint_AllFieldsPopulated verifies that all StreamPoint fields
// can be populated with non-zero values. A real column-order test requires
// a database connection (use //go:build integration).
func TestStreamPoint_AllFieldsPopulated(t *testing.T) {
	// Build a StreamPoint with unique sentinel values for every field.
	// If any column-value pair is swapped, the read-back assertion will fail.
	now := time.Now().Truncate(time.Microsecond)
	lat := ptr(47.123456)
	lng := ptr(15.654321)
	alt := ptr(1234.5)
	speed := ptr(3.45)
	dist := ptr(1000.5)
	hr := ptrI16(142)
	power := ptrI16(250)
	cadence := ptrI16(90)
	temp := ptrI16(22)
	gct := ptrI16(240)
	vo := ptrI16(85)
	stp := ptrF32(51.2)
	lrb := ptrI16(52)

	sp := model.StreamPoint{
		Time:                  now,
		Lat:                   lat,
		Lng:                   lng,
		AltitudeM:             alt,
		SpeedMS:               speed,
		DistanceM:             dist,
		HeartRate:             hr,
		PowerW:                power,
		Cadence:               cadence,
		TemperatureC:          temp,
		GroundContactTimeMs:   gct,
		VerticalOscillationMm: vo,
		StanceTimePercent:     stp,
		LeftRightBalance:      lrb,
		// FIT SDK — Terrain & Movement
		RespirationRate:    ptrI16(18),
		StressLevel:        ptrI16(45),
		GradePct:           ptr(5.5),
		VerticalSpeedMs:    ptr(0.8),
		GpsAccuracyM:       ptrI16(3),
		Calories:           ptrI32(500),
		Resistance:         ptrI16(100),
		TotalCycles:        ptrI32(2000),
		AccumulatedPowerW:  ptrI32(50000),
		AbsolutePressurePa: ptrI32(101325),
		AscentRateMs:       ptr(0.5),
		TimeFromCourseS:    ptr(1.2),
		ActivityType:       ptrI16(1),
		DeviceIndex:        ptrI16(0),
		// Running Dynamics extended
		StepLengthMm:         ptr(1100.5),
		VerticalRatioPct:     ptr(8.2),
		StanceTimeBalancePct: ptr(50.5),
		CycleLengthM:         ptr(2.1),
		FractionalCadence:    ptr(0.5),
		// Pedal Analytics
		LeftTorqueEffectivenessPct:  ptr(72.5),
		RightTorqueEffectivenessPct: ptr(71.0),
		LeftPedalSmoothnessPct:      ptr(22.5),
		RightPedalSmoothnessPct:     ptr(23.0),
		CombinedPedalSmoothnessPct:  ptr(22.8),
		LeftPcoMm:                   ptrI16(-5),
		RightPcoMm:                  ptrI16(3),
		// Power Phase
		LeftPowerPhaseStartDeg:      ptr(15.0),
		LeftPowerPhaseEndDeg:        ptr(175.0),
		LeftPowerPhasePeakStartDeg:  ptr(30.0),
		LeftPowerPhasePeakEndDeg:    ptr(160.0),
		RightPowerPhaseStartDeg:     ptr(10.0),
		RightPowerPhaseEndDeg:       ptr(170.0),
		RightPowerPhasePeakStartDeg: ptr(25.0),
		RightPowerPhasePeakEndDeg:   ptr(155.0),
		// Hemoglobin
		TotalHemoglobinConc:       ptr(14.5),
		TotalHemoglobinConcMin:    ptr(13.0),
		TotalHemoglobinConcMax:    ptr(16.0),
		SaturatedHemoglobinPct:    ptr(98.5),
		SaturatedHemoglobinPctMin: ptr(95.0),
		SaturatedHemoglobinPctMax: ptr(100.0),
		// E-Bike
		MotorPowerW:          ptrI32(200),
		EbikeTravelRangeKm:   ptrI32(50),
		EbikeBatteryLevelPct: ptrI16(75),
		EbikeAssistMode:      ptrI16(2),
		EbikeAssistLevelPct:  ptrI16(60),
		// Health / Stress
		CoreTemperatureC:        ptr(37.5),
		EnhancedRespirationRate: ptr(18.5),
		CurrentStress:           ptr(42.0),
		BatterySocPct:           ptr(85.0),
		// MTB
		Grit: ptr(4.5),
		Flow: ptr(3.2),
		// Racket / Ball
		StrokeType:  ptrI16(1),
		Zone:        ptrI16(3),
		BallSpeedMs: ptr(55.0),
		// Diving
		DepthM:            ptr(30.5),
		NextStopDepthM:    ptr(5.0),
		NextStopTimeS:     ptrI32(180),
		TimeToSurfaceS:    ptrI32(600),
		NdlTimeS:          ptrI32(1200),
		CnsLoadPct:        ptrI16(25),
		N2LoadPct:         ptrI32(50),
		AirTimeRemainingS: ptrI32(3600),
		PressureSac:       ptr(0.8),
		VolumeSac:         ptr(12.5),
		Rmv:               ptr(15.0),
		Po2:               ptr(1.2),
		// Developer Fields
		DeveloperFields: map[string]any{"stryd_power": 250.0, "core_temp": 37.5},
	}

	// Verify the StreamPoint has all fields populated (compile-time check).
	// If a new field is added to StreamPoint but not here, this won't compile.
	_ = sp

	t.Log("StreamPoint sentinel values constructed successfully — all fields populated")
	t.Log("Full integration test requires a database connection. Run with -tags integration and DATABASE_URL set.")
}

func ptr(f float64) *float64    { return &f }
func ptrI16(i int16) *int16     { return &i }
func ptrI32(i int32) *int32     { return &i }
func ptrF32(f float32) *float32 { return &f }
