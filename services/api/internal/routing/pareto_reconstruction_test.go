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
	// elevProfiles[handle] = (elevsDm, segDistsCm) — optional per-edge DEM data.
	elevProfiles map[uint64][2]interface{} // [0]=[]int32, [1]=[]uint16
	// edgeDistCm[handle] = edge length in cm — optional, defaults to 0.
	edgeDistCm map[uint64]uint32
	// edgeGeom[handle] = (lats×1e7, lons×1e7) — optional intermediate geometry.
	edgeGeom map[uint64][2][]int32
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
func (m *buildRouteReaderMock) EdgeElevProfile(h uint64) ([]int32, []uint16) {
	if m.elevProfiles == nil {
		return nil, nil
	}
	pair, ok := m.elevProfiles[h]
	if !ok {
		return nil, nil
	}
	elevs, _ := pair[0].([]int32)
	segs, _ := pair[1].([]uint16)
	return elevs, segs
}
func (m *buildRouteReaderMock) EdgeGeometry(h uint64) ([]int32, []int32) {
	if m.edgeGeom == nil {
		return nil, nil
	}
	g, ok := m.edgeGeom[h]
	if !ok {
		return nil, nil
	}
	return g[0], g[1]
}
func (m *buildRouteReaderMock) EdgeOSMID(_ uint64) int64 { return 0 }
func (m *buildRouteReaderMock) EdgeDistanceCm(h uint64) uint32 {
	if m.edgeDistCm == nil {
		return 0
	}
	return m.edgeDistCm[h]
}
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

