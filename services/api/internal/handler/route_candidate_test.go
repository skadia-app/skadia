package handler

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing"
	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// --- mockGraphReader ---

type mockGraphReader struct {
	tags2  map[uint64]uint64
	distCm map[uint64]uint32
}

func (m *mockGraphReader) EdgeTags2(h uint64) uint64 {
	return m.tags2[h]
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
