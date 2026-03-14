package routing

// PathState holds the elevation hysteresis buffers.
// 8 bytes, inline copied (too small for sync.Pool).
type PathState struct {
	Ehbu int32 // uphill buffer in millimeters
	Ehbd int32 // downhill buffer in millimeters
}

// propagateHysteresis updates hysteresis buffers and returns elevation cost.
// deltaEleMilli: elevation change in millimeters (positive = uphill).
// distMilli: edge distance in millimeters.
// Unit choice: millimeters (not micrometers) to avoid int32 overflow.
// Max EdgeDist 65535 dm * 100 = 6,553,500 mm — fits int32.
func propagateHysteresis(parent PathState, deltaEleMilli int32,
	distMilli int32, cfg *CostConfig) (PathState, float32) {

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

	// Cost activation
	var elevCost float32
	penBuf := int32(cfg.ElevPenaltyBuffer * 1e3)
	if state.Ehbd > penBuf {
		excess := float32(state.Ehbd-penBuf) / 1e3
		elevCost += excess * cfg.DownhillCostFactor
	}
	if state.Ehbu > penBuf {
		excess := float32(state.Ehbu-penBuf) / 1e3
		elevCost += excess * cfg.UphillCostFactor
	}

	return state, elevCost
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
		correction += float32(s.Ehbu) / 1e3 * cfg.UphillCostFactor
	}
	if s.Ehbd > 0 {
		correction += float32(s.Ehbd) / 1e3 * cfg.DownhillCostFactor
	}
	return correction
}

