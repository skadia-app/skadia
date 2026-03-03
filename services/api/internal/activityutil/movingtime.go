package activityutil

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/model"
	"github.com/skadia-app/skadia/services/api/internal/sport"
)

// maxGapSeconds is the maximum time gap between two consecutive stream points
// that is still counted as a single interval. Gaps larger than this are
// treated as pauses regardless of speed (e.g. GPS signal loss, tunnel, rest).
const maxGapSeconds = 300 // 5 minutes

// movingThreshold returns the minimum speed in m/s that counts as "moving"
// for the given sport type. Delegates to the centralized sport taxonomy.
func movingThreshold(sportType string) float64 {
	return sport.MovingThreshold(sportType)
}

// haversineDistance returns the great-circle distance in metres between two
// WGS-84 coordinates.
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadius * 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))
}

// CalcMovingTime berechnet die Bewegungszeit aus den Stream-Datenpunkten.
//
// Algorithmus:
//  1. Iteriere über aufeinanderfolgende Punktpaare.
//  2. Bestimme die Geschwindigkeit für das Intervall:
//     a. Falls sp.SpeedMS gesetzt ist, verwende diesen Wert direkt.
//     b. Andernfalls berechne v = haversine(prev, curr) / dt aus GPS-Koordinaten.
//  3. Zähle das Intervall nur dann als Bewegungszeit, wenn:
//     a. Die Geschwindigkeit >= sportspezifischer Schwellwert ist, UND
//     b. Die Zeitlücke zwischen den Punkten <= maxGapSeconds beträgt.
//
// Zeitlücken > 5 Minuten gelten als Pause (GPS-Ausfall, Tunnel, echte Pause)
// und werden nicht gezählt, unabhängig von der berechneten Geschwindigkeit.
//
// Gibt 0 zurück, wenn weniger als 2 auswertbare Punkte vorhanden sind.
func CalcMovingTime(streams []model.StreamPoint, sportType string) int {
	if len(streams) < 2 {
		return 0
	}

	threshold := movingThreshold(sportType)
	var movingSeconds int

	// Find the first valid point (non-zero timestamp).
	startIdx := 0
	for startIdx < len(streams) && streams[startIdx].Time.IsZero() {
		startIdx++
	}
	if startIdx >= len(streams)-1 {
		return 0
	}

	prev := streams[startIdx]

	for i := startIdx + 1; i < len(streams); i++ {
		curr := streams[i]
		if curr.Time.IsZero() {
			continue
		}

		dt := curr.Time.Sub(prev.Time).Seconds()
		if dt <= 0 {
			// Same timestamp or backwards — skip.
			prev = curr
			continue
		}

		// Large gap: treat as pause, do not accumulate.
		if dt > maxGapSeconds {
			prev = curr
			continue
		}

		// Determine speed for this interval.
		speed := speedForInterval(prev, curr, dt)

		if speed >= threshold {
			movingSeconds += int(math.Round(dt))
		}

		prev = curr
	}

	return movingSeconds
}

// speedForInterval returns the speed in m/s for the interval between prev and
// curr. It prefers the recorded SpeedMS of the current point (device-measured,
// e.g. from a speed sensor or Doppler GPS). If that is unavailable but both
// points carry GPS coordinates, Haversine displacement divided by dt is used.
// If neither is available, 0 is returned (interval treated as stationary).
func speedForInterval(prev, curr model.StreamPoint, dt float64) float64 {
	// Prefer device-recorded speed at the current point.
	if curr.SpeedMS != nil && *curr.SpeedMS >= 0 {
		return *curr.SpeedMS
	}
	// Fall back to previous point's speed if current is missing.
	if prev.SpeedMS != nil && *prev.SpeedMS >= 0 {
		return *prev.SpeedMS
	}

	// Compute from GPS coordinates if available.
	if prev.Lat != nil && prev.Lng != nil && curr.Lat != nil && curr.Lng != nil {
		dist := haversineDistance(*prev.Lat, *prev.Lng, *curr.Lat, *curr.Lng)
		return dist / dt
	}

	// No speed information available — caller treats as 0.
	return 0.0
}

// HasSpeedData reports whether any stream point carries a measured speed value.
// Used by callers to decide whether to prefer CalcMovingTime over device data.
func HasSpeedData(streams []model.StreamPoint) bool {
	for i := range streams {
		if streams[i].SpeedMS != nil {
			return true
		}
	}
	return false
}

// HasGPSData reports whether any stream point carries GPS coordinates.
func HasGPSData(streams []model.StreamPoint) bool {
	for i := range streams {
		if streams[i].Lat != nil && streams[i].Lng != nil {
			return true
		}
	}
	return false
}

// MinMovingTime returns the smaller of two durations — used to cap the
// stream-derived value against the device-recorded timer time when the device
// value is clearly an upper bound (FIT TotalTimerTime already excludes pauses
// that the device itself detected).
func MinMovingTime(a, b int) int {
	if a < b {
		return a
	}
	return b
}

