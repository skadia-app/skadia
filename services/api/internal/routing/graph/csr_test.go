package graph

import (
	"math"
	"testing"
)

func TestBuildCSRTriangle(t *testing.T) {
	topo := TopologyResult{
		Nodes: []Coord{
			{Lat: 47.0, Lon: 11.0},
			{Lat: 47.001, Lon: 11.0},
			{Lat: 47.0, Lon: 11.001},
		},
		Edges: []TopologyEdge{
			{From: 0, To: 1, OSMID: 100, Tags: RawTags{Highway: "residential"}},
			{From: 1, To: 0, OSMID: 100, Tags: RawTags{Highway: "residential"}},
			{From: 1, To: 2, OSMID: 200, Tags: RawTags{Highway: "tertiary"}},
			{From: 2, To: 1, OSMID: 200, Tags: RawTags{Highway: "tertiary"}},
			{From: 0, To: 2, OSMID: 300, Tags: RawTags{Highway: "path"}},
			{From: 2, To: 0, OSMID: 300, Tags: RawTags{Highway: "path"}},
		},
		NodeTags:  []uint8{0, 0, 0},
		NodeCount: 3,
	}

	g := BuildCSR(topo)

	if g.NodeCount != 3 {
		t.Errorf("NodeCount: got %d, want 3", g.NodeCount)
	}
	if g.EdgeCount != 6 {
		t.Errorf("EdgeCount: got %d, want 6", g.EdgeCount)
	}
	for i := uint32(0); i < g.NodeCount; i++ {
		deg := g.OutDegree(i)
		if deg != 2 {
			t.Errorf("node %d out-degree: got %d, want 2", i, deg)
		}
	}
	if len(g.Offsets) != 4 {
		t.Errorf("Offsets len: got %d, want 4", len(g.Offsets))
	}
	for i, d := range g.EdgeDist {
		if d == 0 {
			t.Errorf("EdgeDist[%d] = 0, want > 0", i)
		}
	}
	if len(g.OSMIDPool) != 3 {
		t.Errorf("OSMIDPool len: got %d, want 3", len(g.OSMIDPool))
	}
}

func TestEquirectangularDist(t *testing.T) {
	dm := equirectangularDistDm(47.0, 11.0, 47.001, 11.0)
	meters := float64(dm) / 10.0
	if math.Abs(meters-111.0) > 5 {
		t.Errorf("equirectangular dist: got %.1fm, want ~111m", meters)
	}
}
