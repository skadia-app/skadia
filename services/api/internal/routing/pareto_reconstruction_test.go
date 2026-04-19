package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestReconstructPath_ShortChain(t *testing.T) {
	rs := newReachableState()

	rootID, _ := rs.tryInsert(1, paretoState{
		cost:          0,
		parentStateID: rootStateID,
	})
	midID, _ := rs.tryInsert(2, paretoState{
		cost:          100,
		handle:        uint64(1001),
		parentStateID: rootID,
	})
	targetID, _ := rs.tryInsert(3, paretoState{
		cost:          200,
		handle:        uint64(1002),
		parentStateID: midID,
	})

	edges, target, err := ReconstructPath(rs, targetID)
	if err != nil {
		t.Fatalf("ReconstructPath: %v", err)
	}
	if len(edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(edges))
	}
	if len(edges) >= 2 && (edges[0] != 1001 || edges[1] != 1002) {
		t.Errorf("edge order wrong: %v", edges)
	}
	if target == nil || target.cost != 200 {
		t.Errorf("target state wrong")
	}
}

func TestReconstructPath_OutOfRange(t *testing.T) {
	rs := newReachableState()
	_, _, err := ReconstructPath(rs, 99999)
	if err == nil {
		t.Error("expected error for out-of-range stateID")
	}
}

func TestReconstructPath_MaxHopsSafeguard(t *testing.T) {
	rs := newReachableState()
	// Build a cyclic parent chain (programmer error simulation)
	rs.slab.states = append(rs.slab.states, paretoState{parentStateID: 1})
	rs.slab.states = append(rs.slab.states, paretoState{parentStateID: 0})

	_, _, err := ReconstructPath(rs, 0)
	if err == nil {
		t.Error("expected error for cyclic chain")
	}
}

func TestComputeWayIDOverlap(t *testing.T) {
	a := map[int64]bool{1: true, 2: true, 3: true}
	b := map[int64]bool{2: true, 3: true, 4: true}
	got := ComputeWayIDOverlap(a, b)
	want := 2.0 / 3.0
	if got < want-0.001 || got > want+0.001 {
		t.Errorf("overlap = %v, want ~%v", got, want)
	}
}

func TestComputeWayIDOverlap_Empty(t *testing.T) {
	if ComputeWayIDOverlap(nil, map[int64]bool{1: true}) != 0 {
		t.Error("empty set should give 0 overlap")
	}
}

func TestComputeWayIDOverlap_Identical(t *testing.T) {
	a := map[int64]bool{1: true, 2: true}
	if ComputeWayIDOverlap(a, a) != 1.0 {
		t.Errorf("identical sets should give 1.0 overlap")
	}
}

// ----------------------------------------------------------------------------
// buildRouteReaderMock — minimal GraphReader for BuildRouteResultFromEdges tests.
// Tags1/Tags2 are looked up by edge handle (uint64); NodeCoord by nodeID.
// All other methods are no-ops to satisfy the interface.
// ----------------------------------------------------------------------------

type buildRouteReaderMock struct {
	// tags1[handle] and tags2[handle] for each edge handle.
	tags1 map[uint64]uint64
	tags2 map[uint64]uint64
	// coords[nodeID] = (lat×1e7, lon×1e7, ele×10) — raw int32 values.
	coords map[uint64][3]int32
}

func (m *buildRouteReaderMock) EdgeTags1(h uint64) uint64 { return m.tags1[h] }
func (m *buildRouteReaderMock) EdgeTags2(h uint64) uint64 { return m.tags2[h] }
func (m *buildRouteReaderMock) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	c := m.coords[nodeID]
	return c[0], c[1], c[2]
}

// No-op implementations for the rest of the interface.
func (m *buildRouteReaderMock) ForEachEdge(_ uint64, _ func(EdgeView))           {}
func (m *buildRouteReaderMock) ForEachInEdge(_ uint64, _ func(EdgeView))         {}
func (m *buildRouteReaderMock) NearestNode(_, _ float64) (uint64, float64, bool) { return 0, 0, false }
func (m *buildRouteReaderMock) EdgeElevProfile(_ uint64) ([]int32, []uint16)     { return nil, nil }
func (m *buildRouteReaderMock) EdgeGeometry(_ uint64) ([]int32, []int32)         { return nil, nil }
func (m *buildRouteReaderMock) EdgeOSMID(_ uint64) int64                         { return 0 }
func (m *buildRouteReaderMock) EdgeDistanceCm(_ uint64) uint32                   { return 0 }
func (m *buildRouteReaderMock) LandmarkDists(_ LandmarkSetID, _ uint64) []float32 {
	return nil
}
func (m *buildRouteReaderMock) LandmarkDistsInto(_ LandmarkSetID, _ uint64, _ []float32, _ []byte) []float32 {
	return nil
}
func (m *buildRouteReaderMock) LandmarkDistsPageInto(_ LandmarkSetID, _ uint32, _, _ uint32, _ []float32, _ []byte) []float32 {
	return nil
}
func (m *buildRouteReaderMock) LandmarkMeta() *graph.LandmarkMetaData { return nil }
func (m *buildRouteReaderMock) GraphAt(_, _ float64) *graph.Graph     { return nil }
func (m *buildRouteReaderMock) HasTilesFor(_, _, _, _ float64) bool   { return false }
func (m *buildRouteReaderMock) FindNearestEdge(_, _ float64, _ func(uint64) bool) (graph.EdgeProjection, bool) {
	return graph.EdgeProjection{}, false
}

