package routing

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

type segMockReader struct {
	eles []int32
}

func (r *segMockReader) NodeCoord(nodeID uint64) (lat, lon, ele int32) {
	_, localID := SplitGlobalNodeID(nodeID)
	if int(localID) < len(r.eles) {
		return 0, 0, r.eles[localID]
	}
	return 0, 0, 0
}

func (r *segMockReader) ForEachEdge(_ uint64, _ func(EdgeView))           {}
func (r *segMockReader) ForEachInEdge(_ uint64, _ func(EdgeView))         {}
func (r *segMockReader) NearestNode(_, _ float64) (uint64, float64, bool) { return 0, 0, false }
func (r *segMockReader) EdgeElevProfile(_ uint64) ([]int32, []uint16)     { return nil, nil }
func (r *segMockReader) EdgeGeometry(_ uint64) ([]int32, []int32)         { return nil, nil }
func (r *segMockReader) EdgeOSMID(_ uint64) int64                         { return 0 }
func (r *segMockReader) LandmarkDists(_ LandmarkSetID, _ uint64) []float32 {
	return nil
}
func (r *segMockReader) LandmarkDistsInto(_ LandmarkSetID, _ uint64, _ []float32, _ []byte) []float32 {
	return nil
}
func (r *segMockReader) LandmarkDistsPageInto(_ LandmarkSetID, _ uint32, _, _ uint32, _ []float32, _ []byte) []float32 {
	return nil
}
func (r *segMockReader) LandmarkMeta() *graph.LandmarkMetaData { return nil }
func (r *segMockReader) GraphAt(_, _ float64) *graph.Graph     { return nil }
func (r *segMockReader) HasTilesFor(_, _, _, _ float64) bool   { return false }

func TestFindValleySegmentsTwoValleys(t *testing.T) {
	// Profile: pass → valley → pass → valley → pass (starts and ends high).
	// This ensures both leftMax and rightMax include a high pass for each valley
	// floor, so the plausibility check (<80% of neighborPeak) passes.
	// windowSize = min(40/4, 20) = 10.
	// Valley floors are at ~1000m, passes at 3000m.
	// Drain check: threshold = 1.2*1000 = 1200. Within 10 nodes of each valley
	// floor there are only 1000m nodes → drain check passes.
	eles := []int32{
		// idx 0-3: first pass
		3000, 3000, 3000, 3000,
		// idx 4-15: first valley floor (12 flat nodes)
		1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000,
		// idx 16-19: second pass
		3000, 3000, 3000, 3000,
		// idx 20-31: second valley floor (12 flat nodes)
		1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000, 1000,
		// idx 32-39: third pass / end
		3000, 3000, 3000, 3000, 3000, 3000, 3000, 3000,
	}
	reader := &segMockReader{eles: eles}
	path := make([]uint64, len(eles))
	for i := range len(eles) {
		path[i] = MakeGlobalNodeID(1, uint32(i))
	}
	segments := FindValleySegments(path, reader, 0)
	if len(segments) < 2 {
		t.Errorf("got %d segments, want >= 2", len(segments))
	}
}

func TestFindValleySegmentsFlatTerrain(t *testing.T) {
	eles := make([]int32, 15)
	for i := range 15 {
		eles[i] = 1000
	}
	reader := &segMockReader{eles: eles}
	path := make([]uint64, 15)
	for i := range 15 {
		path[i] = MakeGlobalNodeID(1, uint32(i))
	}
	segments := FindValleySegments(path, reader, 0)
	if len(segments) != 0 {
		t.Errorf("got %d segments on flat terrain, want 0", len(segments))
	}
}

func TestFindValleySegmentsShortPath(t *testing.T) {
	reader := &segMockReader{eles: []int32{1000, 2000, 1000}}
	path := []uint64{MakeGlobalNodeID(1, 0), MakeGlobalNodeID(1, 1), MakeGlobalNodeID(1, 2)}
	segments := FindValleySegments(path, reader, 0)
	if segments != nil {
		t.Error("expected nil for short path")
	}
}
