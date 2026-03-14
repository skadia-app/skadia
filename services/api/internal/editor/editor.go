package editor

import (
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

// MetricsFromStreams recalculates summary metrics from a slice of stream points.
// Used after a trim operation to update the activity row without re-parsing the file.
func MetricsFromStreams(points []model.StreamPoint) StreamMetrics {
	if len(points) == 0 {
		return StreamMetrics{}
	}

	first := points[0]
	last := points[len(points)-1]

	elapsed := max(int(last.Time.Sub(first.Time).Seconds()), 0)

	var (
		hrSum     float64
		hrCount   int
		maxHR     int
		pwSum     float64
		pwCount   int
		maxPW     int
		cadSum    float64
		cadCount  int
		tempSum   float64
		tempCount int
		maxSpd    float64
		movingS   int
	)

	prevTime := first.Time
	for _, p := range points {
		dt := p.Time.Sub(prevTime).Seconds()
		prevTime = p.Time

		if p.SpeedMS != nil && *p.SpeedMS > 0.5 {
			movingS += int(dt)
		}
		if p.SpeedMS != nil && *p.SpeedMS > maxSpd {
			maxSpd = *p.SpeedMS
		}
		if p.HeartRate != nil && *p.HeartRate > 0 {
			hrSum += float64(*p.HeartRate)
			hrCount++
			if int(*p.HeartRate) > maxHR {
				maxHR = int(*p.HeartRate)
			}
		}
		if p.PowerW != nil && *p.PowerW > 0 {
			pwSum += float64(*p.PowerW)
			pwCount++
			if int(*p.PowerW) > maxPW {
				maxPW = int(*p.PowerW)
			}
		}
		if p.Cadence != nil && *p.Cadence > 0 {
			cadSum += float64(*p.Cadence)
			cadCount++
		}
		if p.TemperatureC != nil {
			tempSum += float64(*p.TemperatureC)
			tempCount++
		}
	}

	m := StreamMetrics{
		StartedAt:  first.Time,
		EndedAt:    last.Time,
		ElapsedS:   elapsed,
		MovingS:    movingS,
		MaxSpeedMS: maxSpd,
	}
	if elapsed > 0 {
		// avg speed = (elapsed distance implied by last DistanceM - first DistanceM) / elapsed
		if first.DistanceM != nil && last.DistanceM != nil {
			m.DistanceM = *last.DistanceM - *first.DistanceM
			if elapsed > 0 {
				m.AvgSpeedMS = m.DistanceM / float64(elapsed)
			}
		}
	}
	if hrCount > 0 {
		avg := hrSum / float64(hrCount)
		m.AvgHR = &avg
		m.MaxHR = &maxHR
	}
	if pwCount > 0 {
		avg := pwSum / float64(pwCount)
		m.AvgPW = &avg
		m.MaxPW = &maxPW
	}
	if cadCount > 0 {
		avg := cadSum / float64(cadCount)
		m.AvgCad = &avg
	}
	if tempCount > 0 {
		avg := tempSum / float64(tempCount)
		m.AvgTemp = &avg
	}
	return m
}

// StreamMetrics holds recalculated summary values derived from stream data.
type StreamMetrics struct {
	StartedAt  time.Time
	EndedAt    time.Time
	ElapsedS   int
	MovingS    int
	DistanceM  float64
	AvgSpeedMS float64
	MaxSpeedMS float64
	AvgHR      *float64
	MaxHR      *int
	AvgPW      *float64
	MaxPW      *int
	AvgCad     *float64
	AvgTemp    *float64
}

// FilterStreamsByWindow returns the subset of points within [from, to].
func FilterStreamsByWindow(points []model.StreamPoint, from, to time.Time) []model.StreamPoint {
	var result []model.StreamPoint
	for _, p := range points {
		if !p.Time.Before(from) && !p.Time.After(to) {
			result = append(result, p)
		}
	}
	return result
}

// FilterStreamsAfter returns points with time >= cutoff.
func FilterStreamsAfter(points []model.StreamPoint, cutoff time.Time) []model.StreamPoint {
	var result []model.StreamPoint
	for _, p := range points {
		if !p.Time.Before(cutoff) {
			result = append(result, p)
		}
	}
	return result
}

// FilterStreamsBefore returns points with time <= cutoff.
func FilterStreamsBefore(points []model.StreamPoint, cutoff time.Time) []model.StreamPoint {
	var result []model.StreamPoint
	for _, p := range points {
		if !p.Time.After(cutoff) {
			result = append(result, p)
		}
	}
	return result
}
