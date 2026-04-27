package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestRouteResult_BuildProperties_ElevationProfileNullVsSlice(t *testing.T) {
	rNil := &RouteResult{ElevationProfile: nil}
	propsNil := rNil.buildProperties()
	raw, err := json.Marshal(propsNil["elevation_profile"])
	if err != nil {
		t.Fatalf("json.Marshal nil elevation_profile: %v", err)
	}
	if string(raw) != "null" {
		t.Fatalf("nil ElevationProfile: got %s, want null", string(raw))
	}

	profile := []ElevationPoint{{DistM: 0, EleM: PtrEle(593)}, {DistM: 100, EleM: PtrEle(610)}}
	rSet := &RouteResult{ElevationProfile: &profile}
	propsSet := rSet.buildProperties()
	raw2, err := json.Marshal(propsSet["elevation_profile"])
	if err != nil {
		t.Fatalf("json.Marshal set elevation_profile: %v", err)
	}
	// Unmarshal both and compare structurally (JSONEq equivalent)
	var got, want []map[string]any
	if err := json.Unmarshal(raw2, &got); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal([]byte(`[{"dist_m":0,"ele_m":593},{"dist_m":100,"ele_m":610}]`), &want); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("elevation_profile len = %d, want %d", len(got), len(want))
	}
	for i, wEntry := range want {
		for k, wv := range wEntry {
			gv, ok := got[i][k]
			if !ok {
				t.Fatalf("elevation_profile[%d] missing key %q", i, k)
			}
			if gv != wv {
				t.Fatalf("elevation_profile[%d][%q] = %v, want %v", i, k, gv, wv)
			}
		}
	}
}

func TestRouteResultAvgFlat(t *testing.T) {
	r := &RouteResult{DistSum: 1000, QualitySum: 1200}
	got := r.AvgFlat()
	if got != 1.2 {
		t.Errorf("AvgFlat = %v, want 1.2", got)
	}
}

func TestRouteResultAvgFlatEmpty(t *testing.T) {
	r := &RouteResult{}
	if r.AvgFlat() != 0 {
		t.Errorf("empty route AvgFlat = %v, want 0", r.AvgFlat())
	}
}

// TestRetrackUnified_PopulatesEdgeSlices verifiziert, dass retrackUnified
// EdgeCoordStart/EdgeTags1/EdgeTags2 mit Länge == len(EdgeHandles) befüllt
// und die CoordStart-Werte monoton nicht-fallend sind.
func TestRetrackUnified_PopulatesEdgeSlices(t *testing.T) {
	// 4-node chain: 3 edges (0→1→2→3)
	g := makeChainGraph(4)
	reader := &SingleGraphReader{G: g, TileID: 0}
	cfg := DefaultCostConfig()

	src := MakeGlobalNodeID(0, 0)
	dst := MakeGlobalNodeID(0, 3)

	pass0, err := findRouteUnified(context.Background(), reader, nil, nil, src, dst, cfg, cfg.Pass0Coefficient, 0, nil, nil, nil)
	if err != nil {
		t.Fatalf("pass0 failed: %v", err)
	}
	trackIndex := buildTrackIndexUnified(pass0.state, dst, 0)

	pass1, err := findRouteUnified(context.Background(), reader, nil, nil, src, dst, cfg, 0, pass0.totalCost*cfg.Pass0Coefficient*cfg.Pass0Coefficient, trackIndex, nil, nil)
	if err != nil {
		t.Fatalf("pass1 failed: %v", err)
	}

	result := retrackUnified(reader, pass1.state, pass1.bestDstNode, nil, nil, cfg)
	if result == nil {
		t.Fatal("retrackUnified returned nil")
	}

	nEdges := len(result.EdgeHandles)
	if nEdges == 0 {
		t.Fatal("EdgeHandles is empty — no edges in route")
	}

	if got, want := len(result.EdgeCoordStart), nEdges; got != want {
		t.Fatalf("EdgeCoordStart len = %d, want %d (== len(EdgeHandles))", got, want)
	}
	if got, want := len(result.EdgeTags1), nEdges; got != want {
		t.Fatalf("EdgeTags1 len = %d, want %d (== len(EdgeHandles))", got, want)
	}
	if got, want := len(result.EdgeTags2), nEdges; got != want {
		t.Fatalf("EdgeTags2 len = %d, want %d (== len(EdgeHandles))", got, want)
	}

	// CoordStart must be monoton nicht-fallend (zero-length edges may share coord start)
	for i := 1; i < len(result.EdgeCoordStart); i++ {
		if result.EdgeCoordStart[i] < result.EdgeCoordStart[i-1] {
			t.Fatalf("EdgeCoordStart not monotonic at i=%d: %d < %d",
				i, result.EdgeCoordStart[i], result.EdgeCoordStart[i-1])
		}
	}

	// Last CoordStart must be a valid index into Coordinates (< len)
	last := result.EdgeCoordStart[len(result.EdgeCoordStart)-1]
	if last >= len(result.Coordinates) {
		t.Fatalf("last EdgeCoordStart %d >= len(Coordinates) %d", last, len(result.Coordinates))
	}
}

