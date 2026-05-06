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

// --- Stage 2: MergeMultiRideForOSM tests ---

// TestMergeMultiRideForOSM_CableHoldMerge: two short rides whose union covers
// >=70% of an OSM lift, with all three cable-hold signals positive
// (inter-ride distance <= 15m, all gap samples <= 10m from anchor, no
// bearing reversal) → one Snap (canonical, IsCableHoldMerge=true) plus one
// SuppressedByMerge sibling pointing at the same OSM lift.
func TestMergeMultiRideForOSM_CableHoldMerge(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)
	// Cable-hold: rideB starts ~10m past rideA's end, with no samples in
	// between (smart-recording paused) — same direction (east).
	rideB := newSyntheticRide(t, startTime.Add(180*time.Second), 310, 600, 0, 1052.0, 1100.0)

	// Concatenate samples; remap rideB's indices to point past rideA.
	allSamples := append([]StreamPoint{}, rideA.samples...)
	rideBOffset := len(allSamples)
	allSamples = append(allSamples, rideB.samples...)
	rideBIndexed := rideB.indexed
	rideBIndexed.StartSampleIdx = rideBOffset
	rideBIndexed.EndSampleIdx = rideBOffset + len(rideB.samples) - 1

	osmLift := osmLiftCandidate{
		SegmentID:        "seg-merge",
		SegmentVersionID: "ver-merge",
		LengthM:          600.0,
		BearingDeg:       90.0,
	}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples, nil,
	)
	// Sanity: Stage 1 marked both as multi-ride-group.
	for i, r := range stage1 {
		if r.Action != SnapActionMultiRideGroup {
			t.Fatalf("ride[%d] Stage 1 action = %q, want multi_ride_group", i, r.Action)
		}
	}

	stage2 := MergeMultiRideForOSM(
		stage1,
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples,
	)

	if len(stage2) != 2 {
		t.Fatalf("stage2 len = %d, want 2", len(stage2))
	}
	canonical := stage2[0]
	suppressed := stage2[1]
	if canonical.Action != SnapActionSnap {
		t.Errorf("canonical Action = %q, want snap", canonical.Action)
	}
	if !canonical.IsCableHoldMerge {
		t.Error("expected canonical.IsCableHoldMerge = true")
	}
	if canonical.SegmentID != "seg-merge" {
		t.Errorf("canonical SegmentID = %q, want seg-merge", canonical.SegmentID)
	}
	if len(canonical.MergedRideIndices) != 2 {
		t.Errorf("canonical MergedRideIndices = %v, want [0,1]", canonical.MergedRideIndices)
	}
	if !suppressed.SuppressedByMerge {
		t.Error("expected suppressed.SuppressedByMerge = true")
	}
}

// TestMergeMultiRideForOSM_RejectsMergeWhenInterRideTooFar: two rides
// 30m apart at the join → fails inter-ride-distance signal → both fall
// through as heuristic-only (Action=Passthrough).
func TestMergeMultiRideForOSM_RejectsMergeWhenInterRideTooFar(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)
	// rideB starts 50m past rideA's end (mid-station-style platform walk).
	rideB := newSyntheticRide(t, startTime.Add(180*time.Second), 350, 600, 0, 1052.0, 1100.0)

	allSamples := append([]StreamPoint{}, rideA.samples...)
	rideBOffset := len(allSamples)
	allSamples = append(allSamples, rideB.samples...)
	rideBIndexed := rideB.indexed
	rideBIndexed.StartSampleIdx = rideBOffset
	rideBIndexed.EndSampleIdx = rideBOffset + len(rideB.samples) - 1

	osmLift := osmLiftCandidate{
		SegmentID: "seg-mid", LengthM: 600.0, BearingDeg: 90.0,
	}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples, nil,
	)
	stage2 := MergeMultiRideForOSM(
		stage1,
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples,
	)
	for i, r := range stage2 {
		if r.Action != SnapActionPassthrough {
			t.Errorf("ride[%d] action=%q, want passthrough (mid-station distance)", i, r.Action)
		}
		if r.IsCableHoldMerge {
			t.Errorf("ride[%d] should NOT be merged", i)
		}
	}
}

// TestMergeMultiRideForOSM_RejectsMergeOnDirectionReversal: same rides
// geometrically close but rideB heads west (reversed). Mid-station with
// direction change → not a cable hold.
func TestMergeMultiRideForOSM_RejectsMergeOnDirectionReversal(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)
	// rideB heads west: startX > endX
	rideB := newSyntheticRide(t, startTime.Add(180*time.Second), 310, 10, 0, 1052.0, 1100.0)

	allSamples := append([]StreamPoint{}, rideA.samples...)
	rideBOffset := len(allSamples)
	allSamples = append(allSamples, rideB.samples...)
	rideBIndexed := rideB.indexed
	rideBIndexed.StartSampleIdx = rideBOffset
	rideBIndexed.EndSampleIdx = rideBOffset + len(rideB.samples) - 1

	osmLift := osmLiftCandidate{SegmentID: "seg-rev", LengthM: 600.0, BearingDeg: 90.0}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}
	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples, nil,
	)
	stage2 := MergeMultiRideForOSM(
		stage1,
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]osmLiftCandidate{osmLift},
		matches, allSamples,
	)
	for i, r := range stage2 {
		if r.IsCableHoldMerge {
			t.Errorf("ride[%d] should NOT be merged on direction reversal", i)
		}
	}
}

// TestMergeMultiRideForOSM_PreservesStage1Snaps: a ride that Stage 1 already
// snapped (Action=Snap) must come out unchanged from Stage 2.
func TestMergeMultiRideForOSM_PreservesStage1Snaps(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)
	osm := osmLiftCandidate{SegmentID: "seg-A", LengthM: 600.0, BearingDeg: 90.0}
	matches := []rideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 600.0},
	}
	stage1 := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []osmLiftCandidate{osm}, matches, ride.samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, []RideWithIndices{ride.indexed}, []osmLiftCandidate{osm}, matches, ride.samples)
	if stage2[0].Action != SnapActionSnap {
		t.Errorf("Stage 2 must preserve Stage 1 snap; got %q", stage2[0].Action)
	}
	if stage2[0].IsCableHoldMerge {
		t.Error("Stage 1 snap must NOT be marked as cable-hold-merge")
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