// TestBuildRouteResultFromEdges_PopulatesSegments verifies that
// BuildRouteResultFromEdges now populates result.Segments (Opus R2-#6 fix).
// Uses three edges with distinct Tags1 values (highway=path, SAC T1/T2/T3) so
// each edge forms its own segment — asserting len(Segments)==3.
func TestBuildRouteResultFromEdges_PopulatesSegments(t *testing.T) {
	// highway=path (enum 16, bits 0-4), sac_scale encoded at bits 14-16.
	// sacScaleEnum: "hiking"=1, "mountain_hiking"=2, "demanding_mountain_hiking"=3
	pathHighway := uint64(16)
	tags1 := []uint64{
		pathHighway | (1 << 14), // highway=path, sac_scale=T1 (hiking)
		pathHighway | (2 << 14), // highway=path, sac_scale=T2 (mountain_hiking)
		pathHighway | (3 << 14), // highway=path, sac_scale=T3 (demanding_mountain_hiking)
	}
	// edge handles 101, 102, 103
	handles := []EdgeHandle{101, 102, 103}
	// four node IDs: 0,1,2,3
	nodeIDs := []uint64{0, 1, 2, 3}

	reader := &buildRouteReaderMock{
		tags1: map[uint64]uint64{101: tags1[0], 102: tags1[1], 103: tags1[2]},
		tags2: map[uint64]uint64{101: 0, 102: 0, 103: 0},
		coords: map[uint64][3]int32{
			0: {470000000, 110000000, 1000},
			1: {470100000, 110100000, 1010},
			2: {470200000, 110200000, 1020},
			3: {470300000, 110300000, 1030},
		},
	}

	target := &paretoState{dist: 300, ascent: 30, descent: 10, cost: 5}
	result := BuildRouteResultFromEdges(handles, nodeIDs, target, reader)

	// Result must have Segments populated — before the fix it was nil.
	if result.Segments == nil {
		t.Fatal("Segments is nil — BuildRouteResultFromEdges did not populate Segments (Opus R2-#6 regression)")
	}
	// 3 edges, each with distinct Tags1 → 3 segments.
	if len(result.Segments) != 3 {
		t.Errorf("expected 3 segments (one per distinct Tags1), got %d: %v", len(result.Segments), result.Segments)
	}

	// Verify SAC scale strings decoded correctly.
	wantSac := []string{"T1", "T2", "T3"}
	for i, seg := range result.Segments {
		if seg.SacScale != wantSac[i] {
			t.Errorf("segment[%d].SacScale = %q, want %q", i, seg.SacScale, wantSac[i])
		}
	}

	// Verify coordinate coverage: each segment's FromIdx must be < ToIdx (except
	// last which ends at lastCoordIdx), and ranges must be contiguous.
	nCoords := len(result.Coordinates)
	if nCoords == 0 {
		t.Fatal("Coordinates is empty — geometry not built")
	}
	last := result.Segments[len(result.Segments)-1]
	if last.ToIdx != nCoords-1 {
		t.Errorf("last segment ToIdx = %d, want %d (last coord index)", last.ToIdx, nCoords-1)
	}
	for i := 0; i < len(result.Segments)-1; i++ {
		if result.Segments[i].ToIdx != result.Segments[i+1].FromIdx {
			t.Errorf("segments[%d].ToIdx=%d != segments[%d].FromIdx=%d — gap or overlap",
				i, result.Segments[i].ToIdx, i+1, result.Segments[i+1].FromIdx)
		}
	}
}

// TestBuildRouteResultFromEdges_EmptyEdges_NoSegments verifies that an
// empty edge list produces no Segments (not a nil-dereference).
func TestBuildRouteResultFromEdges_EmptyEdges_NoSegments(t *testing.T) {
	reader := &buildRouteReaderMock{
		tags1:  map[uint64]uint64{},
		tags2:  map[uint64]uint64{},
		coords: map[uint64][3]int32{0: {470000000, 110000000, 1000}},
	}
	target := &paretoState{}
	result := BuildRouteResultFromEdges(nil, []uint64{0}, target, reader)

	if len(result.Segments) != 0 {
		t.Errorf("expected 0 segments for empty edge list, got %d", len(result.Segments))
	}
}
