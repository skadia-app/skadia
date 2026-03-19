package routing

// PathState holds the elevation hysteresis buffers.
// 8 bytes, inline copied (too small for sync.Pool).
type PathState struct {
	Ehbu int32 // uphill buffer in millimeters
	Ehbd int32 // downhill buffer in millimeters
}

// HysteresisResult holds the output of a hysteresis propagation step.
type HysteresisResult struct {
	State      PathState
	ElevCost   float32
	UpWeight   float32 // 0-1: fraction of edge that is uphill-penalized
	DownWeight float32 // 0-1: fraction of edge that is downhill-penalized
}

// propagateHysteresis updates hysteresis buffers and returns elevation cost.
// deltaEleMilli: elevation change in millimeters (positive = uphill).
// distMilli: edge distance in millimeters.
// Unit choice: millimeters (not micrometers) to avoid int32 overflow.
// Max EdgeDist 65535 dm * 100 = 6,553,500 mm — fits int32.
func propagateHysteresis(parent PathState, deltaEleMilli int32,
	distMilli int32, cfg *CostConfig) HysteresisResult {

	state := parent

	// Accumulate buffers
	state.Ehbd += -deltaEleMilli - int32(float32(distMilli)*cfg.DownhillCutoff)
	state.Ehbu += deltaEleMilli - int32(float32(distMilli)*cfg.UphillCutoff)

	// Clamp to [0, ElevMaxBuffer]
	if state.Ehbd < 0 {
		state.Ehbd = 0
	}
	if state.Ehbu < 0 {
		state.Ehbu = 0
	}
	maxBuf := int32(cfg.ElevMaxBuffer * 1e3)
	if state.Ehbd > maxBuf {
		state.Ehbd = maxBuf
	}
	if state.Ehbu > maxBuf {
		state.Ehbu = maxBuf
	}

	// Cost activation with ElevBufferReduce support
	var elevCost float32
	var upWeight, downWeight float32
	penBuf := int32(cfg.ElevPenaltyBuffer * 1e3)

	if state.Ehbd > penBuf {
		excess := float32(state.Ehbd - penBuf)
		reduce, w := computeReduce(excess, float32(distMilli), cfg.ElevBufferReduce, float32(state.Ehbd-maxBuf))
		downWeight = w
		elevCost += (reduce / 1e3) * cfg.DownhillCost
		state.Ehbd -= int32(reduce)
		// Re-clamp after reduce (should not go below penBuf, but safety)
		if state.Ehbd < penBuf {
			state.Ehbd = penBuf
		}
	}
	if state.Ehbu > penBuf {
		excess := float32(state.Ehbu - penBuf)
		reduce, w := computeReduce(excess, float32(distMilli), cfg.ElevBufferReduce, float32(state.Ehbu-maxBuf))
		upWeight = w
		elevCost += (reduce / 1e3) * cfg.UphillCost
		state.Ehbu -= int32(reduce)
		// Re-clamp after reduce
		if state.Ehbu < penBuf {
			state.Ehbu = penBuf
		}
	}

	return HysteresisResult{
		State:      state,
		ElevCost:   elevCost,
		UpWeight:   upWeight,
		DownWeight: downWeight,
	}
}

// computeReduce implements the BRouter StdPath ElevBufferReduce pattern.
// excess: buffer above PenaltyBuffer in mm.
// distMilli: edge distance in mm.
// elevBufferReduce: decay rate (mm drain per mm distance). 0 = legacy mode.
// overMax: buffer above MaxBuffer in mm (negative means within bounds).
// Returns (reduce_mm, weight). reduce is clamped so buffer stays at or below MaxBuffer.
func computeReduce(excess, distMilli, elevBufferReduce, overMax float32) (float32, float32) {
	if elevBufferReduce == 0 {
		// Legacy mode: no decay — excess is reduced fully (weight = 1.0)
		return excess, 1.0
	}

	reduce := distMilli * elevBufferReduce
	var weight float32

	if reduce > excess {
		// Partial activation: only drain what's available
		weight = excess / reduce
		reduce = excess
	} else {
		weight = 1.0
	}

	// Clamp: reduce must not be less than overMax (buffer must not stay above MaxBuffer)
	if overMax > 0 && reduce < overMax {
		reduce = overMax
	}

	return reduce, weight
}

