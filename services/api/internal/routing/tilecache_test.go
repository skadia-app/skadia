package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestTileCache_GetAndEvict(t *testing.T) {
	// Each graph with NodeCount=100, EdgeCount=50 is ~4304 bytes.
	// Budget for 2 tiles: 9000.
	cache := NewTileCache(9000)
	g1 := &graph.Graph{TileID: 1, NodeCount: 100, EdgeCount: 50}
	g2 := &graph.Graph{TileID: 2, NodeCount: 100, EdgeCount: 50}
	g3 := &graph.Graph{TileID: 3, NodeCount: 100, EdgeCount: 50}

	cache.Put(1, g1)
	cache.Put(2, g2)

	got, ok := cache.Get(1)
	if !ok || got.TileID != 1 {
		t.Fatal("tile 1 should be cached")
	}

	// Insert third tile → evicts LRU (tile 2, since tile 1 was just accessed).
	cache.Put(3, g3)
	_, ok = cache.Get(2)
	if ok {
		t.Error("tile 2 should have been evicted")
	}
	_, ok = cache.Get(1)
	if !ok {
		t.Error("tile 1 should still be cached (recently accessed)")
	}
}

func TestTileCache_LargeEvictsMultiple(t *testing.T) {
	// 3 small tiles (~490 bytes each) fit in 2000 budget.
	cache := NewTileCache(2000)
	cache.Put(1, &graph.Graph{TileID: 1, NodeCount: 10, EdgeCount: 5})
	cache.Put(2, &graph.Graph{TileID: 2, NodeCount: 10, EdgeCount: 5})
	cache.Put(3, &graph.Graph{TileID: 3, NodeCount: 10, EdgeCount: 5})
	if cache.Len() != 3 {
		t.Fatalf("len = %d, want 3", cache.Len())
	}

	// Insert a large tile that exceeds budget → evicts all small tiles.
	cache.Put(4, &graph.Graph{TileID: 4, NodeCount: 100000, EdgeCount: 50000})
	if cache.Len() != 1 {
		t.Errorf("after large insert: len = %d, want 1", cache.Len())
	}
}
