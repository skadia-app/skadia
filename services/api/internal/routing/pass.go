// pass.go — Shared cost computation functions used by unified.go.
package routing

import "math"

// computeTurnCost returns the turn cost and the normalized direction of the new edge.
// lastDX/lastDY are the previous edge direction (0,0 if no previous edge).
// turnCostBase is cfg.TurnCost (0 means no turn costs).
// edgeDistCm is the edge distance in centimeters — used to scale turn cost proportionally
// so short edges (< 200m, common after TNE in urban areas) don't accumulate excessive turn costs.
func computeTurnCost(curLon, curLat, targetLon, targetLat int32,
	lastDX, lastDY, turnCostBase float32, edgeDistCm uint32) (turnCost, newDX, newDY float32) {

	edgeDX := float32(targetLon - curLon)
	edgeDY := float32(targetLat - curLat)
	lenSq := edgeDX*edgeDX + edgeDY*edgeDY
	if lenSq > 0 {
		invLen := 1.0 / float32(math.Sqrt(float64(lenSq)))
		newDX = edgeDX * invLen
		newDY = edgeDY * invLen
	}

	if turnCostBase > 0 && lastDX != 0 && lenSq > 0 {
		cosAngle := lastDX*newDX + lastDY*newDY
		// Scale turn cost by edge distance: short edges (< 200m) get proportionally
		// less turn cost. Without this, TNE edges in pedestrian zones (2-50m) accumulate
		// 60+ turn costs that blow the pass0 budget for 2km routes.
		distScale := min(1.0, float32(edgeDistCm)/20000.0) // 20000 cm = 200m
		turnCost = turnCostBase * (1.0 - cosAngle) * distScale
	}
	return
}
