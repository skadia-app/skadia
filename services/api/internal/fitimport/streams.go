package fitimport

import (
	"math"
	"strconv"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"

	"github.com/skadia-app/skadia/services/api/internal/activityutil"
	"github.com/skadia-app/skadia/services/api/internal/model"
)

const semicirclesToDegrees = 180.0 / 2147483648.0
const invalidSint32 = math.MaxInt32 // 0x7FFFFFFF

func ExtractStreams(records []*mesgdef.Record) []model.StreamPoint {
	streams := make([]model.StreamPoint, 0, len(records))

	for _, rec := range records {
		if rec.Timestamp.IsZero() {
			continue
		}

		sp := model.StreamPoint{
			Time: rec.Timestamp,
		}

		// GPS coordinates (semicircles → degrees)
		if rec.PositionLat != invalidSint32 && rec.PositionLong != invalidSint32 {
			lat := float64(rec.PositionLat) * semicirclesToDegrees
			lng := float64(rec.PositionLong) * semicirclesToDegrees
			if lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 {
				sp.Lat = &lat
				sp.Lng = &lng
			}
		}

		// Altitude (EnhancedAltitude: uint32, Scale: 5, Offset: 500)
		if rec.EnhancedAltitude != 0xFFFFFFFF && rec.EnhancedAltitude != 0 {
			alt := float64(rec.EnhancedAltitude)/5.0 - 500.0
			sp.AltitudeM = &alt
		} else if rec.Altitude != 0xFFFF {
			alt := float64(rec.Altitude)/5.0 - 500.0
			if alt > -500 && alt < 10000 {
				sp.AltitudeM = &alt
			}
		}

		// Speed (EnhancedSpeed: uint32, Scale: 1000)
		if rec.EnhancedSpeed != 0xFFFFFFFF && rec.EnhancedSpeed != 0 {
			spd := float64(rec.EnhancedSpeed) / 1000.0
			sp.SpeedMS = &spd
		} else if rec.Speed != 0xFFFF {
			spd := float64(rec.Speed) / 1000.0
			sp.SpeedMS = &spd
		}

		// Distance (uint32, Scale: 100)
		if rec.Distance != 0xFFFFFFFF && rec.Distance != 0 {
			dist := float64(rec.Distance) / 100.0
			sp.DistanceM = &dist
		}

		// Heart rate
		if rec.HeartRate != 0xFF && rec.HeartRate != 0 {
			hr := int16(rec.HeartRate)
			sp.HeartRate = &hr
		}

		// Power
		if rec.Power != 0xFFFF && rec.Power != 0 {
			pw := int16(rec.Power)
			sp.PowerW = &pw
		}

		// Cadence
		if rec.Cadence != 0xFF && rec.Cadence != 0 {
			cad := int16(rec.Cadence)
			sp.Cadence = &cad
		}

		// Temperature
		if rec.Temperature != 0x7F {
			temp := int16(rec.Temperature)
			sp.TemperatureC = &temp
		}

		// Running Dynamics — Ground Contact Time (StanceTime: uint16, Scale: 10, Units: ms)
		if rec.StanceTime != 0xFFFF && rec.StanceTime != 0 {
			gct := int16(rec.StanceTimeScaled()) // ms
			sp.GroundContactTimeMs = &gct
		}

		// Running Dynamics — Vertical Oscillation (uint16, Scale: 10, Units: mm)
		if rec.VerticalOscillation != 0xFFFF && rec.VerticalOscillation != 0 {
			vo := int16(rec.VerticalOscillationScaled()) // mm
			sp.VerticalOscillationMm = &vo
		}

		// Running Dynamics — Stance Time Percent (uint16, Scale: 100, Units: %)
		if rec.StanceTimePercent != 0xFFFF && rec.StanceTimePercent != 0 {
			stp := float32(rec.StanceTimePercentScaled()) // %
			sp.StanceTimePercent = &stp
		}

		// Left/Right Balance — mask out the "left" bit (bit 7); value 0–100 = left%
		if uint8(rec.LeftRightBalance) != 0xFF {
			lrb := int16(uint8(rec.LeftRightBalance) & 0x7F)
			sp.LeftRightBalance = &lrb
		}

		// --- FIT SDK — Terrain & Movement ---

		// Grade (int16, Scale: 100 → %)
		if rec.Grade != basetype.Sint16Invalid {
			v := rec.GradeScaled()
			sp.GradePct = &v
		}

		// Vertical Speed (int16, Scale: 1000 → m/s)
		if rec.VerticalSpeed != basetype.Sint16Invalid {
			v := rec.VerticalSpeedScaled()
			sp.VerticalSpeedMs = &v
		}

		// GPS Accuracy (uint8, raw m)
		if rec.GpsAccuracy != basetype.Uint8Invalid {
			v := int16(rec.GpsAccuracy)
			sp.GpsAccuracyM = &v
		}

		// Calories (uint16, raw kcal)
		if rec.Calories != basetype.Uint16Invalid {
			v := int32(rec.Calories)
			sp.Calories = &v
		}

		// Resistance (uint8, raw 0-254)
		if rec.Resistance != basetype.Uint8Invalid {
			v := int16(rec.Resistance)
			sp.Resistance = &v
		}

		// Total Cycles (uint32, raw)
		if rec.TotalCycles != basetype.Uint32Invalid {
			v := int32(rec.TotalCycles)
			sp.TotalCycles = &v
		}

		// Accumulated Power (uint32, raw watts)
		if rec.AccumulatedPower != basetype.Uint32Invalid {
			v := int32(rec.AccumulatedPower)
			sp.AccumulatedPowerW = &v
		}

		// Absolute Pressure (uint32, raw Pa)
		if rec.AbsolutePressure != basetype.Uint32Invalid {
			v := int32(rec.AbsolutePressure)
			sp.AbsolutePressurePa = &v
		}

		// Ascent Rate (int32, Scale: 1000 → m/s)
		if rec.AscentRate != basetype.Sint32Invalid {
			v := rec.AscentRateScaled()
			sp.AscentRateMs = &v
		}

		// Time From Course (int32, Scale: 1000 → s)
		if rec.TimeFromCourse != basetype.Sint32Invalid {
			v := rec.TimeFromCourseScaled()
			sp.TimeFromCourseS = &v
		}

		// Activity Type (enum uint8)
		if rec.ActivityType != typedef.ActivityTypeInvalid {
			v := int16(rec.ActivityType)
			sp.ActivityType = &v
		}

		// Device Index (enum uint8)
		if rec.DeviceIndex != typedef.DeviceIndexInvalid {
			v := int16(rec.DeviceIndex)
			sp.DeviceIndex = &v
		}

		// --- FIT SDK — Running Dynamics (extended) ---

		// Step Length (uint16, Scale: 10 → mm, REAL because fractional)
		if rec.StepLength != basetype.Uint16Invalid {
			v := rec.StepLengthScaled()
			sp.StepLengthMm = &v
		}

		// Vertical Ratio (uint16, Scale: 100 → %)
		if rec.VerticalRatio != basetype.Uint16Invalid {
			v := rec.VerticalRatioScaled()
			sp.VerticalRatioPct = &v
		}

		// Stance Time Balance (uint16, Scale: 100 → %)
		if rec.StanceTimeBalance != basetype.Uint16Invalid {
			v := rec.StanceTimeBalanceScaled()
			sp.StanceTimeBalancePct = &v
		}

		// Cycle Length — prefer CycleLength16 (uint16, Scale: 100 → m) over CycleLength (uint8, Scale: 100 → m)
		if rec.CycleLength16 != basetype.Uint16Invalid {
			v := rec.CycleLength16Scaled()
			sp.CycleLengthM = &v
		} else if rec.CycleLength != basetype.Uint8Invalid {
			v := rec.CycleLengthScaled()
			sp.CycleLengthM = &v
		}

		// Fractional Cadence (uint8, Scale: 128 → rpm)
		if rec.FractionalCadence != basetype.Uint8Invalid {
			v := rec.FractionalCadenceScaled()
			sp.FractionalCadence = &v
		}

		// --- FIT SDK — Pedal Analytics ---

		// Left Torque Effectiveness (uint8, Scale: 2 → %)
		if rec.LeftTorqueEffectiveness != basetype.Uint8Invalid {
			v := rec.LeftTorqueEffectivenessScaled()
			sp.LeftTorqueEffectivenessPct = &v
		}

		// Right Torque Effectiveness (uint8, Scale: 2 → %)
		if rec.RightTorqueEffectiveness != basetype.Uint8Invalid {
			v := rec.RightTorqueEffectivenessScaled()
			sp.RightTorqueEffectivenessPct = &v
		}

		// Left Pedal Smoothness (uint8, Scale: 2 → %)
		if rec.LeftPedalSmoothness != basetype.Uint8Invalid {
			v := rec.LeftPedalSmoothnessScaled()
			sp.LeftPedalSmoothnessPct = &v
		}

		// Right Pedal Smoothness (uint8, Scale: 2 → %)
		if rec.RightPedalSmoothness != basetype.Uint8Invalid {
			v := rec.RightPedalSmoothnessScaled()
			sp.RightPedalSmoothnessPct = &v
		}

		// Combined Pedal Smoothness (uint8, Scale: 2 → %)
		if rec.CombinedPedalSmoothness != basetype.Uint8Invalid {
			v := rec.CombinedPedalSmoothnessScaled()
			sp.CombinedPedalSmoothnessPct = &v
		}

		// Left PCO (int8, raw mm)
		if rec.LeftPco != basetype.Sint8Invalid {
			v := int16(rec.LeftPco)
			sp.LeftPcoMm = &v
		}

		// Right PCO (int8, raw mm)
		if rec.RightPco != basetype.Sint8Invalid {
			v := int16(rec.RightPco)
			sp.RightPcoMm = &v
		}

		// --- FIT SDK — Power Phase (fixed 2-element arrays → start/end columns) ---

		if phases := rec.LeftPowerPhaseScaled(); len(phases) >= 2 {
			if !math.IsNaN(phases[0]) {
				sp.LeftPowerPhaseStartDeg = &phases[0]
			}
			if !math.IsNaN(phases[1]) {
				sp.LeftPowerPhaseEndDeg = &phases[1]
			}
		}

		if phases := rec.LeftPowerPhasePeakScaled(); len(phases) >= 2 {
			if !math.IsNaN(phases[0]) {
				sp.LeftPowerPhasePeakStartDeg = &phases[0]
			}
			if !math.IsNaN(phases[1]) {
				sp.LeftPowerPhasePeakEndDeg = &phases[1]
			}
		}

		if phases := rec.RightPowerPhaseScaled(); len(phases) >= 2 {
			if !math.IsNaN(phases[0]) {
				sp.RightPowerPhaseStartDeg = &phases[0]
			}
			if !math.IsNaN(phases[1]) {
				sp.RightPowerPhaseEndDeg = &phases[1]
			}
		}

		if phases := rec.RightPowerPhasePeakScaled(); len(phases) >= 2 {
			if !math.IsNaN(phases[0]) {
				sp.RightPowerPhasePeakStartDeg = &phases[0]
			}
			if !math.IsNaN(phases[1]) {
				sp.RightPowerPhasePeakEndDeg = &phases[1]
			}
		}

		// --- FIT SDK — Hemoglobin / Muscle Oxygen ---

		// Total Hemoglobin Conc (uint16, Scale: 100 → g/dL)
		if rec.TotalHemoglobinConc != basetype.Uint16Invalid {
			v := rec.TotalHemoglobinConcScaled()
			sp.TotalHemoglobinConc = &v
		}
		if rec.TotalHemoglobinConcMin != basetype.Uint16Invalid {
			v := rec.TotalHemoglobinConcMinScaled()
			sp.TotalHemoglobinConcMin = &v
		}
		if rec.TotalHemoglobinConcMax != basetype.Uint16Invalid {
			v := rec.TotalHemoglobinConcMaxScaled()
			sp.TotalHemoglobinConcMax = &v
		}

		// Saturated Hemoglobin Percent (uint16, Scale: 10 → %)
		if rec.SaturatedHemoglobinPercent != basetype.Uint16Invalid {
			v := rec.SaturatedHemoglobinPercentScaled()
			sp.SaturatedHemoglobinPct = &v
		}
		if rec.SaturatedHemoglobinPercentMin != basetype.Uint16Invalid {
			v := rec.SaturatedHemoglobinPercentMinScaled()
			sp.SaturatedHemoglobinPctMin = &v
		}
		if rec.SaturatedHemoglobinPercentMax != basetype.Uint16Invalid {
			v := rec.SaturatedHemoglobinPercentMaxScaled()
			sp.SaturatedHemoglobinPctMax = &v
		}

		// --- FIT SDK — E-Bike ---

		// Motor Power (uint16, raw watts)
		if rec.MotorPower != basetype.Uint16Invalid {
			v := int32(rec.MotorPower)
			sp.MotorPowerW = &v
		}

		// E-Bike Travel Range (uint16, raw km)
		if rec.EbikeTravelRange != basetype.Uint16Invalid {
			v := int32(rec.EbikeTravelRange)
			sp.EbikeTravelRangeKm = &v
		}

		// E-Bike Battery Level (uint8, raw %)
		if rec.EbikeBatteryLevel != basetype.Uint8Invalid {
			v := int16(rec.EbikeBatteryLevel)
			sp.EbikeBatteryLevelPct = &v
		}

		// E-Bike Assist Mode (uint8, raw)
		if rec.EbikeAssistMode != basetype.Uint8Invalid {
			v := int16(rec.EbikeAssistMode)
			sp.EbikeAssistMode = &v
		}

		// E-Bike Assist Level Percent (uint8, raw %)
		if rec.EbikeAssistLevelPercent != basetype.Uint8Invalid {
			v := int16(rec.EbikeAssistLevelPercent)
			sp.EbikeAssistLevelPct = &v
		}

		// --- FIT SDK — Health / Stress ---

		// Core Temperature (uint16, Scale: 100 → °C)
		if rec.CoreTemperature != basetype.Uint16Invalid {
			v := rec.CoreTemperatureScaled()
			sp.CoreTemperatureC = &v
		}

		// Enhanced Respiration Rate (uint16, Scale: 100 → breaths/min)
		if rec.EnhancedRespirationRate != basetype.Uint16Invalid {
			v := rec.EnhancedRespirationRateScaled()
			sp.EnhancedRespirationRate = &v
		}

		// Current Stress (uint16, Scale: 100)
		if rec.CurrentStress != basetype.Uint16Invalid {
			v := rec.CurrentStressScaled()
			sp.CurrentStress = &v
		}

		// Battery SOC (uint8, Scale: 2 → %)
		if rec.BatterySoc != basetype.Uint8Invalid {
			v := rec.BatterySocScaled()
			sp.BatterySocPct = &v
		}

		// --- FIT SDK — MTB Metrics ---

		// Grit (float32)
		if math.Float32bits(rec.Grit) != basetype.Float32Invalid {
			v := float64(rec.Grit)
			sp.Grit = &v
		}

		// Flow (float32)
		if math.Float32bits(rec.Flow) != basetype.Float32Invalid {
			v := float64(rec.Flow)
			sp.Flow = &v
		}

		// --- FIT SDK — Racket / Ball Sports ---

		// Stroke Type (enum uint8)
		if rec.StrokeType != typedef.StrokeTypeInvalid {
			v := int16(rec.StrokeType)
			sp.StrokeType = &v
		}

		// Zone (uint8, raw)
		if rec.Zone != basetype.Uint8Invalid {
			v := int16(rec.Zone)
			sp.Zone = &v
		}

		// Ball Speed (uint16, Scale: 100 → m/s)
		if rec.BallSpeed != basetype.Uint16Invalid {
			v := rec.BallSpeedScaled()
			sp.BallSpeedMs = &v
		}

		// --- FIT SDK — Diving ---

		// Depth (uint32, Scale: 1000 → m)
		if rec.Depth != basetype.Uint32Invalid {
			v := rec.DepthScaled()
			sp.DepthM = &v
		}

		// Next Stop Depth (uint32, Scale: 1000 → m)
		if rec.NextStopDepth != basetype.Uint32Invalid {
			v := rec.NextStopDepthScaled()
			sp.NextStopDepthM = &v
		}

		// Next Stop Time (uint32, raw s)
		if rec.NextStopTime != basetype.Uint32Invalid {
			v := int32(rec.NextStopTime)
			sp.NextStopTimeS = &v
		}

		// Time To Surface (uint32, raw s)
		if rec.TimeToSurface != basetype.Uint32Invalid {
			v := int32(rec.TimeToSurface)
			sp.TimeToSurfaceS = &v
		}

		// NDL Time (uint32, raw s)
		if rec.NdlTime != basetype.Uint32Invalid {
			v := int32(rec.NdlTime)
			sp.NdlTimeS = &v
		}

		// CNS Load (uint8, raw %)
		if rec.CnsLoad != basetype.Uint8Invalid {
			v := int16(rec.CnsLoad)
			sp.CnsLoadPct = &v
		}

		// N2 Load (uint16, raw %)
		if rec.N2Load != basetype.Uint16Invalid {
			v := int32(rec.N2Load)
			sp.N2LoadPct = &v
		}

		// Air Time Remaining (uint32, raw s)
		if rec.AirTimeRemaining != basetype.Uint32Invalid {
			v := int32(rec.AirTimeRemaining)
			sp.AirTimeRemainingS = &v
		}

		// Pressure SAC (uint16, Scale: 100 → bar/min)
		if rec.PressureSac != basetype.Uint16Invalid {
			v := rec.PressureSacScaled()
			sp.PressureSac = &v
		}

		// Volume SAC (uint16, Scale: 100 → L/min)
		if rec.VolumeSac != basetype.Uint16Invalid {
			v := rec.VolumeSacScaled()
			sp.VolumeSac = &v
		}

		// RMV (uint16, Scale: 100 → L/min)
		if rec.Rmv != basetype.Uint16Invalid {
			v := rec.RmvScaled()
			sp.Rmv = &v
		}

		// PO2 (uint8, Scale: 100 → %)
		if rec.Po2 != basetype.Uint8Invalid {
			v := rec.Po2Scaled()
			sp.Po2 = &v
		}

		// --- FIT SDK — Developer Fields ---
		// TODO: Full FieldDescription lookup will be handled in Task 14.
		// For now, store raw developer field values keyed by field num.
		if len(rec.DeveloperFields) > 0 {
			devFields := make(map[string]any, len(rec.DeveloperFields))
			for _, df := range rec.DeveloperFields {
				key := strconv.Itoa(int(df.Num))
				val := df.Value.Any()
				// Sanitize NaN/Inf — JSONB cannot encode IEEE 754 special values
				switch v := val.(type) {
				case float64:
					if math.IsNaN(v) || math.IsInf(v, 0) {
						continue // skip this field
					}
				case float32:
					if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
						continue
					}
				}
				devFields[key] = val
			}
			if len(devFields) > 0 {
				sp.DeveloperFields = devFields
			}
		}

		// Helper: extract uint8 percentage (0-100) from UnknownField
		extractStaminaPct := func(uf proto.Field) *int16 {
			raw := uf.Value.Any()
			if raw == nil {
				return nil
			}
			switch v := raw.(type) {
			case uint8:
				if v != 0xFF && v <= 100 {
					val := int16(v)
					return &val
				}
			case int8:
				if v >= 0 && v <= 100 {
					val := int16(v)
					return &val
				}
			}
			return nil
		}

		// Garmin undokumentierte Record-Felder (nicht im FIT-Profil)
		for _, uf := range rec.UnknownFields {
			switch uf.Num {
			case 137: // PotentialStamina (0-100%)
				sp.PotentialStamina = extractStaminaPct(uf)
			case 138: // Stamina (0-100%)
				sp.Stamina = extractStaminaPct(uf)
			case 140: // Grade-Adjusted Speed (uint32, scale 1000 → m/s)
				if raw := uf.Value.Any(); raw != nil {
					if v, ok := raw.(uint32); ok && v != 0xFFFFFFFF && v > 0 {
						gap := float64(v) / 1000.0
						sp.GapSpeedMS = &gap
					}
				}
			}
		}

		streams = append(streams, sp)
	}

	return activityutil.FillMissingDistanceAndSpeed(activityutil.FixDistanceResets(streams))
}
