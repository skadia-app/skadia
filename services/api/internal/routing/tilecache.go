package routing

import (
	"container/list"
	"log/slog"
	"sync"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type TileCache struct {
	mu        sync.RWMutex
	maxBytes  int64
	usedBytes int64
	items     map[uint32]*list.Element
	order     *list.List
}

type cacheEntry struct {
	tileID    uint32
	graph     *graph.Graph
	sizeBytes int64
}

// estimateGraphBytes returns a rough byte estimate for a loaded graph's arrays.
// Used by the LRU cache to enforce its RAM budget.
func estimateGraphBytes(g *graph.Graph) int64 {
	n := int64(g.NodeCount)
	e := int64(g.EdgeCount)
	// Hot-path arrays: Offsets(4*(n+1)) + Targets(4e) + EdgeDist(4e)
	//   + EdgeTags1(8e) + EdgeTags2(8e) + EdgeOSMIdx(4e)
	//   + NodeLat(4n) + NodeLon(4n) + NodeEle(4n) + NodeTags(1n)
	//   + EdgeGeomOff(4e) + EdgeElevOff(4e)
	est := (n+1)*4 + e*(4+4+8+8+4+4+4) // 36 bytes/edge + offsets
	est += n * (4 + 4 + 4 + 1)         // 13 bytes/node
	// SpatialGrid: ~8 bytes/node (cell index + node list overhead)
	est += n * 8
	// IsBorder: 1 byte/node when present
	if g.IsBorder != nil {
		est += n
	}
	// BorderNodeIndex: 12 bytes/entry
	if g.BorderIndex != nil {
		est += int64(len(g.BorderIndex.Entries)) * 12
	}
	return est
}

func NewTileCache(maxBytes int64) *TileCache {
	return &TileCache{
		maxBytes: maxBytes,
		items:    make(map[uint32]*list.Element),
		order:    list.New(),
	}
}

func (c *TileCache) Get(tileID uint32) (*graph.Graph, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[tileID]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*cacheEntry).graph, true
}

func (c *TileCache) Put(tileID uint32, g *graph.Graph) {
	c.mu.Lock()
	defer c.mu.Unlock()
	size := estimateGraphBytes(g)
	if elem, ok := c.items[tileID]; ok {
		old := elem.Value.(*cacheEntry)
		c.usedBytes -= old.sizeBytes
		old.graph = g
		old.sizeBytes = size
		c.usedBytes += size
		c.order.MoveToFront(elem)
		return
	}
	for c.order.Len() > 0 && c.usedBytes+size > c.maxBytes {
		back := c.order.Back()
		if back == nil {
			break
		}
		evicted := c.order.Remove(back).(*cacheEntry)
		c.usedBytes -= evicted.sizeBytes
		delete(c.items, evicted.tileID)
	}
	if size > c.maxBytes {
		slog.Warn("tile exceeds cache budget", "tile_id", tileID, "tile_bytes", size, "budget_bytes", c.maxBytes)
	}
	entry := &cacheEntry{tileID: tileID, graph: g, sizeBytes: size}
	elem := c.order.PushFront(entry)
	c.items[tileID] = elem
	c.usedBytes += size
}

func (c *TileCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

func (c *TileCache) UsedBytes() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.usedBytes
}

// Clear evicts all tiles from the cache. In-flight users of *graph.Graph
// pointers are safe — Go GC keeps the objects alive until unreferenced.
func (c *TileCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[uint32]*list.Element)
	c.order.Init()
	c.usedBytes = 0
}
