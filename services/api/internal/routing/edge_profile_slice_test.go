// edge_profile_slice_test.go — Tests for SliceEdgeElevProf helper.
package routing

import (
	"fmt"
	"math"
	"testing"
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
