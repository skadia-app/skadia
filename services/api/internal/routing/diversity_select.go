package routing

import (
	"math"
	"sort"
)

// PickDiverseIndices selects up to k indices from metrics that are maximally
// diverse in Z-score space. Two anchor candidates are seeded first:
//  1. The route closest to targetDistanceM (always selected).
//  2. The route closest to targetAscentM, if provided and sufficiently distant
//     in Z-score space from anchor 1 (euclideanZDist >= 0.5).
//
// Remaining slots are filled via max-min diversity (greedy farthest-point).
// Ties in all comparisons are broken by ascending NodeID.
// The returned slice is sorted ascending for determinism.
func PickDiverseIndices(
	metrics []CandidateMetrics,
	targetDistanceM float64,
	targetAscentM *float64,
	k int,
) []int {
	n := len(metrics)
	if n == 0 || k <= 0 {
		return nil
	}
	if n <= k {
		result := make([]int, n)
		for i := range result {
			result[i] = i
		}
		return result
	}

	zs, active := computeZScores(metrics)

	selected := make([]int, 0, k)
	inSelected := make(map[int]bool, k)

	// Anchor 1: closest to targetDistanceM.
	anchor1 := -1
	bestDelta := math.MaxFloat64
	for i, m := range metrics {
		delta := math.Abs(m.DistanceM - targetDistanceM)
		if anchor1 == -1 || delta < bestDelta || (delta == bestDelta && m.NodeID < metrics[anchor1].NodeID) {
			anchor1 = i
			bestDelta = delta
		}
	}
	selected = append(selected, anchor1)
	inSelected[anchor1] = true

	// Anchor 2: closest to targetAscentM, if provided and k >= 2.
	if targetAscentM != nil && k >= 2 {
		anchor2 := -1
		bestDelta = math.MaxFloat64
		for i, m := range metrics {
			if inSelected[i] {
				continue
			}
			delta := math.Abs(m.AscentM - *targetAscentM)
			if anchor2 == -1 || delta < bestDelta || (delta == bestDelta && m.NodeID < metrics[anchor2].NodeID) {
				anchor2 = i
				bestDelta = delta
			}
		}
		if anchor2 != -1 {
			dist := euclideanZDist(zs[anchor2], zs[anchor1], active)
			if dist >= 0.5 {
				selected = append(selected, anchor2)
				inSelected[anchor2] = true
			}
		}
	}

	// Max-min diversity fill.
	for len(selected) < k {
		bestIdx := -1
		bestMinDist := -1.0
		for i := range metrics {
			if inSelected[i] {
				continue
			}
			// Compute min distance from i to all selected.
			minDist := math.MaxFloat64
			for _, s := range selected {
				d := euclideanZDist(zs[i], zs[s], active)
				if d < minDist {
					minDist = d
				}
			}
			if bestIdx == -1 ||
				minDist > bestMinDist ||
				(minDist == bestMinDist && metrics[i].NodeID < metrics[bestIdx].NodeID) {
				bestIdx = i
				bestMinDist = minDist
			}
		}
		if bestIdx == -1 {
			break
		}
		selected = append(selected, bestIdx)
		inSelected[bestIdx] = true
	}

	sort.Ints(selected)
	return selected
}
