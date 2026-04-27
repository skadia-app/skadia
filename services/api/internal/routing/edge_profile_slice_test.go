// edge_profile_slice_test.go — Tests for SliceEdgeElevProf helper and the
// unified sliceEdgeCoordsAndProfile helper used by routeSameEdge (T0 C.4).
package routing

import (
	"fmt"
	"math"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/geo"
)

func TestSliceEdgeElevProf_FullEdge(t *testing.T) {
	prof := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(110)},
		{DistM: 200, EleM: PtrEle(120)},
		{DistM: 300, EleM: PtrEle(130)},
	}
	got := SliceEdgeElevProf(prof, 0.0, 1.0)
	if len(got) != len(prof) {
		t.Fatalf("full-edge slice: want %d points, got %d", len(prof), len(got))
	}
	for i := range got {
		if math.Abs(got[i].DistM-prof[i].DistM) > 0.001 {
			t.Errorf("point[%d] DistM: want %.3f, got %.3f", i, prof[i].DistM, got[i].DistM)
		}
		if got[i].EleM == nil || prof[i].EleM == nil {
			t.Errorf("point[%d] EleM is nil", i)
			continue
		}
		if math.Abs(*got[i].EleM-*prof[i].EleM) > 0.001 {
			t.Errorf("point[%d] EleM: want %.3f, got %.3f", i, *prof[i].EleM, *got[i].EleM)
		}
	}
}

func TestSliceEdgeElevProf_PartialEdge_InterpolatedEndpoints(t *testing.T) {
	// 300m edge with 4 profile points: dist 0/100/200/300, ele 100/110/120/130.
	// Slice 25%-75% = dist [75, 225].
	// Interpolated start at dist 75 (between p0 and p1): ele = 100 + (75-0)/(100-0)*(110-100) = 107.5
	// Interior points: dist 100 → offset 100-75=25, ele 110; dist 200 → offset 200-75=125, ele 120.
	// Interpolated end at dist 225 (between p2 and p3): ele = 120 + (225-200)/(300-200)*(130-120) = 122.5
	// Output dist axis resets to 0: [0, 25, 125, 150].
	prof := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(110)},
		{DistM: 200, EleM: PtrEle(120)},
		{DistM: 300, EleM: PtrEle(130)},
	}
	got := SliceEdgeElevProf(prof, 0.25, 0.75)
	// Expect 4 points: interpolated start, p1 offset, p2 offset, interpolated end.
	if len(got) != 4 {
		t.Fatalf("partial-edge slice: want 4 points, got %d: %v", len(got), got)
	}
	if math.Abs(got[0].DistM-0) > 0.001 {
		t.Errorf("start DistM: want 0, got %.3f", got[0].DistM)
	}
	if got[0].EleM == nil || math.Abs(*got[0].EleM-107.5) > 0.01 {
		ele := "<nil>"
		if got[0].EleM != nil {
			ele = fmt.Sprintf("%.3f", *got[0].EleM)
		}
		t.Errorf("start EleM: want 107.5, got %s", ele)
	}
	if math.Abs(got[1].DistM-25) > 0.001 {
		t.Errorf("point[1] DistM: want 25, got %.3f", got[1].DistM)
	}
	if got[1].EleM == nil || math.Abs(*got[1].EleM-110) > 0.01 {
		t.Errorf("point[1] EleM: want 110, got %v", got[1].EleM)
	}
	if math.Abs(got[2].DistM-125) > 0.001 {
		t.Errorf("point[2] DistM: want 125, got %.3f", got[2].DistM)
	}
	if got[2].EleM == nil || math.Abs(*got[2].EleM-120) > 0.01 {
		t.Errorf("point[2] EleM: want 120, got %v", got[2].EleM)
	}
	if math.Abs(got[3].DistM-150) > 0.001 {
		t.Errorf("end DistM: want 150, got %.3f", got[3].DistM)
	}
	if got[3].EleM == nil || math.Abs(*got[3].EleM-122.5) > 0.01 {
		ele := "<nil>"
		if got[3].EleM != nil {
			ele = fmt.Sprintf("%.3f", *got[3].EleM)
		}
		t.Errorf("end EleM: want 122.5, got %s", ele)
	}
}

func TestSliceEdgeElevProf_DegenerateRange(t *testing.T) {
	// fStart == fEnd: single interpolated point at that fraction, DistM must be 0.
	prof := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(110)},
	}
	got := SliceEdgeElevProf(prof, 0.5, 0.5)
	if len(got) != 1 {
		t.Fatalf("degenerate range: want 1 point, got %d", len(got))
	}
	if math.Abs(got[0].DistM-0) > 0.001 {
		t.Errorf("degenerate DistM: want 0, got %.3f", got[0].DistM)
	}
	// ele at dist 50 on [0..100 → 100..110]: 105.0
	if got[0].EleM == nil || math.Abs(*got[0].EleM-105.0) > 0.01 {
		t.Errorf("degenerate EleM: want 105.0, got %v", got[0].EleM)
	}
}

