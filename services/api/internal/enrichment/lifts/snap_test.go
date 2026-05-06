package lifts

import (
	"testing"
	"time"
)

// TestSnapToOSMLifts_FullSnap: Stage 1 (conservative snap) — one heuristic
// ride covers >= 70% of one OSM-lift geometry → Action="snap", IsReverse
// derived from bearing comparison, segment_effort_id contract via SnapResult.
func TestSnapToOSMLifts_FullSnap(t *testing.T) {
	// Synthetic 600m east-going lift, 600m duration, ascent 100m.
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0 /*startX*/, 600 /*endX*/, 0 /*startY*/, 1000.0, 1100.0)

	osmLift := osmLiftCandidate{
		SegmentID:        "seg-A",
		SegmentVersionID: "ver-A",
		LengthM:          600.0,
		BearingDeg:       90.0, // east
	}
	// Matchable: hausdorff 5m, intersection covers all 600m of C → coverage = 1.0
	matches := []rideOSMOverlap{
		{
			RideIdx:        0,
			OSMIdx:         0,
			HausdorffM:     5.0,
			IntersectionLen: 600.0,
		},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{osmLift}, matches, nil, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Action != SnapActionSnap {
		t.Errorf("Action = %q, want %q", r.Action, SnapActionSnap)
	}
	if r.SegmentID != "seg-A" {
		t.Errorf("SegmentID = %q, want seg-A", r.SegmentID)
	}
	if r.IsCableHoldMerge {
		t.Error("expected IsCableHoldMerge=false for single-ride snap")
	}
	if r.IsMethodAOnly {
		t.Error("expected IsMethodAOnly=false for stage 1")
	}
	if r.IsReverse {
		t.Error("expected IsReverse=false (ride east-going matches lift east-going)")
	}
}

// TestSnapToOSMLifts_NoSnapWhenCoverageBelow30: when no OSM lift reaches 30%
// coverage, the ride passes through unchanged (Action="passthrough").
func TestSnapToOSMLifts_NoSnapWhenCoverageBelow30(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)

	osmLift := osmLiftCandidate{
		SegmentID:        "seg-B",
		SegmentVersionID: "ver-B",
		LengthM:          5000.0, // very long lift, ride only covers tiny fraction
		BearingDeg:       90.0,
	}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 200.0 /*too far*/, IntersectionLen: 100.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{osmLift}, matches, nil, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Action != SnapActionPassthrough {
		t.Errorf("Action = %q, want %q (ride should pass-through to heuristic-only candidate)", r.Action, SnapActionPassthrough)
	}
}

// TestSnapToOSMLifts_PartialCoverageMarksMultiRideGroup: coverage 0.30..0.70
// AND a sibling ride also covers the same OSM lift → ride is marked as
// candidate for Stage 2 (Action="multi_ride_group", carrying the OSM idx).
func TestSnapToOSMLifts_PartialCoverageMarksMultiRideGroup(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)
	rideB := newSyntheticRide(t, startTime.Add(180*time.Second), 320, 600, 0, 1052.0, 1100.0)

	osmLift := osmLiftCandidate{
		SegmentID:        "seg-C",
		SegmentVersionID: "ver-C",
		LengthM:          600.0,
		BearingDeg:       90.0,
	}
	// Each ride covers 300m of a 600m lift = 0.50 coverage individually.
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{rideA.indexed, rideB.indexed}, []osmLiftCandidate{osmLift}, matches, nil, nil)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Action != SnapActionMultiRideGroup {
			t.Errorf("ride[%d] Action = %q, want %q", i, r.Action, SnapActionMultiRideGroup)
		}
		if r.OSMCandidateIdx != 0 {
			t.Errorf("ride[%d] OSMCandidateIdx = %d, want 0", i, r.OSMCandidateIdx)
		}
	}
}

