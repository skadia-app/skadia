// ve_tail_profile_test.go — Unit tests for buildVeTailProfile,
// reverseElevSlice, concatProfilesWithJunctionDedup. These exercise the
// Phase C.2 VE-tail elevation logic that retrackUnified now uses to feed
// applyProfileToCoords across the full route (instead of only the network
// portion).
package routing

import (
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/dem"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// fakeVeTailReader satisfies veTailProfileReader. Only EdgeElevProfile is
// required by buildVeTailProfile; we mock just that.
type fakeVeTailReader struct {
	elevs      []int32
	segDistsCm []uint16
}

func (r *fakeVeTailReader) EdgeElevProfile(_ uint64) ([]int32, []uint16) {
	return r.elevs, r.segDistsCm
}

// makeVE constructs a VirtualEndpoint for the helper tests.
func makeVE(globalA, globalB uint64, fractionAB float64) *VirtualEndpoint {
	return &VirtualEndpoint{
		GlobalA: globalA,
		GlobalB: globalB,
		Proj: graph.EdgeProjection{
			EdgeHandle: 1,
			FractionAB: fractionAB,
		},
	}
}

func ptrEqualOrBothNil(a, b *float64, eps float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return math.Abs(*a-*b) < eps
}

// --- reverseElevSlice ---

func TestReverseElevSlice_ReBasesDistFromZero(t *testing.T) {
	in := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 50, EleM: PtrEle(110)},
		{DistM: 120, EleM: PtrEle(130)},
	}
	got := reverseElevSlice(in)
	want := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(130)},
		{DistM: 70, EleM: PtrEle(110)},
		{DistM: 120, EleM: PtrEle(100)},
	}
	if len(got) != len(want) {
		t.Fatalf("len: want %d, got %d", len(want), len(got))
	}
	for i := range want {
		if math.Abs(got[i].DistM-want[i].DistM) > 1e-6 {
			t.Errorf("[%d].DistM: want %v, got %v", i, want[i].DistM, got[i].DistM)
		}
		if !ptrEqualOrBothNil(got[i].EleM, want[i].EleM, 1e-6) {
			t.Errorf("[%d].EleM mismatch", i)
		}
	}
}

func TestReverseElevSlice_PropagatesNil(t *testing.T) {
	in := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 30, EleM: nil},
		{DistM: 60, EleM: PtrEle(120)},
	}
	got := reverseElevSlice(in)
	if got[0].EleM == nil || *got[0].EleM != 120 {
		t.Errorf("got[0].EleM: want 120, got %v", got[0].EleM)
	}
	if got[1].EleM != nil {
		t.Errorf("got[1].EleM: want nil, got %v", got[1].EleM)
	}
	if got[2].EleM == nil || *got[2].EleM != 100 {
		t.Errorf("got[2].EleM: want 100, got %v", got[2].EleM)
	}
}

// --- concatProfilesWithJunctionDedup ---

func TestConcatProfilesWithJunctionDedup_ThreeParts(t *testing.T) {
	// src tail: 0..50m, ele 100..110
	// network: 0..200m, ele 110..130 (first point junction with src tail's last)
	// dst tail: 0..40m, ele 130..125 (first point junction with network's last)
	src := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 50, EleM: PtrEle(110)},
	}
	net := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(110)},
		{DistM: 100, EleM: PtrEle(120)},
		{DistM: 200, EleM: PtrEle(130)},
	}
	dst := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(130)},
		{DistM: 40, EleM: PtrEle(125)},
	}
	got := concatProfilesWithJunctionDedup(src, net, dst)
	// Expected: 2 + (3-1) + (2-1) = 5 points
	if len(got) != 5 {
		t.Fatalf("len: want 5, got %d", len(got))
	}
	wantDist := []float64{0, 50, 150, 250, 290}
	wantEle := []float64{100, 110, 120, 130, 125}
	for i := range got {
		if math.Abs(got[i].DistM-wantDist[i]) > 1e-6 {
			t.Errorf("[%d].DistM: want %v, got %v", i, wantDist[i], got[i].DistM)
		}
		if got[i].EleM == nil || math.Abs(*got[i].EleM-wantEle[i]) > 1e-6 {
			t.Errorf("[%d].EleM: want %v, got %v", i, wantEle[i], got[i].EleM)
		}
	}
}

func TestConcatProfilesWithJunctionDedup_SkipsEmpty(t *testing.T) {
	net := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(120)},
	}
	got := concatProfilesWithJunctionDedup(nil, net, nil)
	if len(got) != 2 {
		t.Fatalf("len: want 2, got %d", len(got))
	}
	if got[0].DistM != 0 || got[1].DistM != 100 {
		t.Errorf("dist axis: want [0, 100], got [%v, %v]", got[0].DistM, got[1].DistM)
	}
}

