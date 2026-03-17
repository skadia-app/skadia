package activityutil

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

const speedGapMaxSeconds = 300.0 // 5 min — don't derive speed across larger gaps

const recordingGapThreshold = 300.0 // 5 min — same as speed gap and CalcMovingTime

// FillMissingDistanceAndSpeed fills nil DistanceM and SpeedMS values by
// deriving them from GPS coordinates and timestamps. Points that already
// have values are preserved — the cumulative distance counter seamlessly
// bridges from real values through GPS-derived gaps and back.
//
// Prerequisites: FixDistanceResets (if applicable) must run BEFORE this.
//
// Algorithm for distance:
//   - Maintain cumDist and prevLat/prevLng.
//   - If point has DistanceM: set cumDist = *DistanceM (adopt real value).
//   - If point has no DistanceM but has Lat/Lng: cumDist += haversine, set DistanceM.
//   - If point has neither: skip (remains nil).
//
// Algorithm for speed:
//   - For each point with nil SpeedMS and both neighbors have GPS:
//     compute speed = haversine(prev, curr) / dt.
//   - Skip if dt <= 0 or gap > 300s (5 min — likely a pause, not travel).
func FillMissingDistanceAndSpeed(points []model.StreamPoint) []model.StreamPoint {
	if len(points) < 2 {
		return points
	}

	// ── Pass 1: Fill distance gaps ──────────────────────────────────────
	var cumDist float64
	var prevLat, prevLng float64
	hasPrev := false

	for i := range points {
		if points[i].DistanceM != nil {
			// Adopt real value as new cumDist baseline.
			// Enforce monotonicity: if GPS-derived cumDist overshot the real
			// value (Haversine overestimates over gaps), take the max to avoid
			// a backwards jump that would break chart rendering.
			if *points[i].DistanceM >= cumDist {
				cumDist = *points[i].DistanceM
			} else {
				// Real value is behind our derived position — adjust upward.
				adjusted := cumDist
				points[i].DistanceM = &adjusted
			}
			if points[i].Lat != nil && points[i].Lng != nil {
				prevLat, prevLng = *points[i].Lat, *points[i].Lng
				hasPrev = true
			}
			continue
		}
		// DistanceM is nil — try to derive from GPS.
		if points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		if hasPrev {
			cumDist += HaversineMeters(prevLat, prevLng, *points[i].Lat, *points[i].Lng)
		}
		dist := cumDist
		points[i].DistanceM = &dist
		prevLat, prevLng = *points[i].Lat, *points[i].Lng
		hasPrev = true
	}

	// ── Pass 2: Fill speed gaps ─────────────────────────────────────────
	for i := 1; i < len(points); i++ {
		if points[i].SpeedMS != nil {
			continue
		}
		if points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		if points[i-1].Lat == nil || points[i-1].Lng == nil {
			continue
		}
		dt := points[i].Time.Sub(points[i-1].Time).Seconds()
		if dt <= 0 || dt > speedGapMaxSeconds {
			continue
		}
		var d float64
		if points[i].DistanceM != nil && points[i-1].DistanceM != nil {
			d = *points[i].DistanceM - *points[i-1].DistanceM
		} else {
			d = HaversineMeters(*points[i-1].Lat, *points[i-1].Lng, *points[i].Lat, *points[i].Lng)
		}
		spd := d / dt
		points[i].SpeedMS = &spd
	}

	return points
}

// FixDistanceResets detects distance counter resets (common in GOTOES-merged
// FIT/TCX files where a second activity starts from 0) and accumulates an
// offset so that the returned distance values are monotonically increasing.
// A reset is detected when distance drops by more than resetThreshold meters.
// Modifies the slice in-place and returns it.
func FixDistanceResets(points []model.StreamPoint) []model.StreamPoint {
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

// CalcRecordingTime returns the total seconds of actual recording, excluding
// gaps longer than 5 minutes. For a normal activity this equals the elapsed
// time. For merged files (multi-source GPX exports and FIT-stitching tools) with multi-hour gaps between
// sessions, it returns only the sum of the individual recording intervals.
func CalcRecordingTime(points []model.StreamPoint) int {
	if len(points) < 2 {
		return 0
	}

	var totalSeconds float64
	for i := 1; i < len(points); i++ {
		dt := points[i].Time.Sub(points[i-1].Time).Seconds()
		if dt <= 0 || dt > recordingGapThreshold {
			continue
		}
		totalSeconds += dt
	}
	return int(totalSeconds)
}

// HaversineMeters returns the great-circle distance in metres between two
// WGS-84 coordinate pairs.
func HaversineMeters(lat1, lng1, lat2, lng2 float64) float64 {
	const R = 6371000.0
	phi1 := lat1 * math.Pi / 180
	phi2 := lat2 * math.Pi / 180
	dPhi := (lat2 - lat1) * math.Pi / 180
	dLam := (lng2 - lng1) * math.Pi / 180
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) +
		math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLam/2)*math.Sin(dLam/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