func TestSliceEdgeElevProf_ClampsBounds(t *testing.T) {
	// fStart < 0 and fEnd > 1 must be clamped to [0, 1].
	prof := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(110)},
	}
	got := SliceEdgeElevProf(prof, -0.1, 1.5)
	full := SliceEdgeElevProf(prof, 0.0, 1.0)
	if len(got) != len(full) {
		t.Fatalf("clamped slice: want %d points like full-edge, got %d", len(full), len(got))
	}
	for i := range got {
		if math.Abs(got[i].DistM-full[i].DistM) > 0.001 {
			t.Errorf("clamped point[%d] DistM: want %.3f, got %.3f", i, full[i].DistM, got[i].DistM)
		}
	}
}

func TestSliceEdgeElevProf_NilProfile(t *testing.T) {
	got := SliceEdgeElevProf(nil, 0.0, 1.0)
	if got != nil {
		t.Errorf("nil input: want nil output, got %v", got)
	}
}

func TestSliceEdgeElevProf_NilEleHandling(t *testing.T) {
	// A profile with a nil EleM mid-edge. The interpolated endpoint that spans
	// a nil segment must also yield nil (NoData propagation).
	// Interior nil points pass through as nil.
	prof := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: nil},   // no DEM data
		{DistM: 200, EleM: PtrEle(120)},
	}
	// Slice 0-1 (full): nil mid-point must be preserved.
	got := SliceEdgeElevProf(prof, 0.0, 1.0)
	if len(got) < 3 {
		t.Fatalf("nil-ele full slice: want at least 3 points, got %d", len(got))
	}
	// The nil mid-point must survive.
	nilFound := false
	for _, p := range got {
		if p.EleM == nil {
			nilFound = true
		}
	}
	if !nilFound {
		t.Error("nil EleM mid-point must be preserved through full-edge slice")
	}

	// Slice that requires interpolation across a nil segment:
	// fStart=0.25 falls between p0(dist=0,ele=100) and p1(dist=100,ele=nil).
	// interpEle at dist=50 must return nil because one endpoint has nil EleM.
	got2 := SliceEdgeElevProf(prof, 0.25, 0.75)
	if len(got2) == 0 {
		t.Fatal("nil-ele partial slice: got empty result")
	}
	if got2[0].EleM != nil {
		t.Errorf("interpolated start across nil segment: want nil EleM, got %.3f", *got2[0].EleM)
	}
}

// --- sliceEdgeCoordsAndProfile (T0 C.4) ---

// fiveVertexEdge builds a 5-vertex coord polyline along a meridian and a
// matching 5-point elevation profile with monotonic ascent. The profile's
// distance axis is the haversine cumulative axis of the coords (built fresh
// here so tests don't have to hard-code it).
func fiveVertexEdge() (coords [][2]int32, prof []ElevationPoint) {
	// Five coords at 47.000, 47.010, 47.020, 47.030, 47.040 along lon=11.0.
	coords = [][2]int32{
		{470000000, 110000000},
		{470100000, 110000000},
		{470200000, 110000000},
		{470300000, 110000000},
		{470400000, 110000000},
	}
	// Build the profile dist axis from the same haversine cumsum so that
	// fraction-sampling lines up with coord-fraction sampling — keeps the
	// test assertions intuitive (profile-ele matches coord-fraction directly).
	dists := []float64{0}
	for i := 1; i < len(coords); i++ {
		dists = append(dists, dists[i-1]+geo.DistanceHaversine(
			float64(coords[i-1][0])/1e7, float64(coords[i-1][1])/1e7,
			float64(coords[i][0])/1e7, float64(coords[i][1])/1e7,
		))
	}
	eles := []float64{100, 110, 120, 130, 140}
	prof = make([]ElevationPoint, len(coords))
	for i := range coords {
		prof[i] = ElevationPoint{DistM: dists[i], EleM: PtrEle(eles[i])}
	}
	return coords, prof
}

func TestSliceEdgeCoordsAndProfile_LengthMatch_FullEdge(t *testing.T) {
	coords, prof := fiveVertexEdge()
	c, p := sliceEdgeCoordsAndProfile(coords, prof, 0, 1)
	if len(c) != len(p) {
		t.Fatalf("len mismatch: coords=%d profile=%d", len(c), len(p))
	}
	if len(c) != 5 {
		t.Fatalf("full slice: want 5 unified vertices, got %d", len(c))
	}
	// First and last DistM must be 0 and total respectively.
	if p[0].DistM != 0 {
		t.Errorf("p[0].DistM: want 0, got %.3f", p[0].DistM)
	}
	expectTotal := geo.DistanceHaversine(47.0, 11.0, 47.04, 11.0)
	if math.Abs(p[len(p)-1].DistM-expectTotal) > 1.0 {
		t.Errorf("p[last].DistM: want ~%.3f, got %.3f", expectTotal, p[len(p)-1].DistM)
	}
	// All elevations populated.
	for i, pt := range p {
		if pt.EleM == nil {
			t.Errorf("p[%d].EleM is nil — full slice should populate every vertex", i)
		}
		if !c[i].EleValid {
			t.Errorf("c[%d].EleValid is false — should be true when EleM != nil", i)
		}
	}
}

