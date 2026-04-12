// Package routing — monotone WayID-based soft-penalty primitive.
//
// Used by the round-trip quality protocol to bias leg 2 away from leg 1's
// edges while keeping ALT active. Penalties must be >= 1.0 so that the
// real cost of every edge is monotonically non-decreasing, which preserves
// the empirical admissibility of landmark lower bounds.
package routing

// lookupWayPenalty returns the penalty factor for a given edge, or 1.0 if no
// penalty applies. Fast path: returns 1.0 immediately if penalties is nil or
// empty, without invoking reader.EdgeOSMID (which is a tile lookup).
func lookupWayPenalty(handle uint64, reader GraphReader,
	penalties map[int64]float32) float32 {
	if len(penalties) == 0 {
		return 1.0
	}
	osmID := reader.EdgeOSMID(handle)
	if osmID == 0 {
		return 1.0
	}
	if p, ok := penalties[osmID]; ok {
		return p
	}
	return 1.0
}

// applyPenaltyToECF scales all cost components of an EdgeCostFactors by the
// penalty. Assumes penalty >= 1.0 (monotonicity invariant).
func applyPenaltyToECF(ecf *EdgeCostFactors, penalty float32) {
	if penalty <= 1.0 {
		if penalty < 1.0 {
			panic("applyPenaltyToECF: penalty must be >= 1.0 (monotonicity violated)")
		}
		return
	}
	ecf.Flat *= penalty
	if ecf.Up > 0 {
		ecf.Up *= penalty
	}
	if ecf.Down > 0 {
		ecf.Down *= penalty
	}
}
