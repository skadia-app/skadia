package editor

import (
	"time"
)

// ApplyEdits filters and modifies stream data points based on stored edits.
func ApplyEdits(points []StreamPoint, sidecar *EditSidecar) []StreamPoint {
	if sidecar == nil || len(sidecar.Edits) == 0 {
		return points
	}
	result := points
	for _, edit := range sidecar.Edits {
		switch edit.Type {
		case "trim_start":
			result = applyTrimStart(result, edit)
		case "trim_end":
			result = applyTrimEnd(result, edit)
		case "crop":
			result = applyCrop(result, edit)
		case "hr_spike_smooth":
			result = applyHRSmooth(result, edit)
		}
	}
	return result
}

func applyTrimStart(points []StreamPoint, edit Edit) []StreamPoint {
	if edit.Timestamp == nil {
		return points
	}
	ts, err := time.Parse(time.RFC3339, *edit.Timestamp)
	if err != nil {
		return points
	}
	for i, p := range points {
		if !p.Time.Before(ts) {
			return points[i:]
		}
	}
	return []StreamPoint{}
}

func applyTrimEnd(points []StreamPoint, edit Edit) []StreamPoint {
	if edit.Timestamp == nil {
		return points
	}
	ts, err := time.Parse(time.RFC3339, *edit.Timestamp)
	if err != nil {
		return points
	}
	for i, p := range points {
		if p.Time.After(ts) {
			return points[:i]
		}
	}
	return points
}

func applyCrop(points []StreamPoint, edit Edit) []StreamPoint {
	if edit.FromUTC == nil || edit.ToUTC == nil {
		return points
	}
	from, err := time.Parse(time.RFC3339, *edit.FromUTC)
	if err != nil {
		return points
	}
	to, err := time.Parse(time.RFC3339, *edit.ToUTC)
	if err != nil {
		return points
	}

	var result []StreamPoint
	for _, p := range points {
		if !p.Time.Before(from) && !p.Time.After(to) {
			result = append(result, p)
		}
	}
	return result
}

func applyHRSmooth(points []StreamPoint, edit Edit) []StreamPoint {
	if len(edit.Timestamps) == 0 {
		return points
	}

	// Build a set of spike timestamps for O(1) lookup
	spikeSet := make(map[int64]bool, len(edit.Timestamps))
	for _, tsStr := range edit.Timestamps {
		t, err := time.Parse(time.RFC3339, tsStr)
		if err != nil {
			continue
		}
		spikeSet[t.Unix()] = true
	}

	result := make([]StreamPoint, len(points))
	copy(result, points)

	for i, p := range result {
		if !spikeSet[p.Time.Unix()] {
			continue
		}

		if edit.Method == "remove" {
			hr := int16(0)
			result[i].HeartRate = &hr
			// Use nil to indicate removal
			result[i].HeartRate = nil
			continue
		}

		// Default method: interpolate — average of surrounding valid HR values
		prevHR := findPrevHRInSlice(result, i)
		nextHR := findNextHRInSlice(result, i)

		if prevHR != nil && nextHR != nil {
			interpolated := int16((*prevHR + *nextHR) / 2)
			result[i].HeartRate = &interpolated
		} else if prevHR != nil {
			v := int16(*prevHR)
			result[i].HeartRate = &v
		} else if nextHR != nil {
			v := int16(*nextHR)
			result[i].HeartRate = &v
		} else {
			result[i].HeartRate = nil
		}
	}

	return result
}

func findPrevHRInSlice(points []StreamPoint, idx int) *int16 {
	for i := idx - 1; i >= 0; i-- {
		if points[i].HeartRate != nil {
			return points[i].HeartRate
		}
	}
	return nil
}

func findNextHRInSlice(points []StreamPoint, idx int) *int16 {
	for i := idx + 1; i < len(points); i++ {
		if points[i].HeartRate != nil {
			return points[i].HeartRate
		}
	}
	return nil
}
