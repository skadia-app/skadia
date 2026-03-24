package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// TestSnapOSMLandmarksToGraph requires real tile files on disk.
// Covered by E2E landmark rebuild test (Task 10).

func TestBBoxFromTiles(t *testing.T) {
	tiles := []graph.TileScanResult{{
		TileID: 1, NodeCount: 3,
		NodeLats: []int32{460000000, 470000000, 480000000},
		NodeLons: []int32{100000000, 110000000, 120000000},
		NodeEles: []int32{100, 200, 300},
	}}
	south, west, north, east := BBoxFromTiles(tiles)
	if south != 46.0 || north != 48.0 || west != 10.0 || east != 12.0 {
		t.Errorf("BBox = (%f,%f,%f,%f), want (46,10,48,12)", south, west, north, east)
	}
}
