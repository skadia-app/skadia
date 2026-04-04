package runningform

import (
	"math"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

const minPointsPerQuartile = 60

// Result holds the computed running form metrics.
// Tier indicates which metrics are available:
//
//	1 = DutyFactor + FatigueDuty
//	2 = Tier 1 + VerticalRatio + FatigueVR
//	3 = Tier 2 + GCTAsymmetry
type Result struct {
	Tier         int
	DutyFactor   float64
	VerticalRatio float64
	GCTAsymmetry  float64
	FatigueDuty   float64
	FatigueVR     float64
}

// Compute derives running form metrics from a stream of data points.
// Returns nil when:
//   - fewer than minPointsPerQuartile duty factor samples are available, or
//   - total duration is less than 15 minutes.
func Compute(points []model.StreamPoint) *Result {
	if len(points) < 2 {
		return nil
	}

	// Duration check: at least 15 minutes of data.
	duration := points[len(points)-1].Time.Sub(points[0].Time)
	if duration < 15*time.Minute {
		return nil
	}

	var dutyFactors []float64
	var verticalRatios []float64
	var asymmetries []float64

	for i := range points {
		p := &points[i]

		// Tier 1: duty factor = GCT_ms / stride_duration_ms
		// stride_duration_ms = 60000 / cadence_spm
		if p.GroundContactTimeMs != nil && p.Cadence != nil && *p.Cadence > 0 {
			gctMs := float64(*p.GroundContactTimeMs)
			strideDurationMs := 60000.0 / float64(*p.Cadence)
			dutyFactors = append(dutyFactors, gctMs/strideDurationMs)
		}

		// Tier 2: vertical ratio = VO_mm / step_length_mm
		if p.VerticalOscillationMm != nil && p.StepLengthMm != nil && *p.StepLengthMm > 0 {
			vr := float64(*p.VerticalOscillationMm) / *p.StepLengthMm
			verticalRatios = append(verticalRatios, vr)
		}

		// Tier 3: GCT asymmetry from stance time balance (50% = perfectly symmetric)
		if p.StanceTimeBalancePct != nil {
			asym := math.Abs(*p.StanceTimeBalancePct-50.0) * 2.0
			asymmetries = append(asymmetries, asym)
		}
	}

	// Need at least minPointsPerQuartile duty factor samples to report anything.
	if len(dutyFactors) < minPointsPerQuartile {
		return nil
	}

	res := &Result{
		Tier:        1,
		DutyFactor:  mean(dutyFactors),
		FatigueDuty: fatigueIndex(dutyFactors),
	}

	if len(verticalRatios) >= minPointsPerQuartile {
		res.Tier = 2
		res.VerticalRatio = mean(verticalRatios)
		res.FatigueVR = fatigueIndex(verticalRatios)
	}

	if len(asymmetries) >= minPointsPerQuartile {
		res.Tier = 3
		res.GCTAsymmetry = mean(asymmetries)
	}

	return res
}

// mean returns the arithmetic mean of vals. Caller must ensure len(vals) > 0.
func mean(vals []float64) float64 {
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// fatigueIndex computes the relative change from the first quartile to the
// last quartile of vals.  Returns math.NaN() when there are fewer than
// 4 * minPointsPerQuartile data points (not enough for meaningful quartiles),
// so callers can distinguish "no data" from "zero fatigue".
// A positive result means the metric worsened (increased) toward the end.
func fatigueIndex(vals []float64) float64 {
	n := len(vals)
	if n < 4*minPointsPerQuartile {
		return math.NaN()
	}

	qSize := n / 4
	q1Mean := mean(vals[:qSize])
	q4Mean := mean(vals[n-qSize:])

	if q1Mean == 0 {
		return 0
	}
	return (q4Mean - q1Mean) / q1Mean * 100
}
