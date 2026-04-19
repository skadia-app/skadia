package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// --- mockGraphReader ---

type mockGraphReader struct {
	tags2        map[uint64]uint64
	distCm       map[uint64]uint32
	tags1ByHandle map[uint64]uint64
}

func (m *mockGraphReader) EdgeTags2(h uint64) uint64 {
	return m.tags2[h]
}
func (m *mockGraphReader) EdgeTags1(h uint64) uint64 {
	if m.tags1ByHandle != nil {
		return m.tags1ByHandle[h]
	}
	return 0
}
func (m *mockGraphReader) EdgeDistanceCm(h uint64) uint32 {
	return m.distCm[h]
}

// Remaining GraphReader methods are no-op stubs.
func (m *mockGraphReader) ForEachEdge(_ uint64, _ func(routing.EdgeView))    {}
func (m *mockGraphReader) ForEachInEdge(_ uint64, _ func(routing.EdgeView))  {}
func (m *mockGraphReader) NodeCoord(_ uint64) (int32, int32, int32)          { return 0, 0, 0 }
func (m *mockGraphReader) NearestNode(_, _ float64) (uint64, float64, bool)  { return 0, 0, false }
func (m *mockGraphReader) FindNearestEdge(_, _ float64, _ func(uint64) bool) (graph.EdgeProjection, bool) {
	return graph.EdgeProjection{}, false
}
func (m *mockGraphReader) EdgeElevProfile(_ uint64) ([]int32, []uint16) { return nil, nil }
func (m *mockGraphReader) EdgeGeometry(_ uint64) ([]int32, []int32)     { return nil, nil }
func (m *mockGraphReader) EdgeOSMID(_ uint64) int64                     { return 0 }
func (m *mockGraphReader) LandmarkDists(_ routing.LandmarkSetID, _ uint64) []float32 {
	return nil
}
func (m *mockGraphReader) LandmarkDistsInto(_ routing.LandmarkSetID, _ uint64, dst []float32, _ []byte) []float32 {
	return nil
}
func (m *mockGraphReader) LandmarkDistsPageInto(_ routing.LandmarkSetID, _ uint32, _, _ uint32, _ []float32, _ []byte) []float32 {
	return nil
}
func (m *mockGraphReader) LandmarkMeta() *graph.LandmarkMetaData { return nil }
func (m *mockGraphReader) GraphAt(_, _ float64) *graph.Graph     { return nil }
func (m *mockGraphReader) HasTilesFor(_, _, _, _ float64) bool   { return false }

// --- Tests: aggregateLeg2Coverage ---

func TestAggregateLeg2Coverage_EmptyHandles(t *testing.T) {
	r := &mockGraphReader{}
	got := aggregateLeg2Coverage(nil, r)
	if got.DistM != 0 || got.HasAnyCov {
		t.Errorf("empty handles: want zero leg2Coverage, got %+v", got)
	}
	for _, v := range []float32{got.Forest, got.Noise, got.Traffic, got.Protected, got.River} {
		if v != 0 {
			t.Errorf("expected all averages 0, got non-zero in %+v", got)
		}
	}
}

func TestAggregateLeg2Coverage_NoCoverage(t *testing.T) {
	// All edges have tags2 == 0 → no coverage data.
	r := &mockGraphReader{
		tags2:  map[uint64]uint64{1: 0, 2: 0},
		distCm: map[uint64]uint32{1: 1000, 2: 2000},
	}
	got := aggregateLeg2Coverage([]uint64{1, 2}, r)
	if got.HasAnyCov {
		t.Errorf("expected HasAnyCov=false when all tags2=0")
	}
	if got.Forest != 0 || got.Noise != 0 || got.Traffic != 0 || got.Protected != 0 || got.River != 0 {
		t.Errorf("expected all averages 0 when tags2=0, got %+v", got)
	}
	if got.DistM != 30.0 { // (1000+2000)/100
		t.Errorf("expected DistM=30, got %v", got.DistM)
	}
}

