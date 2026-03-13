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
	graph atomic.Pointer[graph.Graph]
	mu    sync.Mutex // serializes rebuilds
}

// Load returns the current graph, or nil if none is loaded.
func (m *GraphManager) Load() *graph.Graph {
	return m.graph.Load()
}

// LoadFromFile deserializes a graph from disk and stores it atomically.
func (m *GraphManager) LoadFromFile(path string) error {
	g, err := graph.Deserialize(path)
	if err != nil {
		return fmt.Errorf("deserialize %s: %w", path, err)
	}
	m.graph.Store(g)
	slog.Info("routing graph loaded",
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

	g, err := graph.Build(ctx, pool, outputPath, progressFn)
	if err != nil {
		return err
	}

	m.graph.Store(g)
	slog.Info("routing graph rebuilt",
		"nodes", g.NodeCount,
		"edges", g.EdgeCount,
	)
	return nil
}
