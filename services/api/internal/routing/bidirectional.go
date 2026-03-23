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
func RunBackwardSearch(reader GraphReader, dst uint64, cfg *CostConfig,
	initialUpperBound float32) BackwardResult {

	result := BackwardResult{
		LowerBound: make(map[uint64]float32, 8192),
		UpperBound: initialUpperBound,
	}

	frontier := Frontier{entries: make([]FrontierEntry, 0, 4096)}
	result.LowerBound[dst] = 0
	frontier.Push(FrontierEntry{NodeID: dst, Cost: 0})

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
