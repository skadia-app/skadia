package editor

import (
	"math"
	"strings"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// DetectHRSpikes finds heart rate outliers where consecutive HR values differ
// by more than threshold bpm. Returns timestamps of the spike points.
func DetectHRSpikes(points []StreamPoint, threshold int) []time.Time {
	var spikes []time.Time

	for i := 1; i < len(points)-1; i++ {
		if points[i].HeartRate == nil {
			continue
		}
		cur := int(*points[i].HeartRate)

		// Check spike relative to previous point with HR data
		prevHR := findPrevHR(points, i)
		nextHR := findNextHR(points, i)

		if prevHR == nil || nextHR == nil {
			continue
		}

		diffPrev := abs(cur - *prevHR)
		diffNext := abs(cur - *nextHR)

		// A spike is a point that is far from both neighbors (sudden jump and drop)
		if diffPrev > threshold && diffNext > threshold {
			spikes = append(spikes, points[i].Time)
		}
	}

	return spikes
}

func findPrevHR(points []StreamPoint, idx int) *int {
	for i := idx - 1; i >= 0; i-- {
		if points[i].HeartRate != nil {
			v := int(*points[i].HeartRate)
			return &v
		}
	}
	return nil
}

func findNextHR(points []StreamPoint, idx int) *int {
	for i := idx + 1; i < len(points); i++ {
		if points[i].HeartRate != nil {
			v := int(*points[i].HeartRate)
			return &v
		}
	}
	return nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DetectGPSOutliers findet GPS-Punkte mit unrealistischer Geschwindigkeit.
// maxSpeedMS ist die sport-spezifische Maximalgeschwindigkeit in m/s.
// stddevFactor bestimmt wie viele Standardabweichungen über dem Mittel als Ausreißer gelten.
func DetectGPSOutliers(points []StreamPoint, maxSpeedMS float64, stddevFactor float64) []time.Time {
	if len(points) < 3 {
		return nil
	}

	type speedEntry struct {
		index int
		speed float64
	}
	var speeds []speedEntry

	for i := 1; i < len(points); i++ {
		if points[i-1].Lat == nil || points[i-1].Lng == nil ||
			points[i].Lat == nil || points[i].Lng == nil {
			continue
		}
		deltaS := points[i].Time.Sub(points[i-1].Time).Seconds()
		if deltaS <= 0 {
			continue
		}
		dist := geo.DistanceHaversine(*points[i-1].Lat, *points[i-1].Lng, *points[i].Lat, *points[i].Lng)
		speeds = append(speeds, speedEntry{index: i, speed: dist / deltaS})
	}

	if len(speeds) < 10 {
		return nil
	}

	var sum, sumSq float64
	for _, s := range speeds {
		sum += s.speed
		sumSq += s.speed * s.speed
	}
	mean := sum / float64(len(speeds))
	variance := sumSq/float64(len(speeds)) - mean*mean
	if variance < 0 {
		variance = 0
	}
	stddev := math.Sqrt(variance)
	threshold := mean + stddevFactor*stddev

	outlierSet := make(map[int]bool)
	for _, s := range speeds {
		if s.speed > maxSpeedMS || s.speed > threshold {
			outlierSet[s.index] = true
		}
	}

	var outliers []time.Time
	for idx := range outlierSet {
		outliers = append(outliers, points[idx].Time)
	}
	return outliers
}


// SportMaxSpeed gibt die sport-spezifische Maximalgeschwindigkeit in m/s zurück.
func SportMaxSpeed(sportType string) float64 {
	switch {
	case strings.HasPrefix(sportType, "running"), sportType == "trail_running":
		return 10.0
	case strings.Contains(sportType, "cycling"), strings.Contains(sportType, "biking"):
		return 25.0
	case strings.Contains(sportType, "skiing"):
		return 40.0
	case sportType == "swimming":
		return 3.0
	default:
		return 50.0
	}
}
