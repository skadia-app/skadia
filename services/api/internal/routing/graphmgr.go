package routing

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/paulmach/orb"

	"github.com/skadia-app/skadia/services/api/internal/osm/coverage"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// coverageAdapter wraps coverage.TileGridCache to satisfy graph.CoverageReader.
// Needed to avoid import cycles: coverage imports graph, so graph cannot import coverage.
type coverageAdapter struct {
	cache *coverage.TileGridCache
}

func (a *coverageAdapter) PreloadTile(tileID uint32) error {
	return a.cache.PreloadTile(tileID)
}

func (a *coverageAdapter) SampleWay(geom orb.LineString) graph.CoverageData {
	cr := a.cache.SampleWay(geom)
	return graph.CoverageData{
		ForestPct:         cr.ForestPct,
		ProtectedArea:     cr.ProtectedArea,
		LanduseClass:      cr.LanduseClass,
		NoiseScore:        cr.NoiseScore,
		Urbanization:      cr.Urbanization,
		RiverProximity:    cr.RiverProximity,
		Water:             cr.Water,
		TrafficPopulate:   cr.TrafficPopulate,
		TrafficIndustrial: cr.TrafficIndustrial,
		TrafficMWDensity:  cr.TrafficMWDensity,
		TrafficHWDensity:  cr.TrafficHWDensity,
	}
}

// GraphManager manages the in-memory routing graph with atomic pointer swap.
// It supports two modes:
//   - Monolithic: a single graph.bin loaded into memory (legacy fallback for Rebuild).
//   - Tiled: TiledGraphReader + UnifiedRouter for on-demand tile loading.
//
// The monolithic graph is only used by Rebuild() for non-tiled graph builds.
// All routing goes through UnifiedRouter().
type GraphManager struct {
	graph    atomic.Pointer[graph.Graph]
	mu       sync.Mutex // serializes rebuilds
	routerMu sync.Mutex

	// Tile management (for status display and rebuild rescan).
	tileMgr *TileManager

	// Unified routing via GraphReader interface.
	tiledReader   *TiledGraphReader
	unifiedRouter *UnifiedRouter
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
	_, err := graph.Build(ctx, pool, outputPath, nil, nil, nil, progressFn)
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

// InitTiledReader initializes a TiledGraphReader and UnifiedRouter.
// Returns (true, nil) if tiles were found, (false, nil) if dir is absent/empty,
// or (false, err) on real errors.
func (m *GraphManager) InitTiledReader(tileDir string, cacheBytes int64) (bool, error) {
	if _, err := os.Stat(tileDir); os.IsNotExist(err) {
		return false, nil
	}
	reader := NewTiledGraphReader(tileDir, cacheBytes)
	if !reader.HasTiles() {
		return false, nil
	}
	m.tiledReader = reader
	m.unifiedRouter = NewUnifiedRouter(reader, 8)

	// Initialize TileManager for status display (tile_count, tile_cached).
	mgr, err := NewTileManager(tileDir, cacheBytes)
	if err == nil && mgr.TileCount() > 0 {
		m.tileMgr = mgr
	}

	slog.Info("unified tiled reader initialized",
		"tile_dir", tileDir,
		"cache_bytes", cacheBytes,
	)
	return true, nil
}

// UnifiedRouter returns the UnifiedRouter.
// Lazily recreates from tiledReader after a Flush/RebuildTiles cycle.
// Returns nil if InitTiledReader has not been called or no tiles are available.
func (m *GraphManager) UnifiedRouter() *UnifiedRouter {
	m.routerMu.Lock()
	defer m.routerMu.Unlock()
	if m.unifiedRouter != nil {
		return m.unifiedRouter
	}
	// Lazy recreate: tiledReader is still valid after Flush, only the
	// unifiedRouter pointer was cleared to force recreation with fresh tiles.
	if m.tiledReader != nil {
		m.unifiedRouter = NewUnifiedRouter(m.tiledReader, 8)
		slog.Info("unified router lazily recreated after tile rebuild")
	}
	return m.unifiedRouter
}

// TileManager returns the underlying TileManager, or nil if tile mode
// is not active. Used by the status handler for tile_count/tile_cached.
func (m *GraphManager) TileManager() *TileManager {
	return m.tileMgr
}

// RebuildTiles builds all tiles from PostGIS data and (re-)initializes
// tile mode. Returns an error if a rebuild is already in progress.
func (m *GraphManager) RebuildTiles(ctx context.Context, pool *pgxpool.Pool,
	tileDir string, cacheSize int64, hgtDirs []string, workers int,
	filterBBox [4]float64, force bool,
	progressFn func(tileID uint32, step string, pct int)) error {

	if !m.mu.TryLock() {
		return fmt.Errorf("rebuild already in progress")
	}
	defer m.mu.Unlock()

	// A07: Create CoverageReader for Phase 2 (no DB roundtrip per way).
	// TODO: load noise settings from user/system configuration instead of defaults.
	noiseCfg := coverage.DefaultNoiseSettings()
	cr := coverage.NewTileGridCache(pool, noiseCfg)
	defer cr.ReleaseAll()
	adapter := &coverageAdapter{cache: cr}

	results, err := graph.BuildAllTiles(ctx, pool, tileDir, hgtDirs, adapter, workers, filterBBox, force, progressFn)
	if err != nil {
		return err
	}
	slog.Info("tile build complete", "tiles_built", len(results))

	// Re-initialize TileManager for status display.
	if m.tileMgr != nil {
		if err := m.tileMgr.Rescan(); err != nil {
			return fmt.Errorf("rescan tiles after build: %w", err)
		}
	} else {
		mgr, err := NewTileManager(tileDir, cacheSize)
		if err == nil && mgr.TileCount() > 0 {
			m.tileMgr = mgr
		}
	}

	// Flush the unified reader cache so rebuilt tiles are served fresh.
	if m.tiledReader != nil {
		m.tiledReader.Flush()
		m.unifiedRouter = nil // force lazy re-creation
	}
	return nil
}
