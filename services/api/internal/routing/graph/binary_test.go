package graph

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
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

func TestDeserializeMmap_RoundTrip(t *testing.T) {
	nodes := []Coord{{47.0, 11.0}, {47.001, 11.0}, {47.0, 11.001}}
	nodeTags := []uint8{0, BarrierGate, 0}

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

	graphPath := filepath.Join(dir, "test_mmap.graph")
	if err := Serialize(g, graphPath); err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	g2, err := DeserializeMmap(graphPath)
	if err != nil {
		t.Fatalf("DeserializeMmap: %v", err)
	}

	// Verify mmap backing is present.
	if g2.mmapData == nil {
		t.Fatal("mmapData should be non-nil")
	}
	if g2.mmapFile == nil {
		t.Fatal("mmapFile should be non-nil")
	}

	// Hot sections.
	if g2.NodeCount != g.NodeCount {
		t.Errorf("NodeCount: got %d, want %d", g2.NodeCount, g.NodeCount)
	}
	if g2.EdgeCount != g.EdgeCount {
		t.Errorf("EdgeCount: got %d, want %d", g2.EdgeCount, g.EdgeCount)
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
		if g2.EdgeDist[i] != g.EdgeDist[i] {
			t.Errorf("EdgeDist[%d]: got %d, want %d", i, g2.EdgeDist[i], g.EdgeDist[i])
		}
		if g2.EdgeTags1[i] != g.EdgeTags1[i] {
			t.Errorf("EdgeTags1[%d]: got %d, want %d", i, g2.EdgeTags1[i], g.EdgeTags1[i])
		}
	}
	for i := uint32(0); i < g.NodeCount; i++ {
		if g2.NodeLat[i] != g.NodeLat[i] {
			t.Errorf("NodeLat[%d]: got %d, want %d", i, g2.NodeLat[i], g.NodeLat[i])
		}
		if g2.NodeEle[i] != g.NodeEle[i] {
			t.Errorf("NodeEle[%d]: got %d, want %d", i, g2.NodeEle[i], g.NodeEle[i])
		}
	}
	if len(g2.EdgeGeomOff) != len(g.EdgeGeomOff) {
		t.Errorf("EdgeGeomOff len: got %d, want %d", len(g2.EdgeGeomOff), len(g.EdgeGeomOff))
	}
	if len(g2.EdgeElevOff) != len(g.EdgeElevOff) {
		t.Errorf("EdgeElevOff len: got %d, want %d", len(g2.EdgeElevOff), len(g.EdgeElevOff))
	}

	// Cold sections are now loaded in DeserializeMmap (required for cross-tile retrack).
	// Verify they are populated and match the original.
	if len(g2.EdgeGeomLat) != len(g.EdgeGeomLat) {
		t.Errorf("EdgeGeomLat len: got %d, want %d", len(g2.EdgeGeomLat), len(g.EdgeGeomLat))
	}
	for i := range g.EdgeGeomLat {
		if g2.EdgeGeomLat[i] != g.EdgeGeomLat[i] {
			t.Errorf("EdgeGeomLat[%d]: got %d, want %d", i, g2.EdgeGeomLat[i], g.EdgeGeomLat[i])
		}
	}
	if len(g2.EdgeGeomLon) != len(g.EdgeGeomLon) {
		t.Errorf("EdgeGeomLon len: got %d, want %d", len(g2.EdgeGeomLon), len(g.EdgeGeomLon))
	}
	for i := range g.EdgeGeomLon {
		if g2.EdgeGeomLon[i] != g.EdgeGeomLon[i] {
			t.Errorf("EdgeGeomLon[%d]: got %d, want %d", i, g2.EdgeGeomLon[i], g.EdgeGeomLon[i])
		}
	}
	if len(g2.OSMIDPool) != len(g.OSMIDPool) {
		t.Errorf("OSMIDPool len: got %d, want %d", len(g2.OSMIDPool), len(g.OSMIDPool))
	}
	for i := range g.OSMIDPool {
		if g2.OSMIDPool[i] != g.OSMIDPool[i] {
			t.Errorf("OSMIDPool[%d]: got %d, want %d", i, g2.OSMIDPool[i], g.OSMIDPool[i])
		}
	}
	for i := range g.EdgeElevProf {
		if g2.EdgeElevProf[i] != g.EdgeElevProf[i] {
			t.Errorf("EdgeElevProf[%d]: got %d, want %d", i, g2.EdgeElevProf[i], g.EdgeElevProf[i])
		}
	}
	if len(g2.EdgeOSMIdx) != len(g.EdgeOSMIdx) {
		t.Fatalf("EdgeOSMIdx len: got %d, want %d", len(g2.EdgeOSMIdx), len(g.EdgeOSMIdx))
	}
	for i := range g.EdgeOSMIdx {
		if g2.EdgeOSMIdx[i] != g.EdgeOSMIdx[i] {
			t.Errorf("EdgeOSMIdx[%d]: got %d, want %d", i, g2.EdgeOSMIdx[i], g.EdgeOSMIdx[i])
		}
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

func TestSerializeDeserializeV4ElevSegDist(t *testing.T) {
	g := &Graph{
		NodeCount: 3,
		EdgeCount: 2,
		Version:   FormatVersion,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{10000, 20000},
		EdgeTags1: []uint64{0, 0},
		EdgeTags2: []uint64{0, 0},
		EdgeOSMIdx: []uint32{0, 0},
		OSMIDPool: []int64{12345},
		NodeLat:   []int32{470000000, 470100000, 470200000},
		NodeLon:   []int32{80000000, 80100000, 80200000},
		NodeEle:   []int32{5000, 5100, 5200},
		NodeTags:  []uint8{0, 0, 0},
		// Edge 0: 3 geom points (after DP), 5 elev points (full)
		EdgeGeomOff: []uint32{0, 3, 5},
		EdgeGeomLat: []int32{470000000, 470050000, 470100000, 470100000, 470200000},
		EdgeGeomLon: []int32{80000000, 80050000, 80100000, 80100000, 80200000},
		// Elevation: 5 points for edge 0, 4 points for edge 1 = 9 total
		EdgeElevOff:  []uint32{0, 5, 9},
		EdgeElevProf: []int32{5000, 5020, 5050, 5080, 5100, 5100, 5130, 5170, 5200},
		// ElevSegDist: 4 for edge 0 (5-1), 3 for edge 1 (4-1) = 7 total
		EdgeElevSegDist: []uint16{1100, 1200, 1050, 1150, 900, 1300, 1100},
		IsBorder: []bool{false, false, false},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test_v4.bin")

	if err := Serialize(g, path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	g2, err := Deserialize(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}

	if g2.Version != FormatVersion {
		t.Errorf("version: got %d, want %d", g2.Version, FormatVersion)
	}

	// ElevSegDist round-trip
	if len(g2.EdgeElevSegDist) != len(g.EdgeElevSegDist) {
		t.Fatalf("ElevSegDist len: got %d, want %d", len(g2.EdgeElevSegDist), len(g.EdgeElevSegDist))
	}
	for i := range g.EdgeElevSegDist {
		if g2.EdgeElevSegDist[i] != g.EdgeElevSegDist[i] {
			t.Errorf("ElevSegDist[%d]: got %d, want %d", i, g2.EdgeElevSegDist[i], g.EdgeElevSegDist[i])
		}
	}

	// Separated ElevOff (different from GeomOff after DP)
	if len(g2.EdgeElevOff) != len(g.EdgeElevOff) {
		t.Fatalf("ElevOff len: got %d, want %d", len(g2.EdgeElevOff), len(g.EdgeElevOff))
	}
	for i := range g.EdgeElevOff {
		if g2.EdgeElevOff[i] != g.EdgeElevOff[i] {
			t.Errorf("ElevOff[%d]: got %d, want %d", i, g2.EdgeElevOff[i], g.EdgeElevOff[i])
		}
	}
}

func TestSerialize_SectionAlignment(t *testing.T) {
	// Graph with odd NodeCount so NodeTags has an odd byte length.
	g := &Graph{
		NodeCount: 3, EdgeCount: 2,
		Offsets:   []uint32{0, 1, 2, 2},
		Targets:   []uint32{1, 2},
		EdgeDist:  []uint32{100, 200},
		EdgeTags1: []uint64{0, 0},
		EdgeTags2: []uint64{0, 0},
		NodeLat:   []int32{470000000, 470010000, 470020000},
		NodeLon:   []int32{80000000, 80000000, 80000000},
		NodeEle:   []int32{5000, 5100, 5200},
		NodeTags:  []uint8{0, 1, 0}, // 3 bytes — odd length
		EdgeGeomOff:     []uint32{0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, true, false},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	f, err := os.CreateTemp("", "align_test_*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	if err := Serialize(g, f.Name()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	sectionCnt := le.Uint16(data[60:62])
	dirStart := HeaderSize
	for i := range int(sectionCnt) {
		off := dirStart + i*12
		typ := le.Uint16(data[off : off+2])
		offset := uint64(data[off+2]) | uint64(data[off+3])<<8 |
			uint64(data[off+4])<<16 | uint64(data[off+5])<<24 |
			uint64(data[off+6])<<32 | uint64(data[off+7])<<40

		// NodeTags and IsBorder are uint8 sections — alignment not required.
		if typ == SectionNodeTags || typ == SectionIsBorder {
			continue
		}
		// All other data sections must be 4-byte aligned.
		if typ >= SectionOffsets && typ <= SectionEdgeElevSegDist {
			if offset%4 != 0 {
				t.Errorf("section type %d has misaligned offset %d (%%4 = %d)", typ, offset, offset%4)
			}
		}
	}
}

func TestDeserializeV3Rejected(t *testing.T) {
	// Create a minimal v3 header with valid CRC so the CRC check passes and
	// the version check fires with "unsupported version: 3".
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "v3.bin")

	// Build the payload (HeaderSize bytes).
	payload := make([]byte, HeaderSize)
	copy(payload[0:4], "FRGR")                                         // magic
	binary.LittleEndian.PutUint32(payload[4:8], 3)                     // version 3
	// remaining bytes are zero (nodeCount, edgeCount, poolSize, bbox, buildTime, sectionCnt)

	// Compute Castagnoli CRC32 over payload, append it.
	crc := crc32.Checksum(payload, castagnoliTable)
	var crcBuf [4]byte
	binary.LittleEndian.PutUint32(crcBuf[:], crc)

	fileData := append(payload, crcBuf[:]...)
	os.WriteFile(path, fileData, 0644)

	_, err := Deserialize(path)
	if err == nil {
		t.Fatal("expected error for v3 tile")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %v", err)
	}
}

// TestSectionAlignmentUint8Last verifies that uint8 sections (NodeTags, IsBorder)
// appear AFTER all multi-byte data sections and BEFORE landmark sections in the
// serialized file. This ordering is required for correct mmap alignment: odd-byte
// uint8 sections must not precede int32/uint32/uint64 sections.
func TestSectionAlignmentUint8Last(t *testing.T) {
	g := &Graph{
		NodeCount: 5, EdgeCount: 4,
		Offsets:         []uint32{0, 1, 2, 3, 4, 4},
		Targets:         []uint32{1, 2, 3, 4},
		EdgeDist:        []uint32{100, 200, 300, 400},
		EdgeTags1:       []uint64{0, 0, 0, 0},
		EdgeTags2:       []uint64{0, 0, 0, 0},
		EdgeOSMIdx:      []uint32{0, 0, 0, 0},
		OSMIDPool:       []int64{1, 2},
		NodeLat:         []int32{470000000, 470010000, 470020000, 470030000, 470040000},
		NodeLon:         []int32{80000000, 80000000, 80000000, 80000000, 80000000},
		NodeEle:         []int32{5000, 5100, 5200, 5300, 5400},
		NodeTags:        []uint8{0, 1, 0, 1, 0}, // 5 bytes — odd length to stress alignment
		EdgeGeomOff:     []uint32{0, 0, 0, 0, 0},
		EdgeGeomLat:     []int32{},
		EdgeGeomLon:     []int32{},
		EdgeElevOff:     []uint32{0, 0, 0, 0, 0},
		EdgeElevProf:    []int32{},
		EdgeElevSegDist: []uint16{},
		IsBorder:        []bool{false, true, false, false, true},
	}
	g.Grid = BuildSpatialIndex(g, 0.01)

	f, err := os.CreateTemp("", "uint8last_test_*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Close()

	if err := Serialize(g, f.Name()); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	sectionCnt := le.Uint16(data[60:62])
	dirStart := HeaderSize

	// Collect section types in file order.
	types := make([]uint16, sectionCnt)
	for i := range int(sectionCnt) {
		off := dirStart + i*12
		types[i] = le.Uint16(data[off : off+2])
	}

	uint8Sections := map[uint16]bool{SectionNodeTags: true, SectionIsBorder: true}
	landmarkBase := SectionLandmarkDistA

	// Find index ranges.
	firstUint8 := -1
	lastUint8 := -1
	firstLandmark := -1
	lastMultiByte := -1

	for i, typ := range types {
		if uint8Sections[typ] {
			if firstUint8 == -1 {
				firstUint8 = i
			}
			lastUint8 = i
		} else if typ >= landmarkBase {
			if firstLandmark == -1 {
				firstLandmark = i
			}
		} else {
			// non-uint8 data section
			lastMultiByte = i
		}
	}

	// uint8 sections must exist.
	if firstUint8 == -1 {
		t.Fatal("NodeTags section not found in serialized file")
	}

	// uint8 sections must come AFTER all multi-byte data sections.
	if lastMultiByte != -1 && firstUint8 <= lastMultiByte {
		t.Errorf("uint8 section appears at index %d but multi-byte section appears at index %d — uint8 must be last among data sections", firstUint8, lastMultiByte)
	}

	// Landmark sections (if present) must come AFTER uint8 sections.
	if firstLandmark != -1 && firstLandmark <= lastUint8 {
		t.Errorf("landmark section appears at index %d but uint8 section appears at index %d — landmarks must follow uint8 sections", firstLandmark, lastUint8)
	}
}
