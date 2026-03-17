package routing

// crossNodeState holds per-node state in the cross-tile A*.
type crossNodeState struct {
	BestCost      float32
	ParentNode    uint64
	ParentEdge    uint32
	ParentEdgeTile uint32 // tile that owns ParentEdge (may differ from parent's tile at border crossings)
}

// CrossTileState holds the sparse working state for cross-tile A*.
type CrossTileState struct {
	nodes    map[uint64]*crossNodeState
	frontier CrossFrontier
}

// CrossFrontierEntry is a frontier entry with GlobalNodeID.
type CrossFrontierEntry struct {
	GlobalNodeID uint64
	Cost         float32
	AirDist      float32
	State        PathState
	Parent       uint64
	Edge         uint32
	TileID       uint32
	LastEdgeDX   float32 // normalized direction X (lon-component)
	LastEdgeDY   float32 // normalized direction Y (lat-component)
	LastWayClass uint8   // 0=normal, 1=ferry, 2=mainroad
}

// CrossFrontier is a min-heap of CrossFrontierEntry.
type CrossFrontier []CrossFrontierEntry

func (f CrossFrontier) Len() int { return len(f) }
func (f CrossFrontier) Less(i, j int) bool {
	return f[i].Cost+f[i].AirDist < f[j].Cost+f[j].AirDist
}
func (f CrossFrontier) Swap(i, j int) { f[i], f[j] = f[j], f[i] }
func (f *CrossFrontier) Push(x CrossFrontierEntry) {
	*f = append(*f, x)
	f.siftUp(len(*f) - 1)
}
func (f *CrossFrontier) Pop() CrossFrontierEntry {
	n := len(*f)
	top := (*f)[0]
	(*f)[0] = (*f)[n-1]
	*f = (*f)[:n-1]
	if len(*f) > 0 {
		f.siftDown(0)
	}
	return top
}

func (f CrossFrontier) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if !f.Less(i, parent) {
			break
		}
		f.Swap(i, parent)
		i = parent
	}
}

func (f CrossFrontier) siftDown(i int) {
	n := len(f)
	for {
		left := 2*i + 1
		if left >= n {
			break
		}
		best := left
		if right := left + 1; right < n && f.Less(right, left) {
			best = right
		}
		if !f.Less(best, i) {
			break
		}
		f.Swap(i, best)
		i = best
	}
}

func newCrossTileState() *CrossTileState {
	return &CrossTileState{
		nodes: make(map[uint64]*crossNodeState),
	}
}

func (s *CrossTileState) getOrCreate(gid uint64) *crossNodeState {
	if ns, ok := s.nodes[gid]; ok {
		return ns
	}
	ns := &crossNodeState{BestCost: 1e30}
	s.nodes[gid] = ns
	return ns
}