func TestMergeLegs_ShiftsEdgeCoordStartAndAppendsTags(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1, Lon: 1}, {Lat: 2, Lon: 2}, {Lat: 3, Lon: 3}},
		EdgeHandles:    []uint64{100, 101},
		EdgeCoordStart: []int{0, 1},
		EdgeTags1:      []uint64{0xAA, 0xBB},
		EdgeTags2:      []uint64{0x11, 0x22},
	}
	legB := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 3, Lon: 3}, {Lat: 4, Lon: 4}}, // dup first
		EdgeHandles:    []uint64{200},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xCC},
		EdgeTags2:      []uint64{0x33},
	}
	merged := mergeLegs([]*RouteResult{legA, legB})

	// Coordinates: legA(3) + legB[1:](1) == 4
	if got, want := len(merged.Coordinates), 4; got != want {
		t.Fatalf("Coordinates len = %d, want %d", got, want)
	}
	// EdgeHandles: 2 + 1 == 3
	if got, want := len(merged.EdgeHandles), 3; got != want {
		t.Fatalf("EdgeHandles len = %d, want %d", got, want)
	}
	// EdgeCoordStart: [0, 1, 2]  (legB's 0 shifted by coordsBefore-1 == 3-1 == 2)
	want := []int{0, 1, 2}
	for i, v := range want {
		if merged.EdgeCoordStart[i] != v {
			t.Fatalf("EdgeCoordStart[%d] = %d, want %d", i, merged.EdgeCoordStart[i], v)
		}
	}
	// Tags appended 1:1
	if merged.EdgeTags1[2] != 0xCC {
		t.Fatalf("EdgeTags1[2] = %x, want 0xCC", merged.EdgeTags1[2])
	}
	if merged.EdgeTags2[2] != 0x33 {
		t.Fatalf("EdgeTags2[2] = %x, want 0x33", merged.EdgeTags2[2])
	}
}

func TestMergeLegs_EmptyFollowLegSkipsCoordStartShift(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1}, {Lat: 2}},
		EdgeHandles:    []uint64{100},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xAA},
		EdgeTags2:      []uint64{0x11},
	}
	legB := &RouteResult{} // empty coords, empty slices

	merged := mergeLegs([]*RouteResult{legA, legB})
	// len(EdgeCoordStart) stays 1; no stray shifted zero was appended
	if got, want := len(merged.EdgeCoordStart), 1; got != want {
		t.Fatalf("EdgeCoordStart len = %d, want %d", got, want)
	}
	if merged.EdgeCoordStart[0] != 0 {
		t.Fatalf("EdgeCoordStart[0] = %d, want 0", merged.EdgeCoordStart[0])
	}
}

// makeTestLeg builds a minimal *RouteResult with the given DistanceM and optional profile.
func makeTestLeg(distM float64, profile []ElevationPoint) *RouteResult {
	r := &RouteResult{DistanceM: distM}
	if profile != nil {
		cp := make([]ElevationPoint, len(profile))
		copy(cp, profile)
		r.ElevationProfile = &cp
	}
	return r
}

func TestMergeLegs_DistanceAxisContinuity(t *testing.T) {
	leg1 := makeTestLeg(50.0, []ElevationPoint{{0, PtrEle(100)}, {25, PtrEle(105)}, {50, PtrEle(110)}})
	leg2 := makeTestLeg(40.0, []ElevationPoint{{0, PtrEle(110)}, {20, PtrEle(120)}, {40, PtrEle(130)}})
	leg3 := makeTestLeg(60.0, []ElevationPoint{{0, PtrEle(130)}, {30, PtrEle(140)}, {60, PtrEle(150)}})
	merged := mergeLegs([]*RouteResult{leg1, leg2, leg3})
	if merged.ElevationProfile == nil {
		t.Fatal("merged.ElevationProfile is nil, want non-nil")
	}
	p := *merged.ElevationProfile
	// Junction-dedup: leg2[0]=110@dist50 dropped (== leg1[last]); leg3[0]=130@dist90 dropped
	// Expected: 3 + 2 + 2 = 7 points
	if got, want := len(p), 7; got != want {
		t.Fatalf("len(profile) = %d, want %d", got, want)
	}
	// Strict monotonic
	for i := 1; i < len(p); i++ {
		if !(p[i].DistM > p[i-1].DistM) {
			t.Fatalf("non-monotonic at %d: %v <= %v", i, p[i].DistM, p[i-1].DistM)
		}
	}
	assertInDelta(t, 150.0, p[len(p)-1].DistM, 0.01, "last DistM")
}

