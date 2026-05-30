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

// encodeSacScale packs a SAC raw value (1..6 → T1..T6, per SacScaleShort) into
// Tags1 bits 14-16.
func encodeSacScale(raw uint8) uint64 {
	return uint64(raw&0x07) << 14
}

// encodeMtbScale packs an MTB raw value (1..6 → S0..S5, per MtbScaleShort) into
// Tags1 bits 17-19.
func encodeMtbScale(raw uint8) uint64 {
	return uint64(raw&0x07) << 17
}

func TestComputeScaleRanges_SAC_T3(t *testing.T) {
	// Build Tags1 words where bits 14-16 encode the SAC-scale enum value for T3.
	// SacScaleShort(3) == "T3" (direct mapping, no offset).
	tagsT3 := encodeSacScale(3) // raw=3 → "T3"
	tagsT1 := encodeSacScale(1) // raw=1 → "T1"
	edgeTags1 := []uint64{tagsT3, tagsT1, tagsT3}
	edgeCoordStart := []int{0, 5, 10}

	got := computeScaleRanges(edgeCoordStart, edgeTags1, 14, "sac", "T3")
	if len(got) != 2 {
		t.Fatalf("SAC T3 → got %d ranges, want 2", len(got))
	}
	// T1 ranges for the same input must be exactly one range (middle edge only).
	if got := computeScaleRanges(edgeCoordStart, edgeTags1, 14, "sac", "T1"); len(got) != 1 {
		t.Fatalf("SAC T1 → got %d ranges, want 1", len(got))
	}
}

func TestComputeScaleRanges_UnknownLevel(t *testing.T) {
	// Levels outside the defined set must yield empty ranges (never panic).
	got := computeScaleRanges([]int{0}, []uint64{0}, 0, "sac", "T99")
	if len(got) != 0 {
		t.Fatalf("unknown level → got %v, want empty", got)
	}
}

func TestComputeScaleRanges_MTB_OffByOneGuard(t *testing.T) {
	// raw=3 maps to "S2" (MtbScaleShort uses -1 offset), NOT "S3".
	// This test documents the invariant to catch copy-paste errors.
	tagsS2 := encodeMtbScale(3) // raw=3 → "S2"
	edgeTags1 := []uint64{tagsS2}
	edgeCoordStart := []int{0}

	// Must match "S2"
	if got := computeScaleRanges(edgeCoordStart, edgeTags1, 4, "mtb", "S2"); len(got) != 1 {
		t.Fatalf("MTB raw=3 must match S2, got %v ranges", len(got))
	}
	// Must NOT match "S3"
	if got := computeScaleRanges(edgeCoordStart, edgeTags1, 4, "mtb", "S3"); len(got) != 0 {
		t.Fatalf("MTB raw=3 must NOT match S3 (off-by-one guard), got %v ranges", len(got))
	}
}

// encodeVfScale packt einen VF raw-Wert (0..6) in Tags2 Bits 33-35.
// Spiegelt EncodeOSMTags in graph/tags.go (bits 33-35 = via_ferrata_scale).
func encodeVfScale(raw uint8) uint64 {
	return uint64(raw&0x07) << 33
}

func TestComputeVfRanges_K3(t *testing.T) {
	// Bits 33-35 kodieren via_ferrata_scale; K3 = raw 3.
	edgeTags2 := []uint64{
		encodeVfScale(3), // K3
		encodeVfScale(1), // K1
		encodeVfScale(3), // K3
	}
	edgeCoordStart := []int{0, 5, 10}
	got := computeVfRanges(edgeCoordStart, edgeTags2, 14, "K3")
	if len(got) != 2 {
		t.Fatalf("VF K3 → got %d ranges, want 2", len(got))
	}
	if got := computeVfRanges(edgeCoordStart, edgeTags2, 14, "K1"); len(got) != 1 {
		t.Fatalf("VF K1 → got %d ranges, want 1", len(got))
	}
}

