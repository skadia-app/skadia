package handler

import (
	"reflect"
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

func TestContiguousRangesFromMatch_Empty(t *testing.T) {
	got := contiguousRangesFromMatch([]bool{}, []int{}, 0)
	if len(got) != 0 {
		t.Fatalf("empty → got %v, want empty", got)
	}
}

func TestContiguousRangesFromMatch_NoMatches(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{false, false, false},
		[]int{0, 5, 10},
		14,
	)
	if len(got) != 0 {
		t.Fatalf("no matches → got %v, want empty", got)
	}
}

func TestContiguousRangesFromMatch_AllMatch(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{true, true, true},
		[]int{0, 5, 10},
		14,
	)
	want := []CoordRange{{From: 0, To: 15}} // last edge: lastCoordIdx+1
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestContiguousRangesFromMatch_Alternating(t *testing.T) {
	// match-nomatch-match → two separate ranges
	got := contiguousRangesFromMatch(
		[]bool{true, false, true},
		[]int{0, 5, 10},
		14,
	)
	want := []CoordRange{{From: 0, To: 5}, {From: 10, To: 15}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestContiguousRangesFromMatch_ContiguousMerges(t *testing.T) {
	got := contiguousRangesFromMatch(
		[]bool{true, true, false, true},
		[]int{0, 5, 10, 15},
		19,
	)
	want := []CoordRange{{From: 0, To: 10}, {From: 15, To: 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// encodeForestPct packs a forest-pct value (0..100) into the Tags2 word at the
// position DecodeForestPct reads from (bits 0-6, 7 bits; see tags.go:326-327
// and EncodeEdgeTags2 bit-layout comment at tags.go:213). Used to build
// synthetic Tags2 words for unit testing — mirrors the production encoder.
func encodeForestPct(pct uint8) uint64 {
	return uint64(pct & 0x7F) // bits 0-6, no shift
}

// encodeProtectedArea packs a protected_area level (0..3) into Tags2 bits 17-18
// (see tags.go:335-336 decoder + tags.go:216 layout comment).
// Bits 17-18 in Tags2 (NICHT Tags1; MTB lebt bei Tags1 bit 17-19 — unrelated).
func encodeProtectedArea(level uint8) uint64 {
	return uint64(level&0x03) << 17
}

// Ensure graph is used (import guard); DecodeForestPct/DecodeProtectedArea
// are the production decoders that mirror these test helpers.
var _ = graph.DecodeForestPct

func TestComputeForestRanges_AllForestEdges(t *testing.T) {
	// 3 edges, all forest ≥ 60
	edgeTags2 := []uint64{encodeForestPct(70), encodeForestPct(80), encodeForestPct(90)}
	edgeCoordStart := []int{0, 5, 10}
	got := computeForestRanges(edgeCoordStart, edgeTags2, 14)
	if len(got) != 1 || got[0].From != 0 || got[0].To != 15 {
		t.Fatalf("got %v, want one range [0,15)", got)
	}
}

func TestComputeForestRanges_VirtualEdgeBreaks(t *testing.T) {
	// edge[1] is virtual (Tags2=0 → ForestPct=0) → breaks the contiguous range
	edgeTags2 := []uint64{encodeForestPct(80), 0, encodeForestPct(80)}
	edgeCoordStart := []int{0, 5, 10}
	got := computeForestRanges(edgeCoordStart, edgeTags2, 14)
	if len(got) != 2 {
		t.Fatalf("got %d ranges, want 2 (virtual edge should break)", len(got))
	}
}

func TestComputeForestRanges_BelowThreshold(t *testing.T) {
	edgeTags2 := []uint64{encodeForestPct(30), encodeForestPct(50)}
	edgeCoordStart := []int{0, 5}
	got := computeForestRanges(edgeCoordStart, edgeTags2, 9)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty (all below threshold)", got)
	}
}

func TestComputeProtectedRanges_AllProtectedEdges(t *testing.T) {
	// 3 edges, all protected_area ≥ 1
	edgeTags2 := []uint64{encodeProtectedArea(1), encodeProtectedArea(2), encodeProtectedArea(3)}
	edgeCoordStart := []int{0, 5, 10}
	got := computeProtectedRanges(edgeCoordStart, edgeTags2, 14)
	if len(got) != 1 || got[0].From != 0 || got[0].To != 15 {
		t.Fatalf("got %v, want one range [0,15)", got)
	}
}

func TestComputeProtectedRanges_UnprotectedEdgeBreaks(t *testing.T) {
	// edge[1] has protected_area=0 → breaks the range
	edgeTags2 := []uint64{encodeProtectedArea(1), encodeProtectedArea(0), encodeProtectedArea(2)}
	edgeCoordStart := []int{0, 5, 10}
	got := computeProtectedRanges(edgeCoordStart, edgeTags2, 14)
	if len(got) != 2 {
		t.Fatalf("got %d ranges, want 2 (unprotected edge should break)", len(got))
	}
}

func TestComputeProtectedRanges_BelowThreshold(t *testing.T) {
	// All edges have protected_area=0 (below protectedHighlightMin=1)
	edgeTags2 := []uint64{encodeProtectedArea(0), encodeProtectedArea(0)}
	edgeCoordStart := []int{0, 5}
	got := computeProtectedRanges(edgeCoordStart, edgeTags2, 9)
	if len(got) != 0 {
		t.Fatalf("got %v, want empty (all below threshold)", got)
	}
}
