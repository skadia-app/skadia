package routing

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// TileManager loads and caches routing tiles on demand.
// It wraps a TileIndex (which tile files exist on disk) and a TileCache
// (LRU budget). Concurrent requests for the same tile are collapsed via
// singleflight so the file is only read once.
type TileManager struct {
	index    atomic.Pointer[TileIndex]
	cache    *TileCache
	loadOnce singleflight.Group
}

// NewTileManager creates a TileManager by scanning tileDir for tile files
// and initialising an LRU cache with the given byte budget.
func NewTileManager(tileDir string, cacheSize int64) (*TileManager, error) {
	idx, err := ScanTileIndex(tileDir)
	if err != nil {
		return nil, err
	}
	m := &TileManager{
		cache: NewTileCache(cacheSize),
	}
	m.index.Store(idx)
	return m, nil
}

// GetTile returns the graph for the given tile ID, loading it from disk if
// necessary. Returns an error if no tile file exists for that ID.
func (m *TileManager) GetTile(tileID uint32) (*graph.Graph, error) {
	if g, ok := m.cache.Get(tileID); ok {
		return g, nil
	}

	key := fmt.Sprintf("%d", tileID)
	v, err, _ := m.loadOnce.Do(key, func() (any, error) {
		// Double-check after acquiring the singleflight slot.
		if g, ok := m.cache.Get(tileID); ok {
			return g, nil
		}

		idx := m.index.Load()
		path, ok := idx.TilePath(tileID)
		if !ok {
			lat, lon := graph.TileLatLon(tileID)
			return nil, fmt.Errorf("no routing data available for region %d°N %d°E (tile %d)",
				lat, lon, tileID)
		}

		loadStart := time.Now()
		g, err := graph.DeserializeMmap(path)
		if err != nil {
			return nil, fmt.Errorf("load tile %d from %s: %w", tileID, path, err)
		}

		m.cache.Put(tileID, g)
		slog.Info("tile loaded",
			"tile_id", tileID,
			"nodes", g.NodeCount,
			"edges", g.EdgeCount,
			"load_ms", time.Since(loadStart).Milliseconds(),
			"cache_bytes", m.cache.UsedBytes(),
			"path", path,
		)
		return g, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*graph.Graph), nil
}

// GetTileForCoord returns the graph for the tile that contains the given
// WGS-84 coordinate.
func (m *TileManager) GetTileForCoord(lat, lon float64) (*graph.Graph, error) {
	return m.GetTile(graph.TileIDFromCoord(lat, lon))
}

// PreloadTiles loads the given tile IDs concurrently into the cache.
// Tiles already cached are skipped. Errors are logged but not returned.
func (m *TileManager) PreloadTiles(tileIDs ...uint32) {
	var wg sync.WaitGroup
	for _, id := range tileIDs {
		if _, ok := m.cache.Get(id); ok {
			continue
		}
		wg.Add(1)
		go func(tid uint32) {
			defer wg.Done()
			if _, err := m.GetTile(tid); err != nil {
				slog.Warn("preload tile failed", "tile_id", tid, "error", err)
			}
		}(id)
	}
	wg.Wait()
}

// TileCount returns the number of tile files known to the index.
func (m *TileManager) TileCount() int { return m.index.Load().Count() }

// CachedCount returns the number of tiles currently held in the LRU cache.
func (m *TileManager) CachedCount() int { return m.cache.Len() }

// Rescan re-reads the tile directory, atomically replaces the index,
// and invalidates all cached tiles (they may have been rebuilt on disk).
func (m *TileManager) Rescan() error {
	oldIdx := m.index.Load()
	idx, err := ScanTileIndex(oldIdx.dir)
	if err != nil {
		return err
	}
	m.index.Store(idx)
	// Invalidate cache — rebuilt tiles on disk may differ from cached versions.
	// Safe because in-flight routing goroutines hold *Graph pointers that remain
	// valid (Go GC keeps them alive until the goroutine releases the reference).
	m.cache.Clear()
	return nil
}