func TestAggregateLeg2Coverage_WithForest(t *testing.T) {
	// One edge: ForestPct=50, dist=2000cm (=20m). All other fields 0.
	tags2 := graph.EncodeEdgeTags2(&graph.RawTags{}, &graph.RawEnrichment{ForestPct: 50})
	r := &mockGraphReader{
		tags2:  map[uint64]uint64{1: tags2},
		distCm: map[uint64]uint32{1: 2000},
	}
	got := aggregateLeg2Coverage([]uint64{1}, r)

	if !got.HasAnyCov {
		t.Errorf("expected HasAnyCov=true when tags2 != 0")
	}
	if got.DistM != 20.0 {
		t.Errorf("expected DistM=20, got %v", got.DistM)
	}
	// ForestPct=50 → forestPct = 0.5; weighted average over single edge = 0.5.
	wantForest := float32(0.5)
	if abs32(got.Forest-wantForest) > 1e-5 {
		t.Errorf("Forest average: want %v, got %v", wantForest, got.Forest)
	}
	if got.Noise != 0 || got.Traffic != 0 || got.Protected != 0 || got.River != 0 {
		t.Errorf("expected non-forest dimensions 0, got %+v", got)
	}
}

// --- Tests: mergeCoverage ---

func TestMergeCoverage_ZeroTotal(t *testing.T) {
	got := mergeCoverage(0.5, 0, 0.1, 0)
	if got != 0 {
		t.Errorf("zero total dist: want 0, got %v", got)
	}
}

