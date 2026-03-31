package routing

import "math"

// R07: HeapIdx from Spec §9.2 deliberately omitted — this implementation
// uses lazy deletion (stale-entry check in A* loop) instead of decrease-key.
type FrontierEntry struct {
	NodeID uint64  // global node ID
	Cost   float32 // f(n) = g(n) + c*h(n)
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

// Pop removes and returns the entry with the lowest Cost.
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

// PeekCost returns the cost of the minimum entry without removing it.
// Returns math.MaxFloat32 if the frontier is empty.
func (f *Frontier) PeekCost() float32 {
	if f.size == 0 {
		return math.MaxFloat32
	}
	return f.entries[0].Cost
}

func (f *Frontier) key(i int) float32 {
	return f.entries[i].Cost
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
