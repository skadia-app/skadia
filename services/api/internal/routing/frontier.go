package routing

// FrontierEntry is a priority queue entry for A*/Dijkstra search.
// Priority key = Cost + AirDist.
type FrontierEntry struct {
	NodeID       uint32
	Cost         float32   // g(n) — accumulated cost
	AirDist      float32   // coefficient * equirectDist(node, dst)
	State        PathState // elevation hysteresis state (8 bytes, inline)
	Parent       uint32    // parent node ID
	Edge         uint32    // edge index for re-tracking
	LastEdgeDX   float32   // normalized direction X (lon-component)
	LastEdgeDY   float32   // normalized direction Y (lat-component)
	LastWayClass uint8     // 0=normal, 1=ferry, 2=mainroad
}

// Frontier is a custom typed binary min-heap over FrontierEntry.
// 2-3x faster than container/heap (no interface dispatch).
type Frontier struct {
	entries []FrontierEntry
	size    int
}

// Len returns the number of entries in the frontier.
func (f *Frontier) Len() int { return f.size }

// Reset clears the frontier without releasing the backing slice.
func (f *Frontier) Reset() { f.size = 0 }

// Push adds an entry to the frontier.
func (f *Frontier) Push(e FrontierEntry) {
	if f.size >= len(f.entries) {
		f.entries = append(f.entries, e)
	} else {
		f.entries[f.size] = e
	}
	f.siftUp(f.size)
	f.size++
}

// Pop removes and returns the entry with the lowest key (Cost + AirDist).
func (f *Frontier) Pop() FrontierEntry {
	if f.size == 0 {
		panic("routing: Pop on empty Frontier")
	}
	f.size--
	min := f.entries[0]
	f.entries[0] = f.entries[f.size]
	if f.size > 0 {
		f.siftDown(0)
	}
	return min
}

func (f *Frontier) key(i int) float32 {
	e := &f.entries[i]
	return e.Cost + e.AirDist
}

func (f *Frontier) siftUp(i int) {
	for i > 0 {
		parent := (i - 1) / 2
		if f.key(i) >= f.key(parent) {
			break
		}
		f.entries[i], f.entries[parent] = f.entries[parent], f.entries[i]
		i = parent
	}
}

func (f *Frontier) siftDown(i int) {
	ki := f.key(i)
	for {
		left := 2*i + 1
		if left >= f.size {
			break
		}
		smallest := left
		kSmallest := f.key(left)
		if right := left + 1; right < f.size {
			if kr := f.key(right); kr < kSmallest {
				smallest = right
				kSmallest = kr
			}
		}
		if ki <= kSmallest {
			break
		}
		f.entries[i], f.entries[smallest] = f.entries[smallest], f.entries[i]
		i = smallest
		// ki stays the same: we continue sifting the same element downward
	}
}
