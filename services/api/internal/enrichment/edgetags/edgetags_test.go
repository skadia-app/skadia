package edgetags

import (
	"math"
	"testing"
)

func TestPackageConstants(t *testing.T) {
	if ForestHighlightMin != 60 {
		t.Fatalf("ForestHighlightMin: got %d, want 60", ForestHighlightMin)
	}
	if ProtectedHighlightMin != 1 {
		t.Fatalf("ProtectedHighlightMin: got %d, want 1", ProtectedHighlightMin)
	}
}

func TestContiguousRangesFromMatch_Basic(t *testing.T) {
	matches := []bool{true, true, false, true, true}
	edgeCoordStart := []int{0, 5, 10, 15, 20}
	lastCoordIdx := 24

	got := ContiguousRangesFromMatch(matches, edgeCoordStart, lastCoordIdx)
	want := []CoordRange{{From: 0, To: 10}, {From: 15, To: 25}}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("range %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestContiguousRangesFromMatch_Empty(t *testing.T) {
	if got := ContiguousRangesFromMatch(nil, nil, 0); got != nil {
		t.Errorf("nil input: got %+v, want nil", got)
	}
}

func TestLastRealCoordIdx_WithEnd(t *testing.T) {
	if got := LastRealCoordIdx(10, 7); got != 6 {
		t.Errorf("got %d, want 6", got)
	}
}

func TestLastRealCoordIdx_NoEnd(t *testing.T) {
	if got := LastRealCoordIdx(10, 0); got != 9 {
		t.Errorf("got %d, want 9", got)
	}
}

func TestComputeAllRanges_SingleSacEdge(t *testing.T) {
	tags1 := uint64(2) << 14
	tags2 := uint64(80)
	got := ComputeAllRanges([]int{0}, []uint64{tags1}, []uint64{tags2}, 9)
	if len(got.Forest) != 1 || got.Forest[0] != (CoordRange{From: 0, To: 10}) {
		t.Errorf("Forest: %+v", got.Forest)
	}
	if r, ok := got.Sac["T2"]; !ok || len(r) != 1 || r[0] != (CoordRange{From: 0, To: 10}) {
		t.Errorf("Sac[T2]: ok=%v val=%+v", ok, r)
	}
	if got.Protected != nil {
		t.Errorf("Protected expected nil, got %+v", got.Protected)
	}
	if got.Mtb != nil {
		t.Errorf("Mtb expected nil, got %+v", got.Mtb)
	}
}

func TestComputeAllRanges_MismatchedSlices(t *testing.T) {
	got := ComputeAllRanges([]int{0, 5}, []uint64{1}, []uint64{1, 2}, 9)
	if got.Forest != nil || got.Sac != nil {
		t.Errorf("mismatched slices: got %+v, want zero AllRanges", got)
	}
}

func TestAggregateForestProtected_AllForest(t *testing.T) {
	coords := []LatLng{{47.0, 11.0}, {47.001, 11.0}, {47.002, 11.0}}
	edgeCoordStart := []int{0, 1}
	tags2 := []uint64{100, 100}

	fp, pp := AggregateForestProtected(coords, edgeCoordStart, tags2, 2)
	if math.Abs(fp-1.0) > 0.01 {
		t.Errorf("forestPct: got %f, want ~1.0", fp)
	}
	if pp != 0 {
		t.Errorf("protectedPct: got %f, want 0", pp)
	}
}

func TestAggregateForestProtected_Empty(t *testing.T) {
	fp, pp := AggregateForestProtected(nil, nil, nil, 0)
	if fp != 0 || pp != 0 {
		t.Errorf("empty: got (%f, %f), want (0, 0)", fp, pp)
	}
}

func TestCoalesceConsecutive_SpecExample(t *testing.T) {
	idxs := []uint32{0xA, 0xA, 0xA, 0xB, 0xB, 0xA, 0xA}
	t1 := []uint64{1, 1, 1, 2, 2, 1, 1}
	t2 := []uint64{10, 10, 10, 20, 20, 10, 10}
	starts, ot1, ot2 := CoalesceConsecutive(idxs, t1, t2)
	wantStarts := []int{0, 3, 5}
	if len(starts) != len(wantStarts) {
		t.Fatalf("starts: got %v, want %v", starts, wantStarts)
	}
	for i, w := range wantStarts {
		if starts[i] != w {
			t.Errorf("starts[%d]: got %d, want %d", i, starts[i], w)
		}
	}
	wantT1 := []uint64{1, 2, 1}
	wantT2 := []uint64{10, 20, 10}
	for i := range wantT1 {
		if ot1[i] != wantT1[i] || ot2[i] != wantT2[i] {
			t.Errorf("tags[%d]: got (%d, %d), want (%d, %d)", i, ot1[i], ot2[i], wantT1[i], wantT2[i])
		}
	}
}

func TestCoalesceConsecutive_Empty(t *testing.T) {
	starts, t1, t2 := CoalesceConsecutive(nil, nil, nil)
	if starts != nil || t1 != nil || t2 != nil {
		t.Errorf("empty input: got non-nil")
	}
}