// TestComputeRanges_LenMismatch_ReturnsNil verifies that all compute*Ranges
// functions return nil (no panic) when edgeCoordStart and the tag slice(s) have
// mismatched lengths.  This guards against programmer errors in the route-build
// pipeline — mismatches are never expected in production after the Phase-1
// Regression fix invariant holds, but the guard provides a safety net.
func TestComputeRanges_LenMismatch_ReturnsNil(t *testing.T) {
	edgeCoordStart := []int{0, 5} // len 2
	tags3 := []uint64{0, 0, 0}    // len 3 — mismatch

	if r := computeForestRanges(edgeCoordStart, tags3, 10); r != nil {
		t.Fatal("computeForestRanges: len-mismatch must return nil")
	}
	if r := computeProtectedRanges(edgeCoordStart, tags3, 10); r != nil {
		t.Fatal("computeProtectedRanges: len-mismatch must return nil")
	}
	if r := computeScaleRanges(edgeCoordStart, tags3, 10, "sac", "T1"); r != nil {
		t.Fatal("computeScaleRanges sac: len-mismatch must return nil")
	}
	if r := computeScaleRanges(edgeCoordStart, tags3, 10, "mtb", "S0"); r != nil {
		t.Fatal("computeScaleRanges mtb: len-mismatch must return nil")
	}
	if r := computeVfRanges(edgeCoordStart, tags3, 10, "K1"); r != nil {
		t.Fatal("computeVfRanges: len-mismatch must return nil")
	}
	if r := computeVfUnknownRanges(edgeCoordStart, tags3, tags3, 10); r != nil {
		t.Fatal("computeVfUnknownRanges: len-mismatch must return nil")
	}
}

func TestComputeVfUnknownRanges_Matches(t *testing.T) {
	// highway == 22 UND vfScale == 0 = K?-Range.
	edgeTags1 := []uint64{
		22, // via_ferrata (Bits 0-4)
		22, // via_ferrata
		1,  // path (nicht VF)
	}
	edgeTags2 := []uint64{
		encodeVfScale(0), // unscaled
		encodeVfScale(3), // K3 — KEIN K?
		encodeVfScale(0), // unscaled — aber kein VF-Highway
	}
	edgeCoordStart := []int{0, 5, 10}
	got := computeVfUnknownRanges(edgeCoordStart, edgeTags1, edgeTags2, 14)
	if len(got) != 1 {
		t.Fatalf("VF unknown → got %d ranges, want 1 (edge 0 only)", len(got))
	}
	if got[0].From != 0 || got[0].To != 5 {
		t.Fatalf("VF unknown range = %+v, want [0,5]", got[0])
	}
}

// TestComputeAllRanges_MatchesIndividualCalls is a regression guard that verifies
// computeAllRanges produces identical output to the 15 individual compute* calls
// on the same synthetic edge arrays. Any divergence between the fused path and
// the individual paths would be caught here.
func TestComputeAllRanges_MatchesIndividualCalls(t *testing.T) {
	// Synthetic 5-edge route exercising every category:
	//   edge 0: forest(70), protected(1), SAC T3 (raw=3), MTB S2 (raw=3), VF K3 (raw=3)
	//   edge 1: forest(30 — below threshold), protected(0), SAC T1 (raw=1), MTB S0 (raw=1), VF K1 (raw=1)
	//   edge 2: forest(80), protected(2), SAC T3 (raw=3), MTB S2 (raw=3), VF K3 (raw=3)
	//   edge 3: highway=22, VF unknown (vfScale=0) — K?
	//   edge 4: forest(90), protected(3), SAC T6 (raw=6), MTB S5 (raw=6), VF K6 (raw=6)
	edgeCoordStart := []int{0, 5, 10, 15, 20}
	lastCoordIdx := 24

	edgeTags1 := []uint64{
		encodeSacScale(3) | encodeMtbScale(3),              // edge 0: T3, S2
		encodeSacScale(1) | encodeMtbScale(1),              // edge 1: T1, S0
		encodeSacScale(3) | encodeMtbScale(3),              // edge 2: T3, S2
		uint64(22) | encodeSacScale(0) | encodeMtbScale(0), // edge 3: via_ferrata hw, no SAC/MTB
		encodeSacScale(6) | encodeMtbScale(6),              // edge 4: T6, S5
	}
	edgeTags2 := []uint64{
		encodeForestPct(70) | encodeProtectedArea(1) | encodeVfScale(3), // edge 0
		encodeForestPct(30) | encodeProtectedArea(0) | encodeVfScale(1), // edge 1
		encodeForestPct(80) | encodeProtectedArea(2) | encodeVfScale(3), // edge 2
		encodeForestPct(0) | encodeProtectedArea(0) | encodeVfScale(0),  // edge 3: K?
		encodeForestPct(90) | encodeProtectedArea(3) | encodeVfScale(6), // edge 4
	}

	got := computeAllRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx)

	// --- Forest ---
	wantForest := computeForestRanges(edgeCoordStart, edgeTags2, lastCoordIdx)
	if !reflect.DeepEqual(got.Forest, wantForest) {
		t.Errorf("Forest: fused=%v individual=%v", got.Forest, wantForest)
	}

	// --- Protected ---
	wantProtected := computeProtectedRanges(edgeCoordStart, edgeTags2, lastCoordIdx)
	if !reflect.DeepEqual(got.Protected, wantProtected) {
		t.Errorf("Protected: fused=%v individual=%v", got.Protected, wantProtected)
	}

	// --- VfUnknown ---
	wantVfUnknown := computeVfUnknownRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx)
	if !reflect.DeepEqual(got.VfUnknown, wantVfUnknown) {
		t.Errorf("VfUnknown: fused=%v individual=%v", got.VfUnknown, wantVfUnknown)
	}

	// --- SAC levels ---
	for _, level := range sacLevels {
		wantSac := computeScaleRanges(edgeCoordStart, edgeTags1, lastCoordIdx, "sac", level)
		gotSac := got.Sac[level]
		if !reflect.DeepEqual(normaliseRanges(gotSac), normaliseRanges(wantSac)) {
			t.Errorf("SAC %s: fused=%v individual=%v", level, gotSac, wantSac)
		}
	}

	// --- MTB levels ---
	for _, level := range mtbLevels {
		wantMtb := computeScaleRanges(edgeCoordStart, edgeTags1, lastCoordIdx, "mtb", level)
		gotMtb := got.Mtb[level]
		if !reflect.DeepEqual(normaliseRanges(gotMtb), normaliseRanges(wantMtb)) {
			t.Errorf("MTB %s: fused=%v individual=%v", level, gotMtb, wantMtb)
		}
	}

	// --- VF levels ---
	for _, level := range vfLevels {
		wantVf := computeVfRanges(edgeCoordStart, edgeTags2, lastCoordIdx, level)
		gotVf := got.Vf[level]
		if !reflect.DeepEqual(normaliseRanges(gotVf), normaliseRanges(wantVf)) {
			t.Errorf("VF %s: fused=%v individual=%v", level, gotVf, wantVf)
		}
	}
}

