package routing

import (
	"math"
	"slices"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

// SelectedVariant is the result of variant selection for a single semantic
// variant: which node was chosen, which Pareto state index to use, and the
// score that drove the selection.
type SelectedVariant struct {
	Variant  SemanticVariant
	Node     *ReachableNode
	StateIdx int
	Score    float64
}

// MinVariantDistanceM returns the minimum spatial separation (in metres)
// that two selected variant nodes must have. The floor of 300 m prevents
// trivial near-duplicate variants on short target distances.
func MinVariantDistanceM(targetDistanceM float64) float64 {
	return math.Max(300.0, 0.05*targetDistanceM)
}

// tooCloseToSelected reports whether node is within minDist metres of any
// already-selected variant node.
func tooCloseToSelected(node *ReachableNode, selected []SelectedVariant, minDist float64) bool {
	for _, s := range selected {
		if geo.DistanceHaversine(node.Lat, node.Lon, s.Node.Lat, s.Node.Lon) < minDist {
			return true
		}
	}
	return false
}

// countVariantCandidates counts how many reachable nodes are eligible for the
// given variant (not too close to already-selected nodes, and the variant
// Score function returns a valid state index for them).
func countVariantCandidates(
	reachable []ReachableNode,
	variant SemanticVariant,
	ctx VariantSelectionContext,
	selected []SelectedVariant,
	minDist float64,
) int {
	count := 0
	for i := range reachable {
		node := &reachable[i]
		if tooCloseToSelected(node, selected, minDist) {
			continue
		}
		_, stateIdx := variant.Score(*node, ctx)
		if stateIdx >= 0 {
			count++
		}
	}
	return count
}

// SelectVariants picks up to maxVariants turn-around nodes from reachable,
// one per semantic variant, with spatial diversity enforced by minDist.
// If fewer than maxVariants semantic variants produce a valid candidate the
// remaining slots are filled by fillWithGeometricFallback.
//
// The function is deterministic: for the same reachable slice ordering and
// the same variants slice it always returns the same result.
func SelectVariants(
	reachable []ReachableNode,
	startLat, startLon float64,
	variants []SemanticVariant,
	maxVariants int,
	ctx VariantSelectionContext,
) []SelectedVariant {
	minDist := MinVariantDistanceM(ctx.TargetDistanceM)
	selected := make([]SelectedVariant, 0, maxVariants)

	for _, variant := range variants {
		if len(selected) >= maxVariants {
			break
		}

		candidateCount := countVariantCandidates(reachable, variant, ctx, selected, minDist)
		if candidateCount < variant.MinPoolSize {
			continue
		}

		bestScore := math.Inf(-1)
		var bestNode *ReachableNode
		bestStateIdx := -1

		for i := range reachable {
			node := &reachable[i]
			if tooCloseToSelected(node, selected, minDist) {
				continue
			}
			score, stateIdx := variant.Score(*node, ctx)
			if stateIdx < 0 || math.IsInf(score, -1) {
				continue
			}
			if score > bestScore {
				bestScore = score
				bestNode = node
				bestStateIdx = stateIdx
			}
		}

		if bestNode != nil {
			selected = append(selected, SelectedVariant{
				Variant:  variant,
				Node:     bestNode,
				StateIdx: bestStateIdx,
				Score:    bestScore,
			})
		}
	}

	if len(selected) < maxVariants {
		selected = fillWithGeometricFallback(reachable, startLat, startLon, selected, maxVariants, minDist)
	}

	return selected
}

// fillWithGeometricFallback fills remaining variant slots by maximising the
// angular spread of turnaround directions relative to the start point.
// Each additional node is chosen so that its bearing from start differs
// maximally from the bearings of already-selected nodes.
//
// Nodes that are within minDist of any already-selected node are skipped to
// preserve spatial diversity.
func fillWithGeometricFallback(
	reachable []ReachableNode,
	startLat, startLon float64,
	selected []SelectedVariant,
	maxVariants int,
	minDist float64,
) []SelectedVariant {
	result := slices.Clone(selected)

	for len(result) < maxVariants {
		bestBearingSpread := -1.0
		var bestNode *ReachableNode
		const fallbackStateIdx = 0

		for i := range reachable {
			node := &reachable[i]
			if tooCloseToSelected(node, result, minDist) {
				continue
			}

			nodeBearing := geo.BearingDeg(startLat, startLon, node.Lat, node.Lon)

			// Find the smallest angular distance to any already-selected node.
			// If result is empty every node has infinite spread, so we pick the
			// first eligible node (minAngle stays MaxFloat64).
			minAngle := math.MaxFloat64
			for _, s := range result {
				selBearing := geo.BearingDeg(startLat, startLon, s.Node.Lat, s.Node.Lon)
				diff := math.Abs(nodeBearing - selBearing)
				if diff > 180 {
					diff = 360 - diff
				}
				if diff < minAngle {
					minAngle = diff
				}
			}

			if minAngle > bestBearingSpread {
				bestBearingSpread = minAngle
				bestNode = node
			}
		}

		if bestNode == nil {
			break
		}

		result = append(result, SelectedVariant{
			Variant: SemanticVariant{
				ID:            "geometric",
				LabelKey:      "route.variant.geometric",
				FallbackLabel: "alternative direction",
				MinPoolSize:   1,
			},
			Node:     bestNode,
			StateIdx: fallbackStateIdx,
			Score:    bestBearingSpread,
		})
	}

	return result
}