// TestSnapToOSMLifts_SinglePartialNoSibling: coverage 0.30..0.70 but no other
// ride hits the same lift → Action="passthrough" (ride stays heuristic-only).
func TestSnapToOSMLifts_SinglePartialNoSibling(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)

	osmLift := osmLiftCandidate{
		SegmentID:  "seg-D",
		LengthM:    600.0,
		BearingDeg: 90.0,
	}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0}, // 0.5 coverage, single ride
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{osmLift}, matches, nil, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != SnapActionPassthrough {
		t.Errorf("Action = %q, want passthrough (single partial without sibling)", results[0].Action)
	}
}

// TestSnapToOSMLifts_PicksHighestCoverage: when a ride overlaps two OSM lifts,
// the snap picks the highest-coverage one (defends against parallel lift pairs).
func TestSnapToOSMLifts_PicksHighestCoverage(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)

	liftA := osmLiftCandidate{SegmentID: "seg-A", SegmentVersionID: "ver-A", LengthM: 800.0, BearingDeg: 90.0}
	liftB := osmLiftCandidate{SegmentID: "seg-B", SegmentVersionID: "ver-B", LengthM: 600.0, BearingDeg: 90.0}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 30.0, IntersectionLen: 600.0}, // 0.75 coverage on liftA (but parallel)
		{RideIdx: 0, OSMIdx: 1, HausdorffM: 5.0, IntersectionLen: 600.0},  // 1.00 coverage on liftB
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{liftA, liftB}, matches, nil, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != SnapActionSnap || results[0].SegmentID != "seg-B" {
		t.Errorf("got Action=%q SegmentID=%q, want snap to seg-B (higher coverage)", results[0].Action, results[0].SegmentID)
	}
}

// TestSnapToOSMLifts_RejectsHighHausdorff: a candidate beyond 50m hausdorff
// is dropped even at 100% coverage (safety against parallel-lift mismatch).
func TestSnapToOSMLifts_RejectsHighHausdorff(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)

	osm := osmLiftCandidate{SegmentID: "seg-X", LengthM: 600.0, BearingDeg: 90.0}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 60.0 /* > 50m cap */, IntersectionLen: 600.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{osm}, matches, nil, nil)
	if results[0].Action != SnapActionPassthrough {
		t.Errorf("expected passthrough for hausdorff>50m, got %q", results[0].Action)
	}
}

// --- helpers ---

type syntheticRide struct {
	indexed RideWithIndices
	samples []StreamPoint
}

// newSyntheticRide builds a RideWithIndices plus the underlying samples.
// The samples are projected through projectToWGS84 (anchor 47.0/12.0) so the
// haversine helpers in osm.go produce realistic metres.
func newSyntheticRide(t *testing.T, start time.Time, startX, endX, y, startAlt, endAlt float64) syntheticRide {
	t.Helper()
	dt := time.Second
	steps := int((endX - startX) / 5.0) // one sample every 5m → speed = 5 m/s
	if steps < 2 {
		steps = 2
	}
	samples := make([]StreamPoint, 0, steps+1)
	for i := 0; i <= steps; i++ {
		frac := float64(i) / float64(steps)
		x := startX + (endX-startX)*frac
		alt := startAlt + (endAlt-startAlt)*frac
		lat, lng := projectToWGS84(x, y)
		latC, lngC := lat, lng
		altC, distC := alt, x
		samples = append(samples, StreamPoint{
			Time:      start.Add(time.Duration(i) * dt),
			Lat:       &latC,
			Lng:       &lngC,
			AltitudeM: &altC,
			DistanceM: &distC,
		})
	}
	durationS := int(samples[len(samples)-1].Time.Sub(samples[0].Time).Seconds())
	return syntheticRide{
		indexed: RideWithIndices{
			Ride: LiftRide{
				StartedAt:  start,
				EndedAt:    samples[len(samples)-1].Time,
				DurationS:  durationS,
				AscentM:    endAlt - startAlt,
				DescentM:   0,
				DistanceM:  endX - startX,
				Confidence: "high",
			},
			StartSampleIdx: 0,
			EndSampleIdx:   len(samples) - 1,
		},
		samples: samples,
	}
}