func TestConcatProfilesWithJunctionDedup_AllEmpty(t *testing.T) {
	if got := concatProfilesWithJunctionDedup(nil, nil, nil); got != nil {
		t.Errorf("want nil, got %v", got)
	}
}

func TestConcatProfilesWithJunctionDedup_SinglePointTail_NotEatenByDedup(t *testing.T) {
	// Edge case: src tail has only 1 point (degenerate fraction). The first
	// non-empty part is emitted in full — dedup only applies to subsequent
	// parts. So a 1-point src tail contributes its single sample.
	src := []ElevationPoint{{DistM: 0, EleM: PtrEle(100)}}
	net := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(105)},
		{DistM: 50, EleM: PtrEle(115)},
	}
	got := concatProfilesWithJunctionDedup(src, net, nil)
	// First part emitted in full (1 point), second part minus its first → 1 point.
	if len(got) != 2 {
		t.Fatalf("len: want 2, got %d", len(got))
	}
	// distAccum after src = 0 (its last DistM); net[1] shifted to 0+50=50.
	if got[0].DistM != 0 || *got[0].EleM != 100 {
		t.Errorf("[0]: want {0, 100}, got {%v, %v}", got[0].DistM, *got[0].EleM)
	}
	if math.Abs(got[1].DistM-50) > 1e-6 || *got[1].EleM != 115 {
		t.Errorf("[1]: want {50, 115}, got {%v, %v}", got[1].DistM, *got[1].EleM)
	}
}

// --- buildVeTailProfile ---

// Reference profile: 4 segments, 200m total. Distinct elevations at every
// recorded sample so a slice in the middle is verifiably DEM-accurate (not
// linearly interpolated NodeA↔NodeB).
//
//	dist (m):    0    50   100   150   200
//	ele  (dm):  1000 1050 1150 1250 1300
//	ele  (m):   100  105  115  125  130
func makeRefReader() *fakeVeTailReader {
	return &fakeVeTailReader{
		elevs:      []int32{1000, 1050, 1150, 1250, 1300},
		segDistsCm: []uint16{5000, 5000, 5000, 5000},
	}
}

// Source side, seedIsB (proj→B, natural direction).
// VE at fraction 0.25 (=50m). Slice [0.25, 1.0] yields 4 points:
// {0, 105}, {50, 115}, {100, 125}, {150, 130}.
func TestBuildVeTailProfile_SrcSeedB_NaturalDirection(t *testing.T) {
	r := makeRefReader()
	ve := makeVE(10, 20, 0.25)
	got := buildVeTailProfile(r, ve, 20 /*=globalB*/, true)
	wantDist := []float64{0, 50, 100, 150}
	wantEle := []float64{105, 115, 125, 130}
	checkProfile(t, got, wantDist, wantEle)
}

// Source side, seedIsA (proj→A, reversed). VE at fraction 0.25.
// Slice [0, 0.25] yields {0, 100}, {50, 105}; reversed → {0, 105}, {50, 100}.
func TestBuildVeTailProfile_SrcSeedA_Reversed(t *testing.T) {
	r := makeRefReader()
	ve := makeVE(10, 20, 0.25)
	got := buildVeTailProfile(r, ve, 10 /*=globalA*/, true)
	wantDist := []float64{0, 50}
	wantEle := []float64{105, 100}
	checkProfile(t, got, wantDist, wantEle)
}

// Destination side, seedIsA (A→proj, natural). VE at fraction 0.25.
// Slice [0, 0.25] → {0, 100}, {50, 105}.
func TestBuildVeTailProfile_DstSeedA_NaturalDirection(t *testing.T) {
	r := makeRefReader()
	ve := makeVE(10, 20, 0.25)
	got := buildVeTailProfile(r, ve, 10, false)
	wantDist := []float64{0, 50}
	wantEle := []float64{100, 105}
	checkProfile(t, got, wantDist, wantEle)
}

// Destination side, seedIsB (B→proj, reversed). VE at fraction 0.25.
// Slice [0.25, 1.0] → {0, 105}, {50, 115}, {100, 125}, {150, 130};
// reversed (re-base from last DistM=150) → {0, 130}, {50, 125}, {100, 115}, {150, 105}.
func TestBuildVeTailProfile_DstSeedB_Reversed(t *testing.T) {
	r := makeRefReader()
	ve := makeVE(10, 20, 0.25)
	got := buildVeTailProfile(r, ve, 20, false)
	wantDist := []float64{0, 50, 100, 150}
	wantEle := []float64{130, 125, 115, 105}
	checkProfile(t, got, wantDist, wantEle)
}