func TestSliceEdgeCoordsAndProfile_LengthMatch_PartialEdge(t *testing.T) {
	coords, prof := fiveVertexEdge()
	// 0.25..0.75 — fraction range covers the 3 interior vertices (frac=0.25,
	// 0.50, 0.75 — wait: 5 coords have fractions ~0, 0.25, 0.50, 0.75, 1.0).
	// So fStart=0.25 hits coord[1] exactly (excluded by strict-interior rule),
	// fEnd=0.75 hits coord[3] exactly (also excluded). Interior strict =
	// only coord[2] at frac=0.5. With endpoints: 3 unified vertices.
	c, p := sliceEdgeCoordsAndProfile(coords, prof, 0.25, 0.75)
	if len(c) != len(p) {
		t.Fatalf("len mismatch: coords=%d profile=%d", len(c), len(p))
	}
	if len(c) < 3 {
		t.Fatalf("partial slice: want >=3 unified vertices (start, interior, end), got %d", len(c))
	}
	if p[0].DistM != 0 {
		t.Errorf("p[0].DistM: want 0, got %.3f", p[0].DistM)
	}
	if p[len(p)-1].DistM <= 0 {
		t.Errorf("p[last].DistM: want >0, got %.3f", p[len(p)-1].DistM)
	}
}

func TestSliceEdgeCoordsAndProfile_DistAxisHaversineConsistent(t *testing.T) {
	coords, prof := fiveVertexEdge()
	c, p := sliceEdgeCoordsAndProfile(coords, prof, 0.10, 0.90)

	// profile[i].DistM ≈ Σ haversine(coords[0..i]) within ±1m.
	cum := 0.0
	for i := 1; i < len(c); i++ {
		cum += geo.DistanceHaversine(c[i-1].Lat, c[i-1].Lon, c[i].Lat, c[i].Lon)
		if math.Abs(cum-p[i].DistM) > 1.0 {
			t.Errorf("p[%d].DistM=%.3f vs haversine cumsum %.3f (diff > 1m)", i, p[i].DistM, cum)
		}
	}
}

func TestSliceEdgeCoordsAndProfile_NilEleM_PropagatesAtVertex(t *testing.T) {
	coords, prof := fiveVertexEdge()
	// Knock out the middle profile sample.
	prof[2].EleM = nil

	c, p := sliceEdgeCoordsAndProfile(coords, prof, 0, 1)
	if len(c) != len(p) {
		t.Fatalf("len mismatch: coords=%d profile=%d", len(c), len(p))
	}
	// The vertex at fraction ≈0.5 must have nil EleM (NoData propagation).
	// All coords have coord-fraction ≈ profile-fraction, so the middle vertex
	// samples prof[2] which is nil.
	foundNil := false
	for i, pt := range p {
		if pt.EleM == nil {
			foundNil = true
			if c[i].EleValid {
				t.Errorf("c[%d].EleValid must be false when EleM is nil", i)
			}
			if c[i].Ele != 0 {
				t.Errorf("c[%d].Ele must be 0 when EleM is nil, got %.3f", i, c[i].Ele)
			}
		}
	}
	if !foundNil {
		t.Error("expected at least one nil-EleM unified vertex (NoData propagation)")
	}
}

func TestSliceEdgeCoordsAndProfile_DegenerateRange(t *testing.T) {
	coords, prof := fiveVertexEdge()
	c, p := sliceEdgeCoordsAndProfile(coords, prof, 0.5, 0.5)
	if len(c) != 1 || len(p) != 1 {
		t.Fatalf("degenerate range: want 1/1, got %d/%d", len(c), len(p))
	}
	if p[0].DistM != 0 {
		t.Errorf("degenerate p[0].DistM: want 0, got %.3f", p[0].DistM)
	}
	if p[0].EleM == nil || math.Abs(*p[0].EleM-120) > 1.0 {
		ele := "<nil>"
		if p[0].EleM != nil {
			ele = fmt.Sprintf("%.3f", *p[0].EleM)
		}
		t.Errorf("degenerate p[0].EleM: want ~120, got %s", ele)
	}
}

func TestSliceEdgeCoordsAndProfile_EmptyInput(t *testing.T) {
	c, p := sliceEdgeCoordsAndProfile(nil, nil, 0, 1)
	if c != nil || p != nil {
		t.Errorf("empty input: want nil/nil, got %v / %v", c, p)
	}
}

func TestSliceEdgeCoordsAndProfile_SwappedRangeNormalised(t *testing.T) {
	coords, prof := fiveVertexEdge()
	a, b := sliceEdgeCoordsAndProfile(coords, prof, 0.7, 0.3)
	c, d := sliceEdgeCoordsAndProfile(coords, prof, 0.3, 0.7)
	if len(a) != len(c) {
		t.Fatalf("swapped range produced different output length: %d vs %d", len(a), len(c))
	}
	for i := range a {
		if math.Abs(a[i].Lat-c[i].Lat) > 1e-9 || math.Abs(b[i].DistM-d[i].DistM) > 1e-6 {
			t.Errorf("swapped vs natural mismatch at %d", i)
		}
	}
}
