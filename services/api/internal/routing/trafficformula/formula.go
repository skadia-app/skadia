// Package trafficformula contains the BRouter traffic classification formula.
// Extracted to avoid import cycles between graph/ and coverage/ packages.
package trafficformula

// Raw computes the BRouter traffic raw score.
// populate: population gravity model × hwWeight
// industrial: industrial/retail/airport proximity
// mwDensity: motorway density in 10km grid
// hwDensity: highway density in 10km grid
// peak_sum_ele = 0 (DEM not available, TODO Stufe 3b).
func Raw(populate, industrial, mwDensity, hwDensity float64) float64 {
	numerator := populate*1200 + industrial // (1 + peak_sum_ele) with peak=0 → factor 1
	denominator := (30 + mwDensity) * (50 + hwDensity)
	if denominator <= 0 {
		return 0
	}
	return numerator / denominator
}

// Classify maps a raw traffic score to BRouter's 6-class system.
// Returns 0 for not classified (raw <= 0), 1-6 for traffic classes.
func Classify(raw float64) uint8 {
	switch {
	case raw <= 0:
		return 0
	case raw < 6:
		return 1
	case raw < 10:
		return 2
	case raw < 19:
		return 3
	case raw < 35:
		return 4
	case raw < 70:
		return 5
	default:
		return 6
	}
}
