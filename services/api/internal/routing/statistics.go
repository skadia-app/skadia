package routing

import (
	"math"
	"sort"
)

// zScoreDimCount defines the number of metrics that contribute to diversity scoring.
// Order: Distance, Ascent, AvgFlat, GreenScore, CalmScore.
const zScoreDimCount = 5

// computeZScores normalizes candidate metrics into Z-scores across 5 dimensions.
// Returns:
//   - zs: [candIdx][dimIdx] matrix of Z-scores (all candIdx, all dimIdx populated)
//   - active: [dimIdx]bool — dimension is "active" (stddev > 1e-9 AND at least
//     one candidate has a real (non-imputed) value)
//
// For GreenScore/CalmScore (nullable): nil values are median-imputed from
// the non-nil values in the same dimension before Z-score computation.
// If ALL values in a dimension are nil → active[dim] = false, Z-scores = 0.
func computeZScores(metrics []CandidateMetrics) (zs [][]float64, active []bool) {
	n := len(metrics)
	zs = make([][]float64, n)
	for i := range zs {
		zs[i] = make([]float64, zScoreDimCount)
	}
	active = make([]bool, zScoreDimCount)
	if n == 0 {
		return zs, active
	}

	rawDist := make([]float64, n)
	rawAscent := make([]float64, n)
	rawFlat := make([]float64, n)
	rawGreen := make([]*float64, n)
	rawCalm := make([]*float64, n)
	for i, m := range metrics {
		rawDist[i] = m.DistanceM
		rawAscent[i] = m.AscentM
		rawFlat[i] = float64(m.AvgFlat)
		if m.GreenScore != nil {
			v := float64(*m.GreenScore)
			rawGreen[i] = &v
		}
		if m.CalmScore != nil {
			v := float64(*m.CalmScore)
			rawCalm[i] = &v
		}
	}

	green := imputeNils(rawGreen)
	calm := imputeNils(rawCalm)

	dims := [][]float64{rawDist, rawAscent, rawFlat, green, calm}
	hasRealValues := []bool{
		true, true, true,
		anyNonNil(rawGreen),
		anyNonNil(rawCalm),
	}

	for d, values := range dims {
		if !hasRealValues[d] {
			active[d] = false
			continue
		}
		mean, std := meanAndStd(values)
		if std < 1e-9 {
			active[d] = false
			continue
		}
		active[d] = true
		for i := 0; i < n; i++ {
			zs[i][d] = (values[i] - mean) / std
		}
	}
	return zs, active
}

// euclideanZDist computes Euclidean distance in Z-score space,
// ignoring dimensions where active[d] is false.
func euclideanZDist(a, b []float64, active []bool) float64 {
	var sum float64
	for d := range a {
		if !active[d] {
			continue
		}
		diff := a[d] - b[d]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

// imputeNils replaces nil values with the median of non-nil values.
// If all values are nil, returns a zero slice (caller should check anyNonNil).
func imputeNils(ptrs []*float64) []float64 {
	nonNil := make([]float64, 0, len(ptrs))
	for _, p := range ptrs {
		if p != nil {
			nonNil = append(nonNil, *p)
		}
	}
	out := make([]float64, len(ptrs))
	if len(nonNil) == 0 {
		return out
	}
	median := computeMedian(nonNil)
	for i, p := range ptrs {
		if p == nil {
			out[i] = median
		} else {
			out[i] = *p
		}
	}
	return out
}

func computeMedian(vals []float64) float64 {
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

func meanAndStd(vals []float64) (mean, std float64) {
	n := float64(len(vals))
	if n == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean = sum / n
	var sq float64
	for _, v := range vals {
		d := v - mean
		sq += d * d
	}
	std = math.Sqrt(sq / n)
	return
}

func anyNonNil(ptrs []*float64) bool {
	for _, p := range ptrs {
		if p != nil {
			return true
		}
	}
	return false
}
