package enrichment

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/routing/graph"
)

// makeCandidate is a convenience helper that creates an EdgeCandidate with the
// given fields. All other fields are zero.
func makeCandidate(edgeIdx, nodeFrom, nodeTo uint32, distM float64, surface, highway string) graph.EdgeCandidate {
	return graph.EdgeCandidate{
		EdgeIdx:  edgeIdx,
		NodeFrom: nodeFrom,
		NodeTo:   nodeTo,
		DistM:    distM,
		Surface:  surface,
		Highway:  highway,
	}
}

// makePts returns n uniformly-spaced struct{lat,lng} points along a horizontal
// line starting at (lat0, lng0), each step advancing by stepDeg in longitude.
func makePts(n int, lat0, lng0, stepDeg float64) []struct{ lat, lng float64 } {
	pts := make([]struct{ lat, lng float64 }, n)
	for i := range n {
		pts[i] = struct{ lat, lng float64 }{lat0, lng0 + float64(i)*stepDeg}
	}
	return pts
}

// --- viterbiMatch tests ---

// TestViterbiMatch_AllOffroad verifies that when every point has nil/empty
// candidates the result is all zero-value EdgeCandidates (Surface == "").
func TestViterbiMatch_AllOffroad(t *testing.T) {
	pts := makePts(5, 46.0, 14.0, 0.0001)
	candidates := make([][]graph.EdgeCandidate, len(pts))
	// All nil — every point is offroad.

	result := viterbiMatch(pts, candidates)

	if len(result) != len(pts) {
		t.Fatalf("result length = %d, want %d", len(result), len(pts))
	}
	for i, ec := range result {
		if ec.Surface != "" {
			t.Errorf("point %d: Surface = %q, want \"\" (zero-value / offroad)", i, ec.Surface)
		}
	}
}

// TestViterbiMatch_SingleEdge verifies that when all points have a single
// identical candidate the algorithm matches all of them to that edge.
func TestViterbiMatch_SingleEdge(t *testing.T) {
	pts := makePts(3, 46.0, 14.0, 0.0001)
	ec := makeCandidate(1, 10, 11, 3.0, "asphalt", "residential")
	candidates := [][]graph.EdgeCandidate{
		{ec},
		{ec},
		{ec},
	}

	result := viterbiMatch(pts, candidates)

	if len(result) != 3 {
		t.Fatalf("result length = %d, want 3", len(result))
	}
	for i, r := range result {
		if r.Surface != "asphalt" {
			t.Errorf("point %d: Surface = %q, want \"asphalt\"", i, r.Surface)
		}
	}
}

// TestViterbiMatch_OffroadGapReinit verifies the re-initialisation behaviour:
// points 0–2 are on-road (gravel), 3–8 are offroad (nil candidates), 9–11
// are on-road again. After a gap of 6 (>5) the algorithm re-inits from
// emission only, so point 9 must be matched (Surface != "") and a mid-gap
// point (5) must remain offroad (Surface == "").
func TestViterbiMatch_OffroadGapReinit(t *testing.T) {
	n := 12
	pts := makePts(n, 46.0, 14.0, 0.0001)

	onRoadEC := makeCandidate(2, 20, 21, 4.0, "gravel", "track")
	candidates := make([][]graph.EdgeCandidate, n)
	for i := range 3 {
		candidates[i] = []graph.EdgeCandidate{onRoadEC}
	}
	// 3–8: nil (offroad gap of 6 points)
	for i := 9; i < 12; i++ {
		candidates[i] = []graph.EdgeCandidate{onRoadEC}
	}

	result := viterbiMatch(pts, candidates)

	if len(result) != n {
		t.Fatalf("result length = %d, want %d", len(result), n)
	}

	// Mid-gap point must be offroad.
	if result[5].Surface != "" {
		t.Errorf("point 5 (mid-gap): Surface = %q, want \"\" (offroad)", result[5].Surface)
	}

	// Post-gap point must be matched (re-init from emission).
	if result[9].Surface != "gravel" {
		t.Errorf("point 9 (post-gap re-init): Surface = %q, want \"gravel\"", result[9].Surface)
	}
}