// TestBuildRouteResultFromEdges_PopulatesEdgeSlices verifies that
// BuildRouteResultFromEdges populates result.EdgeCoordStart, EdgeTags1, EdgeTags2
// with length == len(EdgeHandles).
func TestBuildRouteResultFromEdges_PopulatesEdgeSlices(t *testing.T) {
	handles := []EdgeHandle{101, 102, 103}
	nodeIDs := []uint64{0, 1, 2, 3}
	reader := &buildRouteReaderMock{
		tags1: map[uint64]uint64{101: 11, 102: 22, 103: 33},
		tags2: map[uint64]uint64{101: 1, 102: 2, 103: 3},
		coords: map[uint64][3]int32{
			0: {470000000, 110000000, 1000},
			1: {470100000, 110100000, 1010},
			2: {470200000, 110200000, 1020},
			3: {470300000, 110300000, 1030},
		},
	}
	target := &paretoState{dist: 300, ascent: 30, descent: 10, cost: 5}
	result := BuildRouteResultFromEdges(handles, nodeIDs, target, reader)

	if got, want := len(result.EdgeCoordStart), len(result.EdgeHandles); got != want {
		t.Fatalf("EdgeCoordStart len = %d, want %d", got, want)
	}
	if got, want := len(result.EdgeTags1), len(result.EdgeHandles); got != want {
		t.Fatalf("EdgeTags1 len = %d, want %d", got, want)
	}
	if got, want := len(result.EdgeTags2), len(result.EdgeHandles); got != want {
		t.Fatalf("EdgeTags2 len = %d, want %d", got, want)
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

// TestBuildRouteResultFromEdges_NoSpike_NonMonotonicEdge verifies that
// BuildRouteResultFromEdges emits a non-nil ElevationProfile and that the
// profile values follow the raw DEM data without a spike. This tests the
// fix of the elevsDm[j]-as-DP-index bug (was: j iterates DP-simplified coords,
// elevsDm is raw-resolution → mismatched indexing → arbitrary elevation values).
//
// One edge with 4 raw elev points; segDists give cumulative positions.
// Expected: profile has 4 points; no adjacent pair has |ΔEle/ΔDist| > 5.0 m/m.
func TestBuildRouteResultFromEdges_NoSpike_NonMonotonicEdge(t *testing.T) {
	// Edge 201: elevs in dm [1000, 1050, 980, 1020], segDists in cm [5000, 4000, 3000].
	// Profile should not spike — max gradient across any adjacent pair: ~0.014 m/m.
	reader := &buildRouteReaderMock{
		tags1: map[uint64]uint64{201: 0},
		tags2: map[uint64]uint64{201: 0},
		coords: map[uint64][3]int32{
			10: {470000000, 110000000, 500},  // node 10: ele 50m (500 dm)
			11: {470100000, 110150000, 600},  // node 11: ele 60m (600 dm)
		},
		elevProfiles: map[uint64][2]interface{}{
			201: {[]int32{1000, 1050, 980, 1020}, []uint16{5000, 4000, 3000}},
		},
		edgeDistCm: map[uint64]uint32{201: 12000},
	}

	handles := []EdgeHandle{201}
	nodeIDs := []uint64{10, 11}
	target := &paretoState{dist: 120, ascent: 7, descent: 3, cost: 1}
	result := BuildRouteResultFromEdges(handles, nodeIDs, target, reader)

	// Profile must be non-nil and non-empty.
	if result.ElevationProfile == nil {
		t.Fatal("ElevationProfile is nil — buildElevationProfile not called")
	}
	prof := *result.ElevationProfile
	if len(prof) == 0 {
		t.Fatal("ElevationProfile is empty")
	}

	// Check that no adjacent pair exceeds 5 m/m gradient — guards against DP-index spike.
	const maxGradient = 5.0
	for i := 1; i < len(prof); i++ {
		distDelta := prof[i].DistM - prof[i-1].DistM
		if distDelta <= 0 {
			continue // monotonic guard; skip zero-span
		}
		if prof[i].EleM == nil || prof[i-1].EleM == nil {
			continue // nil samples skipped; phase-D will resolve
		}
		eleDelta := *prof[i].EleM - *prof[i-1].EleM
		if eleDelta < 0 {
			eleDelta = -eleDelta
		}
		gradient := eleDelta / distDelta
		if gradient > maxGradient {
			t.Errorf("profile[%d→%d]: gradient %.3f m/m exceeds %.1f m/m — likely DP-index bug",
				i-1, i, gradient, maxGradient)
		}
	}

	// Profile values should be close to the raw DEM values (dm→m: 100.0, 105.0, 98.0, 102.0).
	wantEleM := []float64{100.0, 105.0, 98.0, 102.0}
	if len(prof) != len(wantEleM) {
		t.Fatalf("profile len = %d, want %d", len(prof), len(wantEleM))
	}
	for i, want := range wantEleM {
		if prof[i].EleM == nil {
			t.Fatalf("prof[%d].EleM is nil", i)
		}
		assertInDelta(t, want, *prof[i].EleM, 0.01, "prof["+itoa(i)+"].EleM")
	}
}

// TestBuildRouteResultFromEdges_CoordsConsistentWithProfile verifies that
// coord[i].Ele after BuildRouteResultFromEdges reflects applyProfileToCoords
// rather than the raw NodeCoord placeholder.
//
// Spec Rev 2.3 §6.5 / F1: after applyProfileToCoords, coord[i].Ele must
// equal InterpolateProfileEleAt(profile, cumDist(i)).
//
// Setup: single edge handle 301; node coords have ele=5m (50 dm); edge DEM
// has [1000dm, 2000dm] = [100m, 200m]; segDist 10000cm = 100m.
// Expected: first coord Ele ≈ 100.0m; last coord Ele ≈ 200.0m.
// (Proves NodeCoord-derived 5.0m placeholder was overwritten by profile.)
func TestBuildRouteResultFromEdges_CoordsConsistentWithProfile(t *testing.T) {
	// node 20: lat 47.0, lon 11.0, ele 5m (50 dm). Placeholder, not profile.
	// node 21: lat 47.01, lon 11.015, ele 5m (50 dm). Same placeholder.
	reader := &buildRouteReaderMock{
		tags1: map[uint64]uint64{301: 0},
		tags2: map[uint64]uint64{301: 0},
		coords: map[uint64][3]int32{
			20: {470000000, 110000000, 50},
			21: {470100000, 110150000, 50},
		},
		elevProfiles: map[uint64][2]interface{}{
			301: {[]int32{1000, 2000}, []uint16{10000}},
		},
		edgeDistCm: map[uint64]uint32{301: 10000},
	}

	handles := []EdgeHandle{301}
	nodeIDs := []uint64{20, 21}
	target := &paretoState{dist: 100, ascent: 100, descent: 0, cost: 1}
	result := BuildRouteResultFromEdges(handles, nodeIDs, target, reader)

	// Profile must be present.
	if result.ElevationProfile == nil || len(*result.ElevationProfile) == 0 {
		t.Fatal("ElevationProfile is nil or empty")
	}

	// Must have at least 2 coordinates (one per node).
	if len(result.Coordinates) < 2 {
		t.Fatalf("expected at least 2 coordinates, got %d", len(result.Coordinates))
	}

	// First coord Ele must NOT be the NodeCoord placeholder (5.0m) but ≈100.0m.
	firstEle := result.Coordinates[0].Ele
	assertInDelta(t, 100.0, firstEle, 1.0, "Coordinates[0].Ele (should be ~100m from profile, not 5m NodeCoord)")
	if firstEle < 10.0 {
		t.Errorf("Coordinates[0].Ele = %v — looks like NodeCoord placeholder was NOT overwritten", firstEle)
	}

	// Last coord Ele must be ≈200.0m (profile end).
	lastEle := result.Coordinates[len(result.Coordinates)-1].Ele
	assertInDelta(t, 200.0, lastEle, 1.0, "Coordinates[last].Ele (should be ~200m from profile)")
}

// itoa is a minimal int-to-string helper for test messages (avoids fmt import).
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := make([]byte, 0, 10)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
