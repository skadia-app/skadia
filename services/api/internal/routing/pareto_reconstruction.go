package routing

import (
	"errors"
	"fmt"
	"slices"
)

// EdgeHandle is an opaque edge identifier used during path reconstruction.
type EdgeHandle uint64

// ReconstructPath rebuilds the edge sequence from the root state to targetStateID
// by walking the parent chain through the slab.
// Returns edges in start-to-target order, plus a pointer to the target state.
func ReconstructPath(
	rs *reachableState,
	targetStateID uint32,
) ([]EdgeHandle, *paretoState, error) {
	const maxHops = 10000

	if int(targetStateID) >= len(rs.slab.states) {
		return nil, nil, fmt.Errorf("reconstruction: targetStateID %d out of range (slab len %d)",
			targetStateID, len(rs.slab.states))
	}
	targetState := &rs.slab.states[targetStateID]

	var edges []EdgeHandle
	curID := targetStateID

	for i := 0; i < maxHops; i++ {
		if curID == rootStateID {
			slices.Reverse(edges)
			return edges, targetState, nil
		}
		if int(curID) >= len(rs.slab.states) {
			return nil, nil, fmt.Errorf("reconstruction: stateID %d out of range", curID)
		}
		state := &rs.slab.states[curID]
		if state.handle != 0 {
			edges = append(edges, EdgeHandle(state.handle))
		}
		curID = state.parentStateID
	}
	return nil, nil, errors.New("reconstruction: max hops exceeded (corrupt parent chain)")
}