// TestViterbiMatch_MultiSegmentBacktrack verifies that the backtrack correctly
// traces through multiple re-init boundaries. Three on-road segments separated
// by two offroad gaps (each >5 points) must all be matched correctly, and the
// gap points must remain offroad.
func TestViterbiMatch_MultiSegmentBacktrack(t *testing.T) {
	// 3 on-road segments separated by 2 offroad gaps (each >5 points)
	// Segment A: 0-4 (edge1), Gap: 5-11, Segment B: 12-16 (edge2), Gap: 17-23, Segment C: 24-28 (edge3)
	n := 29
	pts := make([]struct{ lat, lng float64 }, n)
	for i := range n {
		pts[i] = struct{ lat, lng float64 }{47.0, 11.0 + float64(i)*0.0001}
	}

	edge1 := graph.EdgeCandidate{EdgeIdx: 1, NodeFrom: 10, NodeTo: 11, DistM: 3.0, Surface: "asphalt", Highway: "residential"}
	edge2 := graph.EdgeCandidate{EdgeIdx: 2, NodeFrom: 20, NodeTo: 21, DistM: 4.0, Surface: "gravel", Highway: "track"}
	edge3 := graph.EdgeCandidate{EdgeIdx: 3, NodeFrom: 30, NodeTo: 31, DistM: 2.0, Surface: "ground", Highway: "path"}

	candidates := make([][]graph.EdgeCandidate, n)
	// Segment A
	for i := range 5 {
		candidates[i] = []graph.EdgeCandidate{edge1}
	}
	// Gap 5-11: nil (offroad)
	// Segment B
	for i := 12; i < 17; i++ {
		candidates[i] = []graph.EdgeCandidate{edge2}
	}
	// Gap 17-23: nil (offroad)
	// Segment C
	for i := 24; i < 29; i++ {
		candidates[i] = []graph.EdgeCandidate{edge3}
	}

	result := viterbiMatch(pts, candidates)

	// Segment A (0-4) must be matched to edge1 (asphalt), NOT offroad
	if result[2].Surface != "asphalt" {
		t.Errorf("segment A point 2: got surface=%q, want asphalt", result[2].Surface)
	}
	// Segment B (12-16) must be matched to edge2 (gravel)
	if result[14].Surface != "gravel" {
		t.Errorf("segment B point 14: got surface=%q, want gravel", result[14].Surface)
	}
	// Segment C (24-28) must be matched to edge3 (ground)
	if result[26].Surface != "ground" {
		t.Errorf("segment C point 26: got surface=%q, want ground", result[26].Surface)
	}
	// Offroad gaps must remain unmatched
	if result[8].Surface != "" {
		t.Errorf("offroad gap point 8: got surface=%q, want empty", result[8].Surface)
	}
}

// --- resampleStreamPoints tests ---

// TestResampleStreamPoints_Basic verifies that a stream of 10 GPS points with
// ~100 m spacing produces at least 5 resampled output points.
func TestResampleStreamPoints_Basic(t *testing.T) {
	// Build 10 EnrichmentStreamPoints spaced ~100 m apart along a latitude line.
	// 0.001° longitude ≈ 71 m at 46° — use 0.0014° ≈ 100 m.
	n := 10
	stream := make([]EnrichmentStreamPoint, n)
	for i := range n {
		lat := 46.0
		lng := 14.0 + float64(i)*0.0014
		stream[i] = EnrichmentStreamPoint{Lat: &lat, Lng: &lng}
	}

	pts := resampleStreamPoints(stream, surfaceResampleM)

	if len(pts) < 5 {
		t.Errorf("resampleStreamPoints returned %d points, want >= 5", len(pts))
	}
}

// TestResampleStreamPoints_NilGPSFiltered verifies that points without GPS are
// skipped and the remaining valid points are still resampled correctly.
func TestResampleStreamPoints_NilGPSFiltered(t *testing.T) {
	lat1, lng1 := 46.0, 14.0
	lat2, lng2 := 46.001, 14.001 // ~130 m

	stream := []EnrichmentStreamPoint{
		{Lat: &lat1, Lng: &lng1},
		{Lat: nil, Lng: nil}, // no GPS — must be skipped
		{Lat: &lat2, Lng: &lng2},
	}

	pts := resampleStreamPoints(stream, surfaceResampleM)

	// Must produce at least 2 points (the minimum guarantee).
	if len(pts) < 2 {
		t.Errorf("expected >= 2 resampled points, got %d", len(pts))
	}
}

// --- equirectDistMeters tests ---

// TestEquirectDistMeters verifies that two points 0.001° latitude apart
// (at the same longitude) are approximately 111 m distant.
func TestEquirectDistMeters(t *testing.T) {
	d := equirectDistMeters(46.0, 14.0, 46.001, 14.0)
	if d < 100 || d > 120 {
		t.Errorf("equirectDistMeters = %.2f m, want 100–120 m", d)
	}
}

// TestEquirectDistMeters_Zero verifies that identical points have zero distance.
func TestEquirectDistMeters_Zero(t *testing.T) {
	d := equirectDistMeters(46.0, 14.0, 46.0, 14.0)
	if d != 0 {
		t.Errorf("equirectDistMeters(same point) = %.6f, want 0", d)
	}
}
