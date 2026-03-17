package routing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// GraphManager manages the in-memory routing graph with atomic pointer swap.
type GraphManager struct {
	graph    atomic.Pointer[graph.Graph]
	mu       sync.Mutex // serializes rebuilds
	routerMu sync.Mutex
	router   *Router
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