func TestMergeLegs_LegProfileDoesntStartAtZero_DistanceShiftPreservesContinuity(t *testing.T) {
	// Critic-Finding #21: wenn ein Leg-Profile nicht bei DistM=0 beginnt
	// (z.B. wegen vorgelagerter degenerate-Edges innerhalb des Legs),
	// bleibt der Distanz-Shift mathematisch konsistent — der erste
	// emittierte Punkt landet bei mergedDistM + legProfile[startIdx].DistM,
	// also nicht zwingend bei mergedDistM.
	leg1 := makeTestLeg(50.0, []ElevationPoint{{0, PtrEle(100)}, {50, PtrEle(110)}})
	// leg2 startet im internen Profile bei 5m (degenerate erste Edge):
	leg2 := makeTestLeg(40.0, []ElevationPoint{{5, PtrEle(110)}, {40, PtrEle(130)}})
	merged := mergeLegs([]*RouteResult{leg1, leg2})
	p := *merged.ElevationProfile
	// leg1 emits 2 points (indices 0,1). leg2 has 2 input points but dedup
	// startIdx=1 drops the first → emits 1 point (index 2). Total = 3.
	// (Plan §5.5 had a stale `Len(p, 4)` assertion that contradicted the
	// emitted-points enumeration in the same test; the index-level assertions
	// p[0..2] are authoritative.)
	if got, want := len(p), 3; got != want {
		t.Fatalf("len(profile) = %d, want %d", got, want)
	}
	// leg1: [0, 50] → emitted as-is
	assertInDelta(t, 0.0, p[0].DistM, 0.01, "p[0].DistM")
	assertInDelta(t, 50.0, p[1].DistM, 0.01, "p[1].DistM")
	// leg2: dedup startIdx=1 (len(mergedProfile)>0); aber leg2.legProfile[0]
	// hat DistM=5, nicht 0 → der gedroppte erste Punkt LIEGT NICHT auf
	// mergedDistM=50, sondern hätte 55 ergeben.
	// Emittiert: legProfile[1] = {40, 130} → DistM = 50 + 40 = 90.
	assertInDelta(t, 90.0, p[2].DistM, 0.01, "p[2].DistM")
	assertInDelta(t, 130.0, *p[2].EleM, 0.01, "p[2].EleM")
	// Strikt monoton trotz potenziellem Distanz-Sprung
	for i := 1; i < len(p); i++ {
		if !(p[i].DistM > p[i-1].DistM) {
			t.Fatalf("non-monotonic at %d: %v <= %v", i, p[i].DistM, p[i-1].DistM)
		}
	}
}

func TestMergeLegs_FirstLegNilProfile_NoDedupOnFollowLegStart(t *testing.T) {
	leg1 := makeTestLeg(50.0, nil) // no profile
	leg2 := makeTestLeg(40.0, []ElevationPoint{{0, PtrEle(110)}, {40, PtrEle(130)}})
	merged := mergeLegs([]*RouteResult{leg1, leg2})
	if merged.ElevationProfile == nil {
		t.Fatal("merged.ElevationProfile is nil, want non-nil")
	}
	p := *merged.ElevationProfile
	// First profile point of leg2 must be emitted (no previous to dedup against)
	if got, want := len(p), 2; got != want {
		t.Fatalf("len(profile) = %d, want %d", got, want)
	}
	// distance shift by leg1.DistanceM = 50
	assertInDelta(t, 50.0, p[0].DistM, 0.01, "p[0].DistM")
	assertInDelta(t, 110.0, *p[0].EleM, 0.01, "p[0].EleM")
	assertInDelta(t, 90.0, p[1].DistM, 0.01, "p[1].DistM")
}

