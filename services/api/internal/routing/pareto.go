package routing

import "slices"

const rootStateID uint32 = 0xFFFFFFFF
const maxStates = 1_500_000
const maxExpansions = 2_500_000

type paretoState struct {
	cost   float32
	ascent float32

	descent    float32
	dist       float32
	qualitySum float32
	avgFlat    float32 // populated at EXPORT time

	pathState PathState
	lastDX    float32
	lastDY    float32

	handle uint64

	parentStateID uint32
	lastWC        uint8

	covForestSum    float32
	covNoiseSum     float32
	covProtectedSum float32
	covUrbanSum     float32
	covRiverSum     float32
	covTrafficSum   float32
	covDistSum      float32
}

func (a paretoState) dominates(b paretoState) bool {
	return a.cost <= b.cost && a.ascent <= b.ascent &&
		(a.cost < b.cost || a.ascent < b.ascent)
}

type paretoFront struct {
	activeIDs  []uint32
	generation uint32
}

type reachableSlab struct {
	states []paretoState
}

type reachableState struct {
	slab            reachableSlab
	fronts          map[uint64]*paretoFront
	truncated       bool
	truncatedReason string
}

func newReachableState() *reachableState {
	return &reachableState{
		slab: reachableSlab{
			states: make([]paretoState, 0, maxStates),
		},
		fronts: make(map[uint64]*paretoFront, 200_000),
	}
}

func (rs *reachableState) tryInsert(nodeID uint64, new paretoState) (uint32, bool) {
	front, ok := rs.fronts[nodeID]
	if !ok {
		front = &paretoFront{
			activeIDs: make([]uint32, 0, 4),
		}
		rs.fronts[nodeID] = front
	}

	for _, id := range front.activeIDs {
		existing := rs.slab.states[id]
		if existing.dominates(new) {
			return 0, false
		}
	}

	if len(rs.slab.states) >= maxStates {
		if !rs.truncated {
			rs.truncated = true
			rs.truncatedReason = "max_states"
		} else if rs.truncatedReason == "max_expansions" {
			rs.truncatedReason = "both"
		}
		return 0, false
	}

	newID := uint32(len(rs.slab.states))
	rs.slab.states = append(rs.slab.states, new)

	j := 0
	for _, id := range front.activeIDs {
		if !new.dominates(rs.slab.states[id]) {
			front.activeIDs[j] = id
			j++
		}
	}
	front.activeIDs = front.activeIDs[:j]

	insertIdx, _ := slices.BinarySearchFunc(front.activeIDs, new.cost, func(id uint32, target float32) int {
		c := rs.slab.states[id].cost
		if c < target {
			return -1
		}
		if c > target {
			return 1
		}
		return 0
	})
	front.activeIDs = slices.Insert(front.activeIDs, insertIdx, newID)
	front.generation++

	return newID, true
}

func (rs *reachableState) markExpansionTruncated() {
	if !rs.truncated {
		rs.truncated = true
		rs.truncatedReason = "max_expansions"
	} else if rs.truncatedReason == "max_states" {
		rs.truncatedReason = "both"
	}
}

type paretoFrontierEntry struct {
	nodeID   uint64
	stateID  uint32
	cost     float32
	frontGen uint32
}
