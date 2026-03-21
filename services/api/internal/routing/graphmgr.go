package routing

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"sync"

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

func (a *coverageAdapter) PreloadTile(ctx context.Context, tileID uint32) error {
	return a.cache.PreloadTile(ctx, tileID)
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

// GraphManager manages the tiled routing graph.
// All routing goes through UnifiedRouter backed by TiledGraphReader.
type GraphManager struct {
	mu       sync.Mutex // serializes rebuilds
	routerMu sync.Mutex

	// Tile management (for status display and rebuild rescan).
	tileMgr *TileManager

	// Unified routing via GraphReader interface.
	tiledReader   *TiledGraphReader
	unifiedRouter *UnifiedRouter
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

// Reader returns the GraphReader for surface enrichment (and other non-routing
// consumers that need tile-level graph access). Returns nil if no tiles loaded.
func (m *GraphManager) Reader() GraphReader {
	if m.tiledReader == nil {
		return nil
	}
	return m.tiledReader
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

	// === Landmark Preprocessing (best-effort, non-fatal) ===
	slog.Info("starting landmark preprocessing after tile build")
	tmpReader := NewTiledGraphReader(tileDir, 2_000_000_000) // 2 GB cache
	landmarkProgressFn := func(step string, current, total int) {
		if progressFn != nil {
			progressFn(math.MaxUint32, fmt.Sprintf("landmarks: %s", step), 0)
		}
	}
	meta, landmarkErr := RunLandmarkPreprocessing(ctx, tmpReader, tileDir, landmarkProgressFn)
	if landmarkErr != nil {
		slog.Warn("landmark preprocessing failed (non-fatal)", "error", landmarkErr)
	} else {
		slog.Info("landmark preprocessing complete", "landmarks", meta.LandmarkCount)
	}
	tmpReader.Flush()

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
