package editor

import "time"

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