// propagateHysteresisProfile processes elevation changes along an edge's
// elevation profile. Each segment between consecutive profile points is
// processed individually through the hysteresis buffer.
// elevProfile: elevation in decimeters at each geometry point (int32 for >3276.7m support)
// segDistMilli: distance in millimeters for each segment (len = len(elevProfile)-1)
func propagateHysteresisProfile(parent PathState, elevProfile []int32,
	segDistMilli []int32, cfg *CostConfig) HysteresisResult {

	if len(elevProfile) < 2 {
		return HysteresisResult{State: parent}
	}

	state := parent
	var totalElevCost float32
	var totalUpW, totalDownW, totalDist float32

	for i := range len(elevProfile) - 1 {
		deltaEleMilli := (elevProfile[i+1] - elevProfile[i]) * 100 // dm → mm
		segDist := float32(segDistMilli[i])

		res := propagateHysteresis(state, deltaEleMilli, segDistMilli[i], cfg)
		state = res.State
		totalElevCost += res.ElevCost

		// Distance-weighted average for UpWeight/DownWeight
		totalUpW += res.UpWeight * segDist
		totalDownW += res.DownWeight * segDist
		totalDist += segDist
	}

	var upWeight, downWeight float32
	if totalDist > 0 {
		upWeight = totalUpW / totalDist
		downWeight = totalDownW / totalDist
	}

	return HysteresisResult{
		State:      state,
		ElevCost:   totalElevCost,
		UpWeight:   upWeight,
		DownWeight: downWeight,
	}
}

// propagateHysteresisProfileCm processes a full elevation profile with
// pre-computed segment distances in centimeters (uint16).
// Converts cm→mm inline per segment to avoid allocating a temporary []int32 slice
// (~800k allocations eliminated on a 50km route).
func propagateHysteresisProfileCm(parent PathState, elevsDm []int32,
	segDistsCm []uint16, cfg *CostConfig) HysteresisResult {

	if len(elevsDm) <= 1 || len(segDistsCm) == 0 {
		return HysteresisResult{State: parent}
	}

	state := parent
	var totalElevCost float32
	var totalUpW, totalDownW, totalDist float32

	for i := range len(elevsDm) - 1 {
		segDistMilli := int32(segDistsCm[i]) * 10              // cm → mm, inline, no alloc
		deltaEleMilli := (elevsDm[i+1] - elevsDm[i]) * 100    // dm → mm
		segDistF := float32(segDistMilli)

		res := propagateHysteresis(state, deltaEleMilli, segDistMilli, cfg)
		state = res.State
		totalElevCost += res.ElevCost

		// Distance-weighted average for UpWeight/DownWeight
		totalUpW += res.UpWeight * segDistF
		totalDownW += res.DownWeight * segDistF
		totalDist += segDistF
	}

	var upWeight, downWeight float32
	if totalDist > 0 {
		upWeight = totalUpW / totalDist
		downWeight = totalDownW / totalDist
	}

	return HysteresisResult{
		State:      state,
		ElevCost:   totalElevCost,
		UpWeight:   upWeight,
		DownWeight: downWeight,
	}
}

// elevationCorrection estimates the latent cost stored in the hysteresis buffers.
// Used for closed-set comparison: bestCost stores cost + elevationCorrection.
//
// NOTE: This intentionally computes cost from the FULL buffer (starting at 0),
// while propagateHysteresis only triggers elevCost for the portion ABOVE PenaltyBuffer.
// This is a conservative over-approximation (same approach as BRouter): it assumes
// all buffered elevation will eventually trigger cost. This guarantees the closed-set
// comparison never underestimates, at the price of slightly aggressive pruning.
func (s PathState) elevationCorrection(cfg *CostConfig) float32 {
	var correction float32
	if s.Ehbu > 0 {
		correction += float32(s.Ehbu) / 1e3 * cfg.UphillCost
	}
	if s.Ehbd > 0 {
		correction += float32(s.Ehbd) / 1e3 * cfg.DownhillCost
	}
	return correction
}
