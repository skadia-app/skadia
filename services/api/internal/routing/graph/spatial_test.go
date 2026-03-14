package graph

import (
	"math"
	"testing"
)

func TestMortonCode(t *testing.T) {
	tests := []struct {
		row, col uint32
	}{
		{0, 0},
		{1, 0},
		{0, 1},
		{3, 3},
		{100, 200},
	}
	for _, tt := range tests {
		code := mortonCode(tt.row, tt.col)
		gotRow, gotCol := mortonDecode(code)
		if gotRow != tt.row || gotCol != tt.col {
			t.Errorf("mortonCode(%d,%d)=%d → decode=(%d,%d)",
				tt.row, tt.col, code, gotRow, gotCol)
		}
	}
}

func TestBuildSpatialIndex(t *testing.T) {
	g := &Graph{
		NodeCount: 4,
		NodeLat:   []int32{470000000, 470100000, 470200000, 480000000},
		NodeLon:   []int32{110000000, 110100000, 110200000, 120000000},
	}

	grid := BuildSpatialIndex(g, 0.01)
	g.Grid = grid

	if grid.Rows == 0 || grid.Cols == 0 {
		t.Fatal("grid should have rows and cols")
	}
	if len(grid.CellStart) != grid.Rows*grid.Cols+1 {
		t.Errorf("CellStart len: got %d, want %d", len(grid.CellStart), grid.Rows*grid.Cols+1)
	}
	if len(grid.NodeOrder) != 4 {
		t.Errorf("NodeOrder len: got %d, want 4", len(grid.NodeOrder))
	}
}

func TestNearestNode(t *testing.T) {
	g := &Graph{
		NodeCount: 3,
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{110000000, 110000000, 110000000},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	nodeID, dist := NearestNode(g, 47.0011, 11.0)

	if nodeID != 1 {
		t.Errorf("nearest node: got %d, want 1", nodeID)
	}
	if dist > 200 {
		t.Errorf("distance too large: got %d dm", dist)
	}
}

func TestNearestNodeExactMatch(t *testing.T) {
	g := &Graph{
		NodeCount: 2,
		NodeLat:   []int32{470000000, 480000000},
		NodeLon:   []int32{110000000, 120000000},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	nodeID, dist := NearestNode(g, 47.0, 11.0)
	if nodeID != 0 {
		t.Errorf("exact match: got node %d, want 0", nodeID)
	}
	if dist > 1 {
		t.Errorf("exact match dist: got %d, want ~0", dist)
	}
}

func testDist(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	meanLat := (lat1 + lat2) / 2 * math.Pi / 180
	x := dLon * math.Cos(meanLat)
	return math.Sqrt(dLat*dLat+x*x) * 6371000
}
