package routing

import (
	"math"
	"testing"
)

// fakeElevReader satisfies the graphReaderForElev interface (see implementation).
// NOT to be confused with the full GraphReader — we mock only the two methods
// buildElevationProfile needs.
type fakeElevReader struct {
	edges map[uint64]fakeElevEdge
}

type fakeElevEdge struct {
	elevs      []int32  // decimeters
	segDistsCm []uint16 // cm (reader-native type!)
	distCm     uint32   // cm (reader-native type!)
}

func (r *fakeElevReader) EdgeElevProfile(h uint64) ([]int32, []uint16) {
	e := r.edges[h]
	return e.elevs, e.segDistsCm
}

func (r *fakeElevReader) EdgeDistanceCm(h uint64) uint32 {
	return r.edges[h].distCm
}

func assertInDelta(t *testing.T, want, got, delta float64, msg string) {
	t.Helper()
	if math.Abs(want-got) > delta {
		t.Errorf("%s: want %v ± %v, got %v", msg, want, delta, got)
	}
}

func TestBuildElevationProfile_LinearAscent(t *testing.T) {
	reader := &fakeElevReader{
		edges: map[uint64]fakeElevEdge{
			100: {elevs: []int32{5930, 6010, 6200, 18910}, segDistsCm: []uint16{3000, 4000, 5000}, distCm: 12000},
		},
	}
	profile, startD, endD := buildElevationProfile(reader, []uint64{100})
	if len(profile) != 4 {
		t.Fatalf("want 4 profile points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0].DistM, 0.01, "profile[0].DistM")
	assertInDelta(t, 30.0, profile[1].DistM, 0.01, "profile[1].DistM")
	assertInDelta(t, 70.0, profile[2].DistM, 0.01, "profile[2].DistM")
	assertInDelta(t, 120.0, profile[3].DistM, 0.01, "profile[3].DistM")
	assertInDelta(t, 593.0, *profile[0].EleM, 0.01, "profile[0].EleM")
	assertInDelta(t, 1891.0, *profile[3].EleM, 0.01, "profile[3].EleM")
	if len(startD) != 1 || startD[0] != 0.0 {
		t.Errorf("want startD=[0.0], got %v", startD)
	}
	if len(endD) != 1 {
		t.Fatalf("want endD len 1, got %d", len(endD))
	}
	assertInDelta(t, 120.0, endD[0], 0.01, "endD[0]")
}

func TestBuildElevationProfile_MultiEdgeJunctionDedup(t *testing.T) {
	reader := &fakeElevReader{
		edges: map[uint64]fakeElevEdge{
			100: {elevs: []int32{5930, 6100}, segDistsCm: []uint16{5000}, distCm: 5000},
			200: {elevs: []int32{6100, 6500}, segDistsCm: []uint16{4000}, distCm: 4000},
		},
	}
	profile, _, _ := buildElevationProfile(reader, []uint64{100, 200})
	// Edge 100: 2 points; Edge 200 first elev dropped (Junction-Dedup),
	// but distance 4000cm = 40m STILL added → strict monotonic + 3 total
	if len(profile) != 3 {
		t.Fatalf("want 3 profile points, got %d", len(profile))
	}
	assertInDelta(t, 0.0, profile[0].DistM, 0.01, "profile[0].DistM")
	assertInDelta(t, 50.0, profile[1].DistM, 0.01, "profile[1].DistM")
	assertInDelta(t, 90.0, profile[2].DistM, 0.01, "profile[2].DistM") // 50 + 40 (segDists[0] of edge 200)
	assertInDelta(t, 593.0, *profile[0].EleM, 0.01, "profile[0].EleM")
	assertInDelta(t, 610.0, *profile[1].EleM, 0.01, "profile[1].EleM")
	assertInDelta(t, 650.0, *profile[2].EleM, 0.01, "profile[2].EleM")
}

func TestBuildElevationProfile_EmptyDEM_ContinuesDistance(t *testing.T) {
	reader := &fakeElevReader{
		edges: map[uint64]fakeElevEdge{
			100: {elevs: []int32{5930, 6100}, segDistsCm: []uint16{5000}, distCm: 5000},
			200: {elevs: nil, segDistsCm: nil, distCm: 4000}, // DEM-loss edge
			300: {elevs: []int32{6100, 6500}, segDistsCm: []uint16{4000}, distCm: 4000},
		},
	}
	profile, _, endD := buildElevationProfile(reader, []uint64{100, 200, 300})
	assertInDelta(t, 50.0, endD[0], 0.01, "endD[0]")
	assertInDelta(t, 90.0, endD[1], 0.01, "endD[1] (distance gap of 40m)")
	assertInDelta(t, 130.0, endD[2], 0.01, "endD[2]")
	if len(profile) < 2 {
		t.Errorf("want at least 2 profile points, got %d", len(profile))
	}
}

func TestBuildElevationProfile_NilSegDistsWithElevsGreater1_DistanceContinues(t *testing.T) {
	// Guard 3 (Critic-Finding #20): segDists==nil but elevs>1 → unexpected
	// tile state. Helper logs Warn, emits no profile append, but
	// edgeEndDistM advances via EdgeDistanceCm (axis continuity §P5).
	reader := &fakeElevReader{
		edges: map[uint64]fakeElevEdge{
			100: {elevs: []int32{5930, 6100}, segDistsCm: nil, distCm: 5000},
		},
	}
	profile, _, endD := buildElevationProfile(reader, []uint64{100})
	if len(profile) != 0 {
		t.Errorf("Guard 3 must not emit profile, got %d points", len(profile))
	}
	assertInDelta(t, 50.0, endD[0], 0.01, "distance axis must advance via EdgeDistanceCm")
}

func TestBuildElevationProfile_SinglePointEdge(t *testing.T) {
	reader := &fakeElevReader{
		edges: map[uint64]fakeElevEdge{
			100: {elevs: []int32{5930}, segDistsCm: nil, distCm: 5000},
		},
	}
	profile, _, endD := buildElevationProfile(reader, []uint64{100})
	if len(profile) != 1 {
		t.Fatalf("want 1 profile point, got %d", len(profile))
	}
	assertInDelta(t, 593.0, *profile[0].EleM, 0.01, "profile[0].EleM")
	assertInDelta(t, 50.0, endD[0], 0.01, "endD[0]")
}

func TestInterpolateProfileEleAt_EdgeCases(t *testing.T) {
	// Empty profile → 0
	assertInDelta(t, 0.0, InterpolateProfileEleAt(nil, 50), 0.0001, "nil profile")
	assertInDelta(t, 0.0, InterpolateProfileEleAt([]ElevationPoint{}, 50), 0.0001, "empty profile")

	// Single-point profile clamps both ways
	single := []ElevationPoint{{DistM: 100, EleM: PtrEle(500)}}
	assertInDelta(t, 500.0, InterpolateProfileEleAt(single, 0), 0.0001, "single-point clamp-start")
	assertInDelta(t, 500.0, InterpolateProfileEleAt(single, 9999), 0.0001, "single-point clamp-end")

	p := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 100, EleM: PtrEle(200)},
		{DistM: 250, EleM: PtrEle(350)},
	}
	// Clamp-start
	assertInDelta(t, 100.0, InterpolateProfileEleAt(p, -10), 0.0001, "clamp-start negative")
	assertInDelta(t, 100.0, InterpolateProfileEleAt(p, 0), 0.0001, "clamp-start zero")
	// Clamp-end
	assertInDelta(t, 350.0, InterpolateProfileEleAt(p, 9999), 0.0001, "clamp-end")
	// Exact sample
	assertInDelta(t, 200.0, InterpolateProfileEleAt(p, 100), 0.0001, "exact sample at 100")
	// Midpoint between sample 0/1 (50% of [0..100], 50% of [100..200] = 150)
	assertInDelta(t, 150.0, InterpolateProfileEleAt(p, 50), 0.0001, "midpoint 0/1")
	// Two-thirds between sample 1/2 (100m through a 150m span = 200 + 0.667*150 = 300)
	assertInDelta(t, 300.0, InterpolateProfileEleAt(p, 200), 0.0001, "two-thirds 1/2")
}

