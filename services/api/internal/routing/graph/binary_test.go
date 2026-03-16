package graph

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	topo := TopologyResult{
		Nodes: []Coord{
			{Lat: 47.0, Lon: 11.0},
			{Lat: 47.001, Lon: 11.0},
			{Lat: 47.002, Lon: 11.0},
			{Lat: 47.0, Lon: 11.001},
		},
		Edges: []TopologyEdge{
			{From: 0, To: 1, OSMID: 100, Tags: RawTags{Highway: "residential", Surface: "asphalt"}},
			{From: 1, To: 0, OSMID: 100, Tags: RawTags{Highway: "residential", Surface: "asphalt"}},
			{From: 1, To: 2, OSMID: 100, Tags: RawTags{Highway: "residential"}},
			{From: 2, To: 1, OSMID: 100, Tags: RawTags{Highway: "residential"}},
			{From: 0, To: 3, OSMID: 200, Tags: RawTags{Highway: "path", Oneway: true}},
		},
		NodeTags:  []uint8{0, BarrierGate, 0, 0},
		NodeCount: 4,
	}

	g := BuildCSR(topo)
	g.NodeEle = []int32{5000, 5200, 5500, 4800}
	g.Grid = BuildSpatialIndex(g, 0.01)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.graph")

	if err := Serialize(g, path); err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() < int64(HeaderSize) {
		t.Fatalf("file too small: %d bytes", info.Size())
	}

	g2, err := Deserialize(path)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}

	if g2.NodeCount != g.NodeCount {
		t.Errorf("NodeCount: got %d, want %d", g2.NodeCount, g.NodeCount)
	}
	if g2.EdgeCount != g.EdgeCount {
		t.Errorf("EdgeCount: got %d, want %d", g2.EdgeCount, g.EdgeCount)
	}
	if g2.Version != FormatVersion {
		t.Errorf("Version: got %d, want %d", g2.Version, FormatVersion)
	}

	for i := uint32(0); i < g.NodeCount; i++ {
		if g2.NodeLat[i] != g.NodeLat[i] {
			t.Errorf("NodeLat[%d]: got %d, want %d", i, g2.NodeLat[i], g.NodeLat[i])
		}
		if g2.NodeEle[i] != g.NodeEle[i] {
			t.Errorf("NodeEle[%d]: got %d, want %d", i, g2.NodeEle[i], g.NodeEle[i])
		}
		if g2.NodeTags[i] != g.NodeTags[i] {
			t.Errorf("NodeTags[%d]: got %d, want %d", i, g2.NodeTags[i], g.NodeTags[i])
		}
	}

	for i := uint32(0); i <= g.NodeCount; i++ {
		if g2.Offsets[i] != g.Offsets[i] {
			t.Errorf("Offsets[%d]: got %d, want %d", i, g2.Offsets[i], g.Offsets[i])
		}
	}
	for i := uint32(0); i < g.EdgeCount; i++ {
		if g2.Targets[i] != g.Targets[i] {
			t.Errorf("Targets[%d]: got %d, want %d", i, g2.Targets[i], g.Targets[i])
		}
		if g2.EdgeTags1[i] != g.EdgeTags1[i] {
			t.Errorf("EdgeTags1[%d]: got %d, want %d", i, g2.EdgeTags1[i], g.EdgeTags1[i])
		}
	}

	if g2.Grid == nil {
		t.Fatal("Grid should be loaded")
	}
	if g2.Grid.Rows != g.Grid.Rows || g2.Grid.Cols != g.Grid.Cols {
		t.Errorf("Grid dims: got %dx%d, want %dx%d",
			g2.Grid.Rows, g2.Grid.Cols, g.Grid.Rows, g.Grid.Cols)
	}
}

func TestDeltaEncodeDecodeElevation(t *testing.T) {
	elevations := []int32{5000, 5200, 5500, 5300, 4800, 5100}
	encoded := deltaEncodeElevation(elevations)
	decoded := deltaDecodeElevation(encoded, len(elevations))

	for i, want := range elevations {
		if decoded[i] != want {
			t.Errorf("elevation[%d]: got %d, want %d", i, decoded[i], want)
		}
	}
}

