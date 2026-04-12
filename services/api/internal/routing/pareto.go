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

type paretoFrontier struct {
	entries []paretoFrontierEntry
}

func (f *paretoFrontier) Len() int { return len(f.entries) }

func (f *paretoFrontier) Push(e paretoFrontierEntry) {
	f.entries = append(f.entries, e)
	f.siftUp(len(f.entries) - 1)
}

func (f *paretoFrontier) Pop() paretoFrontierEntry {
	top := f.entries[0]
	n := len(f.entries)
	f.entries[0] = f.entries[n-1]
	f.entries = f.entries[:n-1]
	if len(f.entries) > 0 {
		f.siftDown(0)
	}
	return top
}

func (f *paretoFrontier) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if f.entries[i].cost < f.entries[parent].cost {
			f.entries[i], f.entries[parent] = f.entries[parent], f.entries[i]
			i = parent
		} else {
			break
		}
	}
}

func (f *paretoFrontier) siftDown(i int) {
	n := len(f.entries)
	for {
		left := 2*i + 1
		right := 2*i + 2
		smallest := i
		if left < n && f.entries[left].cost < f.entries[smallest].cost {
			smallest = left
		}
		if right < n && f.entries[right].cost < f.entries[smallest].cost {
			smallest = right
		}
		if smallest == i {
			break
		}
		f.entries[i], f.entries[smallest] = f.entries[smallest], f.entries[i]
		i = smallest
	}
}
