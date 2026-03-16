package graph

import (
	"math"
	"path/filepath"
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

func TestBuildCSRFromFile_Triangle(t *testing.T) {
	nodes := []Coord{{47.0, 11.0}, {47.001, 11.0}, {47.0, 11.001}}
	nodeTags := []uint8{0, 0, 0}

	tmpFile := filepath.Join(t.TempDir(), "edges.tmp")
	ew, err := NewEdgeFileWriter(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	// Way 1: node 0 ↔ node 1 (bidirectional)
	tags1a := EncodeOSMTags(&RawTags{Highway: "path"})
	ew.WriteWay(100, tags1a, 0, false, []WaySegment{
		{FromID: 0, ToID: 1, Dist: 11100,
			GeomLat: []int32{470000000, 470010000},
			GeomLon: []int32{110000000, 110000000}},
	})
	// Way 2: node 1 ↔ node 2 (bidirectional)
	tags1b := EncodeOSMTags(&RawTags{Highway: "track"})
	ew.WriteWay(200, tags1b, 0, false, []WaySegment{
		{FromID: 1, ToID: 2, Dist: 11100,
			GeomLat: []int32{470010000, 470000000},
			GeomLon: []int32{110000000, 110010000}},
	})
	ew.CloseWithNodeCount(3)

	g, err := BuildCSRFromFile(nodes, nodeTags, tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if g.NodeCount != 3 {
		t.Errorf("NodeCount: got %d, want 3", g.NodeCount)
	}
	if g.EdgeCount != 4 {
		t.Errorf("EdgeCount: got %d, want 4", g.EdgeCount)
	}
	if g.EdgeDist[0] != 11100 {
		t.Errorf("EdgeDist[0]: got %d, want 11100 cm", g.EdgeDist[0])
	}
	if len(g.EdgeGeomOff) != 5 {
		t.Errorf("EdgeGeomOff len: got %d, want 5", len(g.EdgeGeomOff))
	}
}

func TestEquirectangularDist(t *testing.T) {
	dm := equirectangularDistDm(47.0, 11.0, 47.001, 11.0)
	meters := float64(dm) / 10.0
	if math.Abs(meters-111.0) > 5 {
		t.Errorf("equirectangular dist: got %.1fm, want ~111m", meters)
	}
}