func TestDeltaEncodeLargeJump(t *testing.T) {
	elevations := []int32{0, 40000}
	encoded := deltaEncodeElevation(elevations)
	decoded := deltaDecodeElevation(encoded, len(elevations))

	if decoded[0] != 0 || decoded[1] != 40000 {
		t.Errorf("large jump: got %v, want [0, 40000]", decoded)
	}
}

func TestSerializeDeserialize_V2_WithGeometry(t *testing.T) {
	nodes := []Coord{{47.0, 11.0}, {47.001, 11.0}, {47.0, 11.001}}
	nodeTags := []uint8{0, 0, 0}

	dir := t.TempDir()
	edgeTmpPath := filepath.Join(dir, "edges.tmp")
	ew, err := NewEdgeFileWriter(edgeTmpPath)
	if err != nil {
		t.Fatal(err)
	}
	tags1 := EncodeOSMTags(&RawTags{Highway: "path"})
	ew.WriteWay(100, tags1, 0, false, []WaySegment{
		{FromID: 0, ToID: 1, Dist: 11100,
			GeomLat: []int32{470000000, 470005000, 470010000},
			GeomLon: []int32{110000000, 110000000, 110000000}},
	})
	ew.CloseWithNodeCount(3)

	g, err := BuildCSRFromFile(nodes, nodeTags, edgeTmpPath)
	if err != nil {
		t.Fatal(err)
	}
	g.NodeEle = []int32{5000, 5200, 4800}
	g.EdgeElevProf = make([]int32, len(g.EdgeGeomLat))
	for i := range g.EdgeElevProf {
		g.EdgeElevProf[i] = 5000 + int32(i)*100
	}
	copy(g.EdgeElevOff, g.EdgeGeomOff)
	g.Grid = BuildSpatialIndex(g, 0.01)

	graphPath := filepath.Join(dir, "test_v2.graph")
	if err := Serialize(g, graphPath); err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	g2, err := Deserialize(graphPath)
	if err != nil {
		t.Fatalf("Deserialize: %v", err)
	}

	if len(g2.EdgeGeomOff) != len(g.EdgeGeomOff) {
		t.Errorf("EdgeGeomOff len: got %d, want %d", len(g2.EdgeGeomOff), len(g.EdgeGeomOff))
	}
	if len(g2.EdgeGeomLat) != len(g.EdgeGeomLat) {
		t.Errorf("EdgeGeomLat len: got %d, want %d", len(g2.EdgeGeomLat), len(g.EdgeGeomLat))
	}
	for i := range g.EdgeGeomLat {
		if g2.EdgeGeomLat[i] != g.EdgeGeomLat[i] {
			t.Errorf("EdgeGeomLat[%d]: got %d, want %d", i, g2.EdgeGeomLat[i], g.EdgeGeomLat[i])
		}
	}
	if len(g2.EdgeElevProf) != len(g.EdgeElevProf) {
		t.Errorf("EdgeElevProf len: got %d, want %d", len(g2.EdgeElevProf), len(g.EdgeElevProf))
	}
	for i := range g.EdgeElevProf {
		if g2.EdgeElevProf[i] != g.EdgeElevProf[i] {
			t.Errorf("EdgeElevProf[%d]: got %d, want %d", i, g2.EdgeElevProf[i], g.EdgeElevProf[i])
		}
	}
	if g2.EdgeDist[0] != 11100 {
		t.Errorf("EdgeDist[0]: got %d, want 11100", g2.EdgeDist[0])
	}
}

func TestCorruptedCRCDetected(t *testing.T) {
	topo := TopologyResult{
		Nodes:     []Coord{{Lat: 47.0, Lon: 11.0}, {Lat: 47.001, Lon: 11.0}},
		Edges:     []TopologyEdge{{From: 0, To: 1, OSMID: 100}},
		NodeTags:  []uint8{0, 0},
		NodeCount: 2,
	}
	g := BuildCSR(topo)
	g.NodeEle = []int32{5000, 5200}
	g.Grid = BuildSpatialIndex(g, 0.01)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.graph")
	if err := Serialize(g, path); err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	data, _ := os.ReadFile(path)
	data[len(data)/2] ^= 0xFF
	os.WriteFile(path, data, 0644)

	_, err := Deserialize(path)
	if err == nil {
		t.Error("expected CRC error for corrupted file")
	}
}