func TestMergeLegs_MiddleLegNilProfile_DistanceGap(t *testing.T) {
	leg1 := makeTestLeg(50.0, []ElevationPoint{{0, PtrEle(100)}, {50, PtrEle(110)}})
	leg2 := makeTestLeg(40.0, nil)
	leg3 := makeTestLeg(60.0, []ElevationPoint{{0, PtrEle(110)}, {60, PtrEle(150)}})
	merged := mergeLegs([]*RouteResult{leg1, leg2, leg3})
	p := *merged.ElevationProfile
	// Per §5.4: dedup gate is len(mergedProfile) > 0.
	// Leg1 → emit 2 points at dist {0, 50}
	// Leg2 (nil profile) → mergedDistM advances by 40 to 90, no point emitted
	// Leg3 → len(mergedProfile) > 0, so dedup startIdx=1, emit only second point at dist 90+60=150
	// Expected: exactly 3 points total.
	if got, want := len(p), 3; got != want {
		t.Fatalf("len(profile) = %d, want %d", got, want)
	}
	assertInDelta(t, 0.0, p[0].DistM, 0.01, "p[0].DistM")
	assertInDelta(t, 100.0, *p[0].EleM, 0.01, "p[0].EleM")
	assertInDelta(t, 50.0, p[1].DistM, 0.01, "p[1].DistM")
	assertInDelta(t, 110.0, *p[1].EleM, 0.01, "p[1].EleM")
	assertInDelta(t, 150.0, p[2].DistM, 0.01, "p[2].DistM")
	assertInDelta(t, 150.0, *p[2].EleM, 0.01, "p[2].EleM")
	// Strikt monoton (inkl. des 100m-Gaps zwischen p[1] und p[2]):
	for i := 1; i < len(p); i++ {
		if !(p[i].DistM > p[i-1].DistM) {
			t.Fatalf("non-monotonic at %d: %v <= %v", i, p[i].DistM, p[i-1].DistM)
		}
	}
	// Der Gap ist die volle leg2-Distanz PLUS der leg3-dedup-geskippten intra-Distanz (0m):
	assertInDelta(t, 100.0, p[2].DistM-p[1].DistM, 0.01, "gap p[2]-p[1]")
}

// TestMergeLegs_EmptyCoordsWithTags_InvariantSafe prüft den pathologischen Fall:
// ein Folge-Leg hat keine Coordinates aber nicht-leere EdgeTags1/EdgeTags2.
// Invariant: len(EdgeTags1) == len(EdgeTags2) == len(EdgeCoordStart) nach dem Merge.
// Vor dem Fix führte das unconditional append von Tags zu einem out-of-range Panic
// in computeScaleRanges / contiguousRangesFromMatch (Iteration über edgeTags1 mit
// edgeCoordStart[i] ohne length-guard).
func TestMergeLegs_EmptyCoordsWithTags_InvariantSafe(t *testing.T) {
	legA := &RouteResult{
		Coordinates:    []LatLonEle{{Lat: 1}, {Lat: 2}},
		EdgeHandles:    []uint64{100},
		EdgeCoordStart: []int{0},
		EdgeTags1:      []uint64{0xAA},
		EdgeTags2:      []uint64{0x11},
	}
	// Pathological leg: no coords but has tags — should NOT append tags
	// (empty leg = no edges to merge; tags without a CoordStart anchor are bogus).
	legB := &RouteResult{
		Coordinates:    []LatLonEle{},
		EdgeHandles:    []uint64{200},
		EdgeCoordStart: []int{},
		EdgeTags1:      []uint64{0xCC},
		EdgeTags2:      []uint64{0x33},
	}
	merged := mergeLegs([]*RouteResult{legA, legB})

	// Invariant: all three per-edge slices must have the same length.
	nCoordStart := len(merged.EdgeCoordStart)
	if got := len(merged.EdgeTags1); got != nCoordStart {
		t.Fatalf("invariant broken: len(EdgeTags1)=%d != len(EdgeCoordStart)=%d", got, nCoordStart)
	}
	if got := len(merged.EdgeTags2); got != nCoordStart {
		t.Fatalf("invariant broken: len(EdgeTags2)=%d != len(EdgeCoordStart)=%d", got, nCoordStart)
	}
	// legA contributes exactly 1 edge; legB (empty coords) contributes nothing.
	if nCoordStart != 1 {
		t.Fatalf("EdgeCoordStart len = %d, want 1 (legA only)", nCoordStart)
	}
	// legA's tags must be intact.
	if merged.EdgeTags1[0] != 0xAA {
		t.Fatalf("EdgeTags1[0] = %x, want 0xAA", merged.EdgeTags1[0])
	}
	if merged.EdgeTags2[0] != 0x11 {
		t.Fatalf("EdgeTags2[0] = %x, want 0x11", merged.EdgeTags2[0])
	}
	// EdgeHandles is NOT length-synchronized with EdgeCoordStart (used by
	// aggregateLegOutdoor/aggregateLeg2Coverage which iterate handles independently
	// via reader.EdgeTags*(h)). So we accept EdgeHandles may have len=2 here.
}

func TestElevationPoint_NullJSONRoundTrip(t *testing.T) {
	in := []ElevationPoint{{DistM: 0, EleM: PtrEle(100.5)}, {DistM: 50, EleM: nil}}
	buf, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(buf, []byte(`"ele_m":null`)) {
		t.Fatalf("expected null in JSON, got %s", buf)
	}
	var out []ElevationPoint
	if err := json.Unmarshal(buf, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out[1].EleM != nil {
		t.Fatal("nil round-trip broken")
	}
	if out[0].EleM == nil || *out[0].EleM != 100.5 {
		t.Fatalf("value round-trip broken: %v", out[0].EleM)
	}
}
