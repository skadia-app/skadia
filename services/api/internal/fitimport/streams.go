package fitimport

import (
	"math"

	"github.com/muktihari/fit/profile/mesgdef"

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

		streams = append(streams, sp)
	}

	return fixDistanceResets(streams)
}

// fixDistanceResets detects distance counter resets (common in GOTOES-merged
// FIT files where a second activity starts from 0) and accumulates an offset
// so that the returned distance values are monotonically increasing.
// A reset is detected when distance drops by more than resetThreshold meters.
func fixDistanceResets(points []model.StreamPoint) []model.StreamPoint {
	const resetThreshold = 100.0 // meters
	var prevDist float64
	var offset float64
	for i := range points {
		if points[i].DistanceM == nil {
			continue
		}
		dist := *points[i].DistanceM
		if prevDist > 0 && dist+resetThreshold < prevDist {
			// Distance counter reset detected — accumulate offset
			offset += prevDist
		}
		prevDist = dist
		accumulated := dist + offset
		points[i].DistanceM = &accumulated
	}
	return points
}
