package routing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// GraphManager manages the in-memory routing graph with atomic pointer swap.
// It supports two modes:
//   - Monolithic: a single graph.bin loaded into memory (legacy).
//   - Tiled: on-demand tile loading via TileManager + TileRouter.
//
// When tile mode is initialized, IsTileMode() returns true and
// TileRoute() delegates to the cross-tile A* implementation.
// The monolithic graph remains available as fallback.
type GraphManager struct {
	graph    atomic.Pointer[graph.Graph]
	mu       sync.Mutex // serializes rebuilds
	routerMu sync.Mutex
	router   *Router

	// Tile-based routing (nil if monolithic mode).
	tileMgr    *TileManager
	tileRouter *TileRouter
}

// Router returns a Router instance for the current graph.
// Returns nil if no graph is loaded.
// The Router is created lazily and replaced when the graph is rebuilt.
// The caller holds a reference to the returned Router for the duration of
// the request. A concurrent Rebuild() may replace m.router, but this does
// not affect in-flight computations; the old graph remains live until all
// references are dropped.
func (m *GraphManager) Router() *Router {
	m.routerMu.Lock()
	defer m.routerMu.Unlock()
	g := m.graph.Load()
	if g == nil {
		return nil
	}
	if m.router == nil || m.router.Graph != g {
		m.router = NewRouter(g, 3, 10)
	}
	return m.router
}

// Load returns the current graph, or nil if none is loaded.
func (m *GraphManager) Load() *graph.Graph {
	return m.graph.Load()
}

// LoadFromFile deserializes a graph from disk using mmap for cold sections
// (geometry, OSM IDs) and stores it atomically. The old graph's mmap
// resources are cleaned up automatically by the GC finalizer after all
// in-flight requests drop their references.
func (m *GraphManager) LoadFromFile(path string) error {
	g, err := graph.DeserializeMmap(path)
	if err != nil {
		return fmt.Errorf("deserialize %s: %w", path, err)
	}
	m.routerMu.Lock()
	m.router = nil
	m.routerMu.Unlock()
	m.graph.Store(g)
	slog.Info("routing graph loaded (mmap)",
		"nodes", g.NodeCount,
		"edges", g.EdgeCount,
		"version", g.Version,
		"build_time", g.BuildTime,
	)
	return nil
}

// Rebuild builds a new graph from PostGIS data and swaps it atomically.
// Returns an error if a rebuild is already in progress.
func (m *GraphManager) Rebuild(ctx context.Context, pool *pgxpool.Pool, outputPath string,
	progressFn func(step string, pct int)) error {

	if !m.mu.TryLock() {
		return fmt.Errorf("rebuild already in progress")
	}
	defer m.mu.Unlock()

	// Build produces a heap graph and serializes to outputPath.
	// Then load the serialized file via mmap for consistent behavior.
	_, err := graph.Build(ctx, pool, outputPath, nil, progressFn)
	if err != nil {
		return err
	}

	if err := m.LoadFromFile(outputPath); err != nil {
		return fmt.Errorf("load after build: %w", err)
	}
	slog.Info("routing graph rebuilt and loaded via mmap",
		"path", outputPath,
	)
	return nil
}

// InitTileMode attempts to initialize tile-based routing from the given
// directory. Returns (true, nil) if tiles were found and loaded,
// (false, nil) if the directory does not exist or contains no tiles,
// or (false, err) on real errors.
func (m *GraphManager) InitTileMode(tileDir string, cacheSize int64) (bool, error) {
	if _, err := os.Stat(tileDir); os.IsNotExist(err) {
		return false, nil
	}
	mgr, err := NewTileManager(tileDir, cacheSize)
	if err != nil {
		return false, err
	}
	if mgr.TileCount() == 0 {
		return false, nil
	}
	m.tileMgr = mgr
	m.tileRouter = NewTileRouter(mgr)
	slog.Info("tile mode initialized",
		"tiles", mgr.TileCount(),
		"cache_bytes", cacheSize,
	)
	return true, nil
}

// IsTileMode reports whether tile-based routing is active.
func (m *GraphManager) IsTileMode() bool {
	return m.tileRouter != nil
}

// TileRoute computes a route using the tiled routing engine.
// Returns an error if tile mode has not been initialized.
func (m *GraphManager) TileRoute(ctx context.Context, req RouteRequest,
	cfg *CostConfig) (*RouteResult, error) {
	if m.tileRouter == nil {
		return nil, fmt.Errorf("tile mode not initialized")
	}
	return m.tileRouter.Route(ctx, req, cfg)
}

// TileManager returns the underlying TileManager, or nil if tile mode
// is not active. Used by the tile-build handler to rescan after builds.
func (m *GraphManager) TileManager() *TileManager {
	return m.tileMgr
}

// RebuildTiles builds all tiles from PostGIS data and (re-)initializes
// tile mode. Returns an error if a rebuild is already in progress.
func (m *GraphManager) RebuildTiles(ctx context.Context, pool *pgxpool.Pool,
	tileDir string, cacheSize int64,
	progressFn func(tileID uint32, step string, pct int)) error {

	if !m.mu.TryLock() {
		return fmt.Errorf("rebuild already in progress")
	}
	defer m.mu.Unlock()

	results, err := graph.BuildAllTiles(ctx, pool, tileDir, progressFn)
	if err != nil {
		return err
	}
	slog.Info("tile build complete", "tiles_built", len(results))

	// Re-initialize tile mode so new tiles are picked up.
	if m.tileMgr != nil {
		if err := m.tileMgr.Rescan(); err != nil {
			return fmt.Errorf("rescan tiles after build: %w", err)
		}
	} else {
		if _, err := m.InitTileMode(tileDir, cacheSize); err != nil {
			return fmt.Errorf("init tile mode after build: %w", err)
		}
	}
	return nil
}
