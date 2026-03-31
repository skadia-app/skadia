package routing

// BackwardResult holds the results of a backward cost_min Dijkstra search.
// Used as pruning boundary for the forward A* search.
type BackwardResult struct {
	LowerBound map[uint64]float32 // nodeID -> cost_min distance from dst
	UpperBound float32            // STATIC — set once, no tightening
}

// RunBackwardSearch runs a Dijkstra with cost_min from dst.
// cost_min = min(ecf.Flat, ecf.Up, ecf.Down) — most optimistic cost for any state.
// Stops when top_cost > initialUpperBound.
func RunBackwardSearch(reader GraphReader, dstA, dstB uint64,
	initCostA, initCostB float32,
	cfg *CostConfig, initialUpperBound float32) BackwardResult {

	// Pre-size map based on expected expansion radius.
	hint := min(max(8192, int(initialUpperBound/10)), 5_000_000)
	result := BackwardResult{
		LowerBound: make(map[uint64]float32, hint),
		UpperBound: initialUpperBound,
	}

	frontier := Frontier{entries: make([]FrontierEntry, 0, min(hint, 65536))}
	result.LowerBound[dstA] = initCostA
	frontier.Push(FrontierEntry{NodeID: dstA, Cost: initCostA})
	if dstB != dstA {
		result.LowerBound[dstB] = initCostB
		frontier.Push(FrontierEntry{NodeID: dstB, Cost: initCostB})
	}

	for frontier.Len() > 0 {
		entry := frontier.Pop()

		if entry.Cost > result.UpperBound {
			break
		}

		if best, ok := result.LowerBound[entry.NodeID]; ok && entry.Cost > best+0.001 {
			continue
		}

		reader.ForEachInEdge(entry.NodeID, func(ev EdgeView) {
			ecf := cfg.EdgeCost(ev.Tags1, ev.Tags2)
			costMin := ecf.CostMin()
			if costMin >= costBlocked {
				return
			}

			edgeCost := float32(ev.Dist) / 100.0 * costMin
			newCost := entry.Cost + edgeCost

			if newCost > result.UpperBound {
				return
			}

			if best, ok := result.LowerBound[ev.Target]; ok && newCost >= best {
				return
			}

			result.LowerBound[ev.Target] = newCost
			frontier.Push(FrontierEntry{NodeID: ev.Target, Cost: newCost})
		})
	}

	return result
}

// ShouldPrune returns true if the forward search should skip this node.
// Uses STATIC upper bound — no tightening during inadmissible Pass0.
// Pass0 is inadmissible (passCoeff > 1) so fwd_g(v) is NOT final at expansion.
// Tightening could prune optimal paths.
func (br *BackwardResult) ShouldPrune(nodeID uint64, fwdCost float32) bool {
	if br == nil {
		return false
	}
	bwdCost, ok := br.LowerBound[nodeID]
	if !ok {
		return false // not reached by backward — don't prune
	}
	return fwdCost+bwdCost > br.UpperBound
}
