package routing

import (
	"math"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// BuildCandidatePool selects up to poolSize diverse turnaround candidates from
// reachable nodes using a max-angular-spread strategy.
//
// Selection order:
//  1. Seed (index 0): the node+state whose DistanceM is closest to
//     targetDistanceM/2.  This anchors the pool at the ideal half-distance
//     turnaround point.
//  2. Subsequent slots: iterative max-min bearing spread from startLat/Lon.
//     Each iteration picks the candidate whose minimum bearing distance to
//     all already-selected candidates is largest.
//
// Spatial floor: a candidate is only eligible if its Haversine distance to
// every already-selected node exceeds minDist.
//
// Tiebreakers are resolved by ascending NodeID, making the result fully
// deterministic for any fixed reachable slice ordering.
//
// The returned slice preserves insertion order (seed first); callers must not
// re-sort if priority order matters.
func BuildCandidatePool(
	reachable []ReachableNode,
	startLat, startLon float64,
	targetDistanceM float64,
	poolSize int,
	minDist float64,
) []CandidateSelection {
	if len(reachable) == 0 || poolSize <= 0 {
		return nil
	}

	// --- Seed (slot 0): state closest to targetDistanceM/2 ---
	halfTarget := targetDistanceM / 2.0
	seedNodeIdx := findNodeClosestToHalfTarget(reachable, halfTarget)
	seedStateIdx := findStateClosestToHalfTarget(reachable[seedNodeIdx].States, halfTarget)

	pool := []CandidateSelection{
		{NodeIdx: seedNodeIdx, StateIdx: seedStateIdx},
	}

	if len(reachable) == 1 || poolSize == 1 {
		return pool
	}

	// selected tracks the actual ReachableNode pointers for distance/bearing checks.
	selected := []*ReachableNode{&reachable[seedNodeIdx]}

	// --- Iterative max-angular-spread ---
	for len(pool) < poolSize {
		bestMinAngle := -1.0
		bestNodeIdx := -1
		bestNodeID := uint64(math.MaxUint64)

		for i := range reachable {
			// Skip already selected nodes.
			if alreadySelected(pool, i) {
				continue
			}

			node := &reachable[i]

			// Spatial floor: must be far enough from every selected node.
			if tooCloseToPool(node, selected, minDist) {
				continue
			}

			// Bearing from start to this candidate.
			bearing := geo.BearingDeg(startLat, startLon, node.Lat, node.Lon)

			// Minimum angular distance to any already-selected candidate.
			minAngle := minAngularDistToSelected(bearing, startLat, startLon, selected)

			// Max-min: pick the candidate that is angularly farthest from the
			// already-selected set.  Tiebreak: lower NodeID wins.
			if minAngle > bestMinAngle ||
				(minAngle == bestMinAngle && node.NodeID < bestNodeID) {
				bestMinAngle = minAngle
				bestNodeIdx = i
				bestNodeID = node.NodeID
			}
		}

		if bestNodeIdx == -1 {
			// No more eligible candidates.
			break
		}

		stateIdx := findStateClosestToHalfTarget(reachable[bestNodeIdx].States, halfTarget)
		pool = append(pool, CandidateSelection{NodeIdx: bestNodeIdx, StateIdx: stateIdx})
		selected = append(selected, &reachable[bestNodeIdx])
	}

	return pool
}

// findNodeClosestToHalfTarget returns the index in reachable whose best state
// (States[0] — lowest cost) has DistanceM closest to halfTarget.
// Tiebreaker: lower NodeID.
func findNodeClosestToHalfTarget(reachable []ReachableNode, halfTarget float64) int {
	bestIdx := 0
	bestDelta := math.MaxFloat64

	for i := range reachable {
		node := &reachable[i]
		if len(node.States) == 0 {
			continue
		}
		// Use the state that is individually closest to halfTarget for each node.
		stateIdx := findStateClosestToHalfTarget(node.States, halfTarget)
		delta := math.Abs(float64(node.States[stateIdx].DistanceM) - halfTarget)

		if delta < bestDelta ||
			(delta == bestDelta && node.NodeID < reachable[bestIdx].NodeID) {
			bestDelta = delta
			bestIdx = i
		}
	}

	return bestIdx
}

// findStateClosestToHalfTarget returns the index within states whose DistanceM
// is closest to halfTarget. Tiebreaker: lower index (states are cost-sorted so
// lower index = lower cost = preferred).
func findStateClosestToHalfTarget(states []NodePareto, halfTarget float64) int {
	if len(states) == 0 {
		return 0
	}

	bestIdx := 0
	bestDelta := math.Abs(float64(states[0].DistanceM) - halfTarget)

	for j := 1; j < len(states); j++ {
		delta := math.Abs(float64(states[j].DistanceM) - halfTarget)
		if delta < bestDelta {
			bestDelta = delta
			bestIdx = j
		}
		// ties: keep lower index (already set on first occurrence)
	}

	return bestIdx
}

// alreadySelected reports whether reachable index i is already in pool.
func alreadySelected(pool []CandidateSelection, i int) bool {
	for _, sel := range pool {
		if sel.NodeIdx == i {
			return true
		}
	}
	return false
}

// tooCloseToPool reports whether node is within minDist metres of any selected
// node.
func tooCloseToPool(node *ReachableNode, selected []*ReachableNode, minDist float64) bool {
	for _, s := range selected {
		if geo.DistanceHaversine(node.Lat, node.Lon, s.Lat, s.Lon) < minDist {
			return true
		}
	}
	return false
}

// minAngularDistToSelected returns the minimum circular bearing distance (in
// degrees, 0–180) between bearing and the bearing from start to each selected
// node.  Returns math.MaxFloat64 when selected is empty (no constraint).
func minAngularDistToSelected(
	bearing float64,
	startLat, startLon float64,
	selected []*ReachableNode,
) float64 {
	if len(selected) == 0 {
		return math.MaxFloat64
	}

	minAngle := math.MaxFloat64
	for _, s := range selected {
		selBearing := geo.BearingDeg(startLat, startLon, s.Lat, s.Lon)
		diff := math.Abs(bearing - selBearing)
		if diff > 180 {
			diff = 360 - diff
		}
		if diff < minAngle {
			minAngle = diff
		}
	}
	return minAngle
}
