package routing

import (
	"context"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// buildTestTile creates a minimal 3-node, 4-edge bidirectional graph for testing.
// Nodes: 0 → 1 → 2 (and reverse). All edges are "tertiary" highway so the
// trekking cost function assigns a finite cost.
func buildTestTile(tileID uint32, lat0, lon0, lat1, lon1, lat2, lon2 float64) *graph.Graph {
	g := &graph.Graph{
		NodeCount: 3,
		EdgeCount: 4,
		TileID:    tileID,
		Version:   2,
		// CSR: node 0 has 1 edge (→1), node 1 has 2 edges (→0, →2), node 2 has 1 edge (→1)
		Offsets:    []uint32{0, 1, 3, 4},
		Targets:    []uint32{1, 0, 2, 1},
		EdgeDist:   []uint32{100000, 100000, 100000, 100000}, // 1000m in cm
		EdgeTags1:  make([]uint64, 4),
		EdgeTags2:  make([]uint64, 4),
		EdgeOSMIdx: []uint32{0, 0, 0, 0},
		OSMIDPool:  []int64{1},
		NodeLat:    []int32{int32(lat0 * 1e7), int32(lat1 * 1e7), int32(lat2 * 1e7)},
		NodeLon:    []int32{int32(lon0 * 1e7), int32(lon1 * 1e7), int32(lon2 * 1e7)},
		NodeEle:    []int32{0, 0, 0},
		NodeTags:   []uint8{0, 0, 0},
		// Edge geometry/elevation: empty but valid offset arrays (len = EdgeCount+1)
		EdgeGeomOff: []uint32{0, 0, 0, 0, 0},
		EdgeElevOff: []uint32{0, 0, 0, 0, 0},
	}

	tags := graph.EncodeOSMTags(&graph.RawTags{Highway: "tertiary"})
	for i := range g.EdgeTags1 {
		g.EdgeTags1[i] = tags
	}

	g.Grid = graph.BuildSpatialIndex(g, 0.01)
	return g
}

func TestTileRouter_SingleTile(t *testing.T) {
	tileID := graph.TileIDFromCoord(47.5, 8.5)

	g := buildTestTile(tileID,
		47.5, 8.5, // node 0
		47.51, 8.5, // node 1
		47.52, 8.5, // node 2
	)

	cache := NewTileCache(2_500_000_000)
	cache.Put(tileID, g)

	tr := newTileRouterFromCache(cache)

	ctx := context.Background()
	cfg := DefaultTrekkingConfig()
	result, err := tr.Route(ctx, RouteRequest{
		FromLat: 47.5, FromLon: 8.5,
		ToLat: 47.52, ToLon: 8.5,
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Coordinates) < 2 {
		t.Errorf("expected >= 2 coordinates, got %d", len(result.Coordinates))
	}
	if result.DistanceM < 1 {
		t.Errorf("distance too small: %f", result.DistanceM)
	}
}

func TestCrossTileRoute_TwoTiles(t *testing.T) {
	tileA := graph.TileIDFromCoord(47.5, 8.5)
	tileB := graph.TileIDFromCoord(46.5, 8.5)

	// Shared border node at 46.997° N — floor(46.997) = 46, so nominally in tileB.
	// However, TileIDFromCoord uses math.Floor, so we need the border node to
	// appear in both tiles. The node at 46.997 has TileIDFromCoord → tileB.
	// In tileA, this node is a border node (near the tile's south edge at lat=47).
	// We use coordinates that TileIDFromCoord maps correctly.

	// tileA: lat range [47, 48). Nodes: 47.5 (interior), 47.003 (border, near south edge), 47.3 (interior)
	// The border node at 47.003 is within 0.02° of the south edge.
	gA := buildTestTile(tileA,
		47.5, 8.5, // node 0: interior (start)
		47.003, 8.5, // node 1: border (near south edge of tile)
		47.3, 8.5, // node 2: interior
	)
	// Mark node 1 as border.
	gA.IsBorder = []bool{false, true, false}
	gA.BorderIndex = graph.BuildBorderNodeIndex([]uint32{1}, gA.NodeLat, gA.NodeLon)

	// tileB: lat range [46, 47). Nodes: 46.5 (interior, destination), 47.003 (border, matching tileA node 1), 46.8 (interior)
	// Wait — 47.003 has floor=47, so TileIDFromCoord gives tileA, not tileB.
	// For cross-tile matching, the border node must have the SAME coordinates in both tiles.
	// In tileB, we place node 1 at the same coords (47.003, 8.5).
	// Even though TileIDFromCoord(47.003, 8.5) = tileA, the node exists in tileB's
	// graph because it's in the overlap region (BorderBuffer).
	gB := buildTestTile(tileB,
		46.5, 8.5, // node 0: interior (destination)
		47.003, 8.5, // node 1: border (same coords as tileA node 1)
		46.8, 8.5, // node 2: interior
	)
	gB.IsBorder = []bool{false, true, false}
	gB.BorderIndex = graph.BuildBorderNodeIndex([]uint32{1}, gB.NodeLat, gB.NodeLon)

	cache := NewTileCache(2_500_000_000)
	cache.Put(tileA, gA)
	cache.Put(tileB, gB)

	tr := newTileRouterFromCache(cache)

	ctx := context.Background()
	cfg := DefaultTrekkingConfig()
	result, err := tr.Route(ctx, RouteRequest{
		FromLat: 47.5, FromLon: 8.5,
		ToLat: 46.5, ToLon: 8.5,
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Coordinates) < 2 {
		t.Errorf("expected >= 2 coordinates, got %d", len(result.Coordinates))
	}
	if result.DistanceM < 1 {
		t.Errorf("distance too small: %f", result.DistanceM)
	}
	t.Logf("cross-tile route: %d coords, %.0fm distance", len(result.Coordinates), result.DistanceM)
}