// Critical regression test: the elevation at fraction 0.25 must be the slice's
// linear interpolation (105m), NOT a NodeA↔NodeB linear interpolation across
// the full edge (100 + 0.25*(130-100) = 107.5m). This is the whole point of
// Phase C.2 — replace the linear NodeA↔NodeB interpolation with the
// high-resolution slice.
func TestBuildVeTailProfile_DemAccurate_NotLinearNodeAB(t *testing.T) {
	// Profile: 100..130m over 200m, but with a non-linear shape.
	// NodeA=100, NodeB=130 → linear at f=0.25 would be 107.5.
	// Reference shape ele[1]=105 at 50m → at exactly 50m we read 105, not 107.5.
	r := makeRefReader()
	ve := makeVE(10, 20, 0.25)
	got := buildVeTailProfile(r, ve, 20, true) // src, seedIsB
	if len(got) == 0 {
		t.Fatal("empty profile")
	}
	// got[0] is at the projection point (DistM=0), elevation = profile sample at fraction 0.25 = 50m = 105m.
	if got[0].EleM == nil {
		t.Fatal("got[0].EleM is nil")
	}
	if math.Abs(*got[0].EleM-105.0) > 1e-6 {
		t.Errorf("DEM-accurate slice expected 105m at projection, got %v (would be 107.5m for linear NodeA-NodeB)", *got[0].EleM)
	}
	if math.Abs(*got[0].EleM-107.5) < 0.5 {
		t.Errorf("regression: VE-tail elevation looks like NodeA-NodeB linear interpolation (%v), not DEM-accurate slice", *got[0].EleM)
	}
}

func TestBuildVeTailProfile_NoDataPropagatesNil(t *testing.T) {
	r := &fakeVeTailReader{
		elevs:      []int32{1000, dem.NoData, 1150, 1250, 1300},
		segDistsCm: []uint16{5000, 5000, 5000, 5000},
	}
	ve := makeVE(10, 20, 0.10) // fraction 0.10 → projection at 20m, between samples 0 (0m, 100) and 1 (50m, NoData).
	got := buildVeTailProfile(r, ve, 20, true)
	if len(got) == 0 {
		t.Fatal("empty profile")
	}
	// The interpolated start point at distance 20m falls between sample[0]
	// (valid) and sample[1] (NoData). NoData propagation rule in
	// SliceEdgeElevProf returns nil EleM in this case.
	if got[0].EleM != nil {
		t.Errorf("got[0].EleM: want nil (NoData propagation), got %v", *got[0].EleM)
	}
}

func TestBuildVeTailProfile_EmptyEdge_ReturnsNil(t *testing.T) {
	r := &fakeVeTailReader{} // no elevs
	ve := makeVE(10, 20, 0.5)
	if got := buildVeTailProfile(r, ve, 20, true); got != nil {
		t.Errorf("want nil for empty EdgeElevProfile, got %v", got)
	}
}

func TestBuildVeTailProfile_SinglePointEdge(t *testing.T) {
	r := &fakeVeTailReader{
		elevs:      []int32{1234},
		segDistsCm: nil,
	}
	ve := makeVE(10, 20, 0.5)
	got := buildVeTailProfile(r, ve, 20, true)
	if len(got) != 1 {
		t.Fatalf("want 1 point, got %d", len(got))
	}
	if got[0].EleM == nil || *got[0].EleM != 123.4 {
		t.Errorf("want 123.4, got %v", got[0].EleM)
	}
}

func TestBuildVeTailProfile_NilVE_ReturnsNil(t *testing.T) {
	if got := buildVeTailProfile(&fakeVeTailReader{}, nil, 0, true); got != nil {
		t.Errorf("want nil for nil VE, got %v", got)
	}
}

// --- helpers ---

func checkProfile(t *testing.T, got []ElevationPoint, wantDist, wantEle []float64) {
	t.Helper()
	if len(got) != len(wantDist) {
		t.Fatalf("profile len: want %d, got %d (%+v)", len(wantDist), len(got), got)
	}
	for i := range wantDist {
		if math.Abs(got[i].DistM-wantDist[i]) > 1e-3 {
			t.Errorf("[%d].DistM: want %v, got %v", i, wantDist[i], got[i].DistM)
		}
		if got[i].EleM == nil {
			t.Errorf("[%d].EleM: want %v, got nil", i, wantEle[i])
			continue
		}
		if math.Abs(*got[i].EleM-wantEle[i]) > 1e-3 {
			t.Errorf("[%d].EleM: want %v, got %v", i, wantEle[i], *got[i].EleM)
		}
	}
}