func TestMergeCoverage_DistanceWeighted(t *testing.T) {
	// l1Avg=0.5, l1Dist=1000; l2Avg=0.1, l2Dist=3000
	// expected = (0.5*1000 + 0.1*3000) / 4000 = (500+300)/4000 = 0.2
	got := mergeCoverage(0.5, 1000, 0.1, 3000)
	want := float32(0.2)
	if abs32(got-want) > 1e-5 {
		t.Errorf("distance-weighted merge: want %v, got %v", want, got)
	}
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// ─── assignLabels tests ───────────────────────────────────────────────────────

func TestAssignLabels_Empty(t *testing.T) {
	got := assignLabels([]RouteCandidate{{}}, []int{}, 10000.0, false)
	if got != nil {
		t.Errorf("expected nil for empty selectedIdx, got %v", got)
	}
}

func TestAssignLabels_K1(t *testing.T) {
	pool := []RouteCandidate{{DistanceM: 10000, AscentM: 200}}
	got := assignLabels(pool, []int{0}, 10000.0, false)
	if len(got) != 1 {
		t.Fatalf("K=1: expected 1 result, got %d", len(got))
	}
	if got[0].VariantID != "only" {
		t.Errorf("K=1: VariantID: want 'only', got %q", got[0].VariantID)
	}
	if got[0].VariantLabelKey != "only" {
		t.Errorf("K=1: VariantLabelKey: want 'only', got %q", got[0].VariantLabelKey)
	}
	if got[0].Descriptors != nil {
		t.Errorf("K=1: Descriptors must be nil, got %v", got[0].Descriptors)
	}
}

func TestAssignLabels_K2(t *testing.T) {
	pool := []RouteCandidate{
		{DistanceM: 8000, AscentM: 100},
		{DistanceM: 12000, AscentM: 100},
	}
	got := assignLabels(pool, []int{0, 1}, 10000.0, false)
	if len(got) != 2 {
		t.Fatalf("K=2: expected 2 results, got %d", len(got))
	}
	if got[0].VariantID != "shortest" {
		t.Errorf("K=2[0]: want 'shortest', got %q", got[0].VariantID)
	}
	if got[1].VariantID != "longest" {
		t.Errorf("K=2[1]: want 'longest', got %q", got[1].VariantID)
	}
}

func TestAssignLabels_K2_InputUnsorted(t *testing.T) {
	// selectedIdx is unsorted — output must still be sorted by DistanceM.
	pool := []RouteCandidate{
		{DistanceM: 15000, AscentM: 100},
		{DistanceM: 9000, AscentM: 100},
	}
	got := assignLabels(pool, []int{0, 1}, 10000.0, false)
	if got[0].VariantID != "shortest" || got[0].Candidate.DistanceM != 9000 {
		t.Errorf("K=2 unsorted: first result should be shortest (9000m), got %+v", got[0])
	}
	if got[1].VariantID != "longest" || got[1].Candidate.DistanceM != 15000 {
		t.Errorf("K=2 unsorted: second result should be longest (15000m), got %+v", got[1])
	}
}

func TestAssignLabels_K3(t *testing.T) {
	// Indices unsorted on purpose; pool[2]=8000, pool[0]=10000, pool[1]=14000.
	pool := []RouteCandidate{
		{DistanceM: 10000, AscentM: 100},
		{DistanceM: 14000, AscentM: 100},
		{DistanceM: 8000, AscentM: 100},
	}
	got := assignLabels(pool, []int{2, 0, 1}, 10000.0, false)
	if len(got) != 3 {
		t.Fatalf("K=3: expected 3 results, got %d", len(got))
	}
	want := []struct{ id string; dist float64 }{
		{"shortest", 8000},
		{"medium", 10000},
		{"longest", 14000},
	}
	for i, w := range want {
		if got[i].VariantID != w.id {
			t.Errorf("K=3[%d]: VariantID want %q, got %q", i, w.id, got[i].VariantID)
		}
		if got[i].Candidate.DistanceM != w.dist {
			t.Errorf("K=3[%d]: DistanceM want %v, got %v", i, w.dist, got[i].Candidate.DistanceM)
		}
	}
}

func TestAssignLabels_NoCoverage_NoCoverageDescriptors(t *testing.T) {
	// coverageAvailable=false; even though routes have high forest values,
	// no coverage descriptor should be emitted.
	pool := []RouteCandidate{
		{DistanceM: 8000, AscentM: 100, MergedForestPct: 0.8, HasCoverage: true},
		{DistanceM: 10000, AscentM: 100, MergedForestPct: 0.1, HasCoverage: true},
		{DistanceM: 12000, AscentM: 100, MergedForestPct: 0.1, HasCoverage: true},
	}
	got := assignLabels(pool, []int{0, 1, 2}, 10000.0, false /* coverageAvailable=false */)
	for _, lr := range got {
		for _, d := range lr.Descriptors {
			if d.Key == "high_forest" || d.Key == "high_water" || d.Key == "high_protected" || d.Key == "low_traffic_noise" {
				t.Errorf("coverage descriptor %q emitted despite coverageAvailable=false", d.Key)
			}
		}
	}
}

func TestAssignLabels_MixedCoverage(t *testing.T) {
	// Route 0: no coverage, high ascent (300m vs 100m others).
	// Routes 1+2: have coverage, lower ascent.
	// Route 0 should get high_ascent but NOT coverage descriptors.
	pool := []RouteCandidate{
		{DistanceM: 8000, AscentM: 300, HasCoverage: false, MergedForestPct: 0.9},
		{DistanceM: 10000, AscentM: 100, HasCoverage: true, MergedForestPct: 0.1},
		{DistanceM: 12000, AscentM: 100, HasCoverage: true, MergedForestPct: 0.1},
	}
	got := assignLabels(pool, []int{0, 1, 2}, 10000.0, true /* coverageAvailable=true */)

	// Find the LabeledRoute for pool[0] (shortest = 8000m).
	var shortest *LabeledRoute
	for i := range got {
		if got[i].Candidate.DistanceM == 8000 {
			shortest = &got[i]
			break
		}
	}
	if shortest == nil {
		t.Fatal("could not find shortest route in output")
	}

	hasHighAscent := false
	for _, d := range shortest.Descriptors {
		if d.Key == "high_ascent" {
			hasHighAscent = true
		}
		if d.Key == "high_forest" || d.Key == "high_water" || d.Key == "high_protected" || d.Key == "low_traffic_noise" {
			t.Errorf("route with HasCoverage=false emitted coverage descriptor %q", d.Key)
		}
	}
	if !hasHighAscent {
		t.Errorf("route with AscentM=300 vs others=100 should have high_ascent descriptor")
	}
}

// ─── computeDescriptorsFor tests ─────────────────────────────────────────────

func TestComputeDescriptorsFor_HighAscentThreshold(t *testing.T) {
	me := RouteCandidate{DistanceM: 10000, AscentM: 200, HasCoverage: false}
	others := []RouteCandidate{
		{DistanceM: 10000, AscentM: 100},
		{DistanceM: 10000, AscentM: 150},
	}
	// delta=200-150=50 >= 30, ratio=200/150=1.33 >= 1.2 → high_ascent expected.
	descs := computeDescriptorsFor(me, others, 10000, false)
	found := hasDescriptor(descs, "high_ascent")
	if !found {
		t.Errorf("expected high_ascent descriptor, got %v", descs)
	}

	// me.AscentM=120, maxOther=150: delta=-30, ratio<1 → NO high_ascent.
	me2 := RouteCandidate{DistanceM: 10000, AscentM: 120, HasCoverage: false}
	descs2 := computeDescriptorsFor(me2, others, 10000, false)
	if hasDescriptor(descs2, "high_ascent") {
		t.Errorf("did NOT expect high_ascent when me<others, got %v", descs2)
	}
}

func TestComputeDescriptorsFor_HighAscentBoundary_30m(t *testing.T) {
	others := []RouteCandidate{{DistanceM: 10000, AscentM: 100}}
	// delta=30 exactly → should trigger (>= 30), ratio=130/100=1.3 >= 1.2.
	me130 := RouteCandidate{DistanceM: 10000, AscentM: 130}
	if !hasDescriptor(computeDescriptorsFor(me130, others, 10000, false), "high_ascent") {
		t.Error("AscentM=130 vs 100: delta=30 should produce high_ascent")
	}

	// delta=29 → should NOT trigger.
	me129 := RouteCandidate{DistanceM: 10000, AscentM: 129}
	if hasDescriptor(computeDescriptorsFor(me129, others, 10000, false), "high_ascent") {
		t.Error("AscentM=129 vs 100: delta=29 should NOT produce high_ascent")
	}
}

func TestComputeDescriptorsFor_HighForestBoundary(t *testing.T) {
	others := []RouteCandidate{{
		DistanceM:       10000,
		MergedForestPct: 0.45,
		HasCoverage:     true,
	}}

	// delta=0.10 exactly → should produce high_forest.
	me55 := RouteCandidate{DistanceM: 10000, MergedForestPct: 0.55, HasCoverage: true}
	if !hasDescriptor(computeDescriptorsFor(me55, others, 10000, true), "high_forest") {
		t.Error("ForestPct=0.55 vs 0.45: delta=0.10 should produce high_forest")
	}

	// delta=0.09 → should NOT produce high_forest.
	me54 := RouteCandidate{DistanceM: 10000, MergedForestPct: 0.54, HasCoverage: true}
	if hasDescriptor(computeDescriptorsFor(me54, others, 10000, true), "high_forest") {
		t.Error("ForestPct=0.54 vs 0.45: delta=0.09 should NOT produce high_forest")
	}
}

func TestComputeDescriptorsFor_TargetDistance(t *testing.T) {
	target := 10000.0
	me := RouteCandidate{DistanceM: 10200}

	// K=2 (len(others)==1): target_distance is suppressed — comparison is
	// meaningless with only one sibling (F23).
	others1 := []RouteCandidate{{DistanceM: 11000}}
	if hasDescriptor(computeDescriptorsFor(me, others1, target, false), "target_distance") {
		t.Error("K=2: target_distance must be suppressed when len(others) < 2")
	}

	// K=3 (len(others)==2): me_delta=200, within 5% of 10000 (<=500).
	// min(other_delta)=min(1000,4000)=1000 > 2*200=400 → target_distance expected.
	others2 := []RouteCandidate{{DistanceM: 11000}, {DistanceM: 14000}}
	if !hasDescriptor(computeDescriptorsFor(me, others2, target, false), "target_distance") {
		t.Error("K=3: 10200 vs target 10000 with distant others should get target_distance")
	}

	// K=3: me=10200, others 10300 and 10400. min_other_delta=300 NOT > 2*200=400 → no target_distance.
	others3 := []RouteCandidate{{DistanceM: 10300}, {DistanceM: 10400}}
	if hasDescriptor(computeDescriptorsFor(me, others3, target, false), "target_distance") {
		t.Error("K=3: other not 2x farther, should NOT get target_distance")
	}
}

func TestComputeDescriptorsFor_EmptyOthers(t *testing.T) {
	me := RouteCandidate{DistanceM: 10000, AscentM: 500}
	got := computeDescriptorsFor(me, nil, 10000, true)
	if got != nil {
		t.Errorf("empty others: expected nil descriptors, got %v", got)
	}
}

func TestAssignLabels_K3_TargetDistanceDescriptor(t *testing.T) {
	// pool[0]=9000m, pool[1]=10200m (closest to 10000 target), pool[2]=14000m.
	// pool[1]: meDelta=200, within 5% of 10000 (<=500). others: delta(9000)=1000, delta(14000)=4000.
	// minOtherDistDelta=1000 > 2*200=400 → pool[1] (medium slot) gets target_distance.
	pool := []RouteCandidate{
		{DistanceM: 9000, AscentM: 100},
		{DistanceM: 10200, AscentM: 100},
		{DistanceM: 14000, AscentM: 100},
	}
	got := assignLabels(pool, []int{0, 1, 2}, 10000.0, false)
	if len(got) != 3 {
		t.Fatalf("K=3 target_distance: expected 3 results, got %d", len(got))
	}

	// Find the medium route (pool[1], 10200m).
	var medium *LabeledRoute
	for i := range got {
		if got[i].VariantID == "medium" {
			medium = &got[i]
			break
		}
	}
	if medium == nil {
		t.Fatal("K=3 target_distance: no 'medium' route found")
	}
	if medium.Candidate.DistanceM != 10200 {
		t.Errorf("K=3 target_distance: medium DistanceM want 10200, got %v", medium.Candidate.DistanceM)
	}
	if !hasDescriptor(medium.Descriptors, "target_distance") {
		t.Errorf("K=3 target_distance: pool[1] (10200m) should have target_distance descriptor, got %v", medium.Descriptors)
	}

	// Other routes must NOT have target_distance.
	for _, lr := range got {
		if lr.VariantID == "medium" {
			continue
		}
		if hasDescriptor(lr.Descriptors, "target_distance") {
			t.Errorf("K=3 target_distance: route %q (%.0fm) should NOT have target_distance", lr.VariantID, lr.Candidate.DistanceM)
		}
	}
}

func TestAssignLabels_K4_MediumNumbering(t *testing.T) {
	// 4 candidates: shortest, medium-2, medium-3, longest.
	pool := []RouteCandidate{
		{DistanceM: 5000, AscentM: 100},
		{DistanceM: 8000, AscentM: 100},
		{DistanceM: 11000, AscentM: 100},
		{DistanceM: 15000, AscentM: 100},
	}
	got := assignLabels(pool, []int{0, 1, 2, 3}, 10000.0, false)
	if len(got) != 4 {
		t.Fatalf("K=4 numbering: expected 4 results, got %d", len(got))
	}

	wantIDs := []string{"shortest", "medium-2", "medium-3", "longest"}
	for i, want := range wantIDs {
		if got[i].VariantID != want {
			t.Errorf("K=4 numbering[%d]: VariantID want %q, got %q", i, want, got[i].VariantID)
		}
	}
}

// hasDescriptor reports whether any VariantDescriptor in ds has the given key.
func hasDescriptor(ds []VariantDescriptor, key string) bool {
	for _, d := range ds {
		if d.Key == key {
			return true
		}
	}
	return false
}
