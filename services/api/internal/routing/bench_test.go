// bench_test.go
package routing

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// makeGridGraph creates a rows×cols grid graph with optional elevation noise.
// Each cell is ~100m apart. Total nodes = rows*cols.
func makeGridGraph(rows, cols int, elevNoise bool) *graph.Graph {
	nodeCount := uint32(rows * cols)
	var edgeList [][4]int

	nodeID := func(r, c int) int { return r*cols + c }

	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			n := nodeID(r, c)
			// Right neighbor
			if c < cols-1 {
				edgeList = append(edgeList, [4]int{n, nodeID(r, c+1), 1000, 16}) // 100m, path
				edgeList = append(edgeList, [4]int{nodeID(r, c+1), n, 1000, 16})
			}
			// Down neighbor
			if r < rows-1 {
				edgeList = append(edgeList, [4]int{n, nodeID(r+1, c), 1000, 16})
				edgeList = append(edgeList, [4]int{nodeID(r+1, c), n, 1000, 16})
			}
		}
	}

	nodes := make([][3]float64, nodeCount)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			ele := 5000.0 // 500m in dm
			if elevNoise {
				ele += float64(rand.Intn(200) - 100) // ±10m noise
			}
			nodes[nodeID(r, c)] = [3]float64{
				47.0 + float64(r)*0.001,
				11.0 + float64(c)*0.001,
				ele,
			}
		}
	}

	return makeTestGraph(nodes, edgeList)
}

func BenchmarkRoute_Grid32x32(b *testing.B) {
	g := makeGridGraph(32, 32, true) // 1K nodes
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()
	ctx := context.Background()

	req := RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.031, ToLon: 11.031,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Route(ctx, req, cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoute_Grid100x100(b *testing.B) {
	g := makeGridGraph(100, 100, true) // 10K nodes
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()
	ctx := context.Background()

	req := RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.099, ToLon: 11.099,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Route(ctx, req, cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoute_Grid316x316(b *testing.B) {
	g := makeGridGraph(316, 316, true) // ~100K nodes
	r := NewRouter(g, 1, 5)
	cfg := simpleCostConfig()
	ctx := context.Background()

	req := RouteRequest{
		FromLat: 47.0, FromLon: 11.0,
		ToLat: 47.315, ToLon: 11.315,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := r.Route(ctx, req, cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRoute_Concurrent_5(b *testing.B) {
	g := makeGridGraph(100, 100, true) // 10K nodes
	r := NewRouter(g, 3, 5)            // 3 hot-pool states
	cfg := simpleCostConfig()

	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		req := RouteRequest{
			FromLat: 47.0, FromLon: 11.0,
			ToLat: 47.099, ToLon: 11.099,
		}
		for pb.Next() {
			_, err := r.Route(ctx, req, cfg)
			if err != nil {
				b.Error(err)
				return
			}
		}
	})
}

func BenchmarkRoute_Concurrent_20_SparseFallback(b *testing.B) {
	g := makeGridGraph(50, 50, true) // 2.5K nodes (small for speed)
	r := NewRouter(g, 1, 10)         // 1 hot-pool → most use sparse
	cfg := simpleCostConfig()

	b.RunParallel(func(pb *testing.PB) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req := RouteRequest{
			FromLat: 47.0, FromLon: 11.0,
			ToLat: 47.049, ToLon: 11.049,
		}
		for pb.Next() {
			_, err := r.Route(ctx, req, cfg)
			if err != nil {
				b.Error(err)
				return
			}
		}
	})
}