func TestApplyProfileToCoords_OverwritesEleInPlace(t *testing.T) {
	// Coords spaced ~111.32m apart (1° lat at equator) so distances are predictable.
	coords := []LatLonEle{
		{Lat: 0.0, Lon: 0.0, Ele: 999},   // pre-populated junk, must be overwritten
		{Lat: 0.001, Lon: 0.0, Ele: 999}, // ~111.32m
		{Lat: 0.002, Lon: 0.0, Ele: 999}, // ~222.64m
	}
	profile := []ElevationPoint{
		{DistM: 0, EleM: PtrEle(100)},
		{DistM: 222.64, EleM: PtrEle(300)},
	}
	applyProfileToCoords(coords, profile)
	assertInDelta(t, 100.0, coords[0].Ele, 0.5, "coords[0].Ele")
	assertInDelta(t, 200.0, coords[1].Ele, 5.0, "coords[1].Ele (~midpoint)")
	assertInDelta(t, 300.0, coords[2].Ele, 0.5, "coords[2].Ele")
}

func TestApplyProfileToCoords_NoOpsOnEmpty(t *testing.T) {
	// Both nil-profile and empty-coords are no-ops, must not panic.
	coords := []LatLonEle{{Lat: 0, Lon: 0, Ele: 42}}
	applyProfileToCoords(coords, nil)
	assertInDelta(t, 42.0, coords[0].Ele, 0.0001, "nil-profile no-op")

	applyProfileToCoords(nil, []ElevationPoint{{DistM: 0, EleM: PtrEle(100)}}) // must not panic
}
