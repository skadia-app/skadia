package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestSnapOSMLandmarksToGraph(t *testing.T) {
	tiles := []graph.TileScanResult{{
		TileID:    graph.TileIDFromCoord(47.0, 11.0),
		NodeCount: 3,
		NodeLats:  []int32{470000000, 470500000, 471000000},
		NodeLons:  []int32{110000000, 110500000, 111000000},
		NodeEles:  []int32{1000, 2000, 3000},
	}}
	landmarks := []OSMLandmark{
		{OSMID: 123, Name: "Brenner", Lat: 47.05, Lon: 11.05, Ele: 1370},
	}
	candidates := SnapOSMLandmarksToGraph(landmarks, tiles)
	if len(candidates) != 1 {
		t.Fatalf("got %d candidates, want 1", len(candidates))
	}
	if candidates[0].Lat != 470500000 {
		t.Errorf("snapped to wrong node: lat=%d, want 470500000", candidates[0].Lat)
	}
}

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