// normaliseRanges converts nil to an empty slice so reflect.DeepEqual treats
// nil and []CoordRange{} as equal (both mean "no ranges for this level").
func normaliseRanges(r []CoordRange) []CoordRange {
	if r == nil {
		return []CoordRange{}
	}
	return r
}

// BenchmarkComputeRanges_Fused_vs_Single compares the fused single-pass
// computeAllRanges against 15 individual compute* calls on a synthetic
// 1000-edge route.
// Run with: go test -bench=BenchmarkComputeRanges -benchmem ./internal/handler/
func BenchmarkComputeRanges_Fused_vs_Single(b *testing.B) {
	const n = 1000
	edgeCoordStart := make([]int, n)
	edgeTags1 := make([]uint64, n)
	edgeTags2 := make([]uint64, n)
	for i := 0; i < n; i++ {
		edgeCoordStart[i] = i * 5
		sacRaw := uint8((i % 6) + 1) // cycles 1..6
		mtbRaw := uint8((i % 6) + 1)
		vfRaw := uint8((i % 6) + 1)
		edgeTags1[i] = encodeSacScale(sacRaw) | encodeMtbScale(mtbRaw)
		edgeTags2[i] = encodeForestPct(uint8((i%100)+1)) |
			encodeProtectedArea(uint8(i%4)) |
			encodeVfScale(vfRaw)
	}
	lastCoordIdx := n*5 - 1

	b.Run("Fused", func(b *testing.B) {
		for b.Loop() {
			_ = computeAllRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx)
		}
	})

	b.Run("Individual15", func(b *testing.B) {
		for b.Loop() {
			_ = computeForestRanges(edgeCoordStart, edgeTags2, lastCoordIdx)
			_ = computeProtectedRanges(edgeCoordStart, edgeTags2, lastCoordIdx)
			_ = computeVfUnknownRanges(edgeCoordStart, edgeTags1, edgeTags2, lastCoordIdx)
			for _, level := range sacLevels {
				_ = computeScaleRanges(edgeCoordStart, edgeTags1, lastCoordIdx, "sac", level)
			}
			for _, level := range mtbLevels {
				_ = computeScaleRanges(edgeCoordStart, edgeTags1, lastCoordIdx, "mtb", level)
			}
			for _, level := range vfLevels {
				_ = computeVfRanges(edgeCoordStart, edgeTags2, lastCoordIdx, level)
			}
		}
	})
}
