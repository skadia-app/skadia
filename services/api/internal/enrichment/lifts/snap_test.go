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

	osmLift := OSMLiftCandidate{
		SegmentID:        "seg-A",
		SegmentVersionID: "ver-A",
		LengthM:          600.0,
		BearingDeg:       90.0, // east
	}
	// Matchable: hausdorff 5m, intersection covers all 600m of C → coverage = 1.0
	matches := []RideOSMOverlap{
		{
			RideIdx:         0,
			OSMIdx:          0,
			HausdorffM:      5.0,
			IntersectionLen: 600.0,
		},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osmLift}, matches, nil, nil)

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

	osmLift := OSMLiftCandidate{
		SegmentID:        "seg-B",
		SegmentVersionID: "ver-B",
		LengthM:          5000.0, // very long lift, ride only covers tiny fraction
		BearingDeg:       90.0,
	}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 200.0 /*too far*/, IntersectionLen: 100.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osmLift}, matches, nil, nil)
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

	osmLift := OSMLiftCandidate{
		SegmentID:        "seg-C",
		SegmentVersionID: "ver-C",
		LengthM:          600.0,
		BearingDeg:       90.0,
	}
	// Each ride covers 300m of a 600m lift = 0.50 coverage individually.
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{rideA.indexed, rideB.indexed}, []OSMLiftCandidate{osmLift}, matches, nil, nil)
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
// ride hits the same lift → Action="passthrough" (ride stays heuristic-only)
// — but the ride physically rode an OSM lift, so OSMCandidateIdx must point
// at that lift to block Stage 3 (Phase-4 critic finding F1).
func TestSnapToOSMLifts_SinglePartialNoSibling(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 300, 0, 1000.0, 1050.0)

	// Two OSM lifts so we can verify OSMCandidateIdx points at the *right*
	// one (idx 1, the only one with partial coverage above the threshold).
	// Using idx 1 also distinguishes the new sentinel-aware code from the
	// old zero-value-as-default behaviour.
	osmA := OSMLiftCandidate{SegmentID: "seg-A", LengthM: 5000.0, BearingDeg: 90.0} // coverage 0.06 — below threshold
	osmB := OSMLiftCandidate{SegmentID: "seg-B", LengthM: 600.0, BearingDeg: 90.0}  // coverage 0.50 — above threshold, single ride
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 0, OSMIdx: 1, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osmA, osmB}, matches, nil, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != SnapActionPassthrough {
		t.Errorf("Action = %q, want passthrough (single partial without sibling)", results[0].Action)
	}
	if results[0].OSMCandidateIdx != 1 {
		t.Errorf("OSMCandidateIdx = %d, want 1 (passthrough must remember the OSM lift to block Stage 3)", results[0].OSMCandidateIdx)
	}
}

// TestSnapToOSMLifts_PassthroughNoOverlapHasSentinelIdx: a ride with no
// usable OSM overlap (every match below 30% coverage) must end up as
// passthrough with OSMCandidateIdx = -1 — the sentinel that signals "no OSM
// relation". This is the symmetric counterpart to the
// TestSnapToOSMLifts_SinglePartialNoSibling case: without the sentinel,
// MethodACoverageGap could not distinguish "no OSM relation" from "OSM lift
// at index 0", and Stage 3 would incorrectly skip osmCandidates[0] for every
// passthrough ride (Phase-4 critic finding F1).
func TestSnapToOSMLifts_PassthroughNoOverlapHasSentinelIdx(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)

	osm := OSMLiftCandidate{SegmentID: "seg-A", LengthM: 5000.0, BearingDeg: 90.0}
	matches := []RideOSMOverlap{
		// 100m intersection on a 5000m lift = 0.02 coverage, below threshold.
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 100.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, matches, nil, nil)
	if results[0].Action != SnapActionPassthrough {
		t.Fatalf("Action = %q, want passthrough", results[0].Action)
	}
	if results[0].OSMCandidateIdx != -1 {
		t.Errorf("OSMCandidateIdx = %d, want -1 (no OSM relation)", results[0].OSMCandidateIdx)
	}
}

// TestSnapToOSMLifts_PicksHighestCoverage: when a ride overlaps two OSM lifts,
// the snap picks the highest-coverage one (defends against parallel lift pairs).
func TestSnapToOSMLifts_PicksHighestCoverage(t *testing.T) {
	startTime := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, startTime, 0, 600, 0, 1000.0, 1100.0)

	liftA := OSMLiftCandidate{SegmentID: "seg-A", SegmentVersionID: "ver-A", LengthM: 800.0, BearingDeg: 90.0}
	liftB := OSMLiftCandidate{SegmentID: "seg-B", SegmentVersionID: "ver-B", LengthM: 600.0, BearingDeg: 90.0}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 30.0, IntersectionLen: 600.0}, // 0.75 coverage on liftA (but parallel)
		{RideIdx: 0, OSMIdx: 1, HausdorffM: 5.0, IntersectionLen: 600.0},  // 1.00 coverage on liftB
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{liftA, liftB}, matches, nil, nil)
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

	osm := OSMLiftCandidate{SegmentID: "seg-X", LengthM: 600.0, BearingDeg: 90.0}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 60.0 /* > 50m cap */, IntersectionLen: 600.0},
	}

	results := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, matches, nil, nil)
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

	osmLift := OSMLiftCandidate{
		SegmentID:        "seg-merge",
		SegmentVersionID: "ver-merge",
		LengthM:          600.0,
		BearingDeg:       90.0,
	}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]OSMLiftCandidate{osmLift},
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
		[]OSMLiftCandidate{osmLift},
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

	osmLift := OSMLiftCandidate{
		SegmentID: "seg-mid", LengthM: 600.0, BearingDeg: 90.0,
	}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]OSMLiftCandidate{osmLift},
		matches, allSamples, nil,
	)
	stage2 := MergeMultiRideForOSM(
		stage1,
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]OSMLiftCandidate{osmLift},
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

	osmLift := OSMLiftCandidate{SegmentID: "seg-rev", LengthM: 600.0, BearingDeg: 90.0}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
	}
	stage1 := SnapToOSMLifts(
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]OSMLiftCandidate{osmLift},
		matches, allSamples, nil,
	)
	stage2 := MergeMultiRideForOSM(
		stage1,
		[]RideWithIndices{rideA.indexed, rideBIndexed},
		[]OSMLiftCandidate{osmLift},
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
	osm := OSMLiftCandidate{SegmentID: "seg-A", LengthM: 600.0, BearingDeg: 90.0}
	matches := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 600.0},
	}
	stage1 := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, matches, ride.samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, []RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, matches, ride.samples)
	if stage2[0].Action != SnapActionSnap {
		t.Errorf("Stage 2 must preserve Stage 1 snap; got %q", stage2[0].Action)
	}
	if stage2[0].IsCableHoldMerge {
		t.Error("Stage 1 snap must NOT be marked as cable-hold-merge")
	}
}

// --- Stage 3: MethodACoverageGap tests ---

// TestMethodACoverageGap_PlansUncoveredOSMLifts: an OSM lift that no Stage-1
// or Stage-2 ride snapped to is returned as a Stage-3 plan entry — caller
// then runs the per-lift time-window SQL.
func TestMethodACoverageGap_PlansUncoveredOSMLifts(t *testing.T) {
	osmA := OSMLiftCandidate{OSMID: 100, SegmentID: "seg-A", LengthM: 600.0, BearingDeg: 90.0}
	osmB := OSMLiftCandidate{OSMID: 101, SegmentID: "seg-B", LengthM: 200.0, BearingDeg: 180.0}

	// Stage 1+2 produced a snap for osmA only.
	finalResults := []SnapResult{
		{RideIdx: 0, Action: SnapActionSnap, OSMCandidateIdx: 0, SegmentID: "seg-A"},
	}
	plan := MethodACoverageGap(finalResults, []OSMLiftCandidate{osmA, osmB})
	if len(plan) != 1 {
		t.Fatalf("plan len = %d, want 1 (only osmB uncovered)", len(plan))
	}
	if plan[0].OSMCandidateIdx != 1 {
		t.Errorf("plan[0].OSMCandidateIdx = %d, want 1 (osmB)", plan[0].OSMCandidateIdx)
	}
	if !plan[0].IsMethodAOnly {
		t.Error("plan[0].IsMethodAOnly must be true (Stage 3 is by definition Method-A-only)")
	}
	if !plan[0].IsLiftSegmentMatch {
		t.Error("plan[0].IsLiftSegmentMatch must be true")
	}
}

// TestMethodACoverageGap_NoPlanWhenAllSnapped: when every OSM lift was
// covered (single ride per lift, both at >=70%) Stage 3 returns nothing.
func TestMethodACoverageGap_NoPlanWhenAllSnapped(t *testing.T) {
	osmA := OSMLiftCandidate{OSMID: 100, SegmentID: "seg-A", LengthM: 600.0}
	osmB := OSMLiftCandidate{OSMID: 101, SegmentID: "seg-B", LengthM: 200.0}
	finalResults := []SnapResult{
		{RideIdx: 0, Action: SnapActionSnap, OSMCandidateIdx: 0, SegmentID: "seg-A"},
		{RideIdx: 1, Action: SnapActionSnap, OSMCandidateIdx: 1, SegmentID: "seg-B"},
	}
	plan := MethodACoverageGap(finalResults, []OSMLiftCandidate{osmA, osmB})
	if len(plan) != 0 {
		t.Errorf("plan len = %d, want 0", len(plan))
	}
}

// TestMethodACoverageGap_PassthroughWithoutOSMRelationStillPlansLift: a ride
// that fell through to passthrough without ever touching an OSM lift
// (OSMCandidateIdx = -1) does NOT count as covering any OSM lift in the
// neighbourhood, so Stage 3 still proposes the unrelated lifts.
//
// This test replaces the older TestMethodACoverageGap_PassthroughDoesNotCoverOSMLift
// (Phase-4 critic finding F1). The old test asserted that *any* passthrough,
// including one whose OSMCandidateIdx defaulted to 0 from the SnapResult
// zero-value, left the lift uncovered. That contract caused recomputeLiftStats
// to double-count: the heuristic detector inserted an activity_lift_candidates
// row for the partial-coverage ride AND Stage 3 inserted an OSM-side
// segment_effort for the same physical ride. The fix makes OSMCandidateIdx an
// explicit sentinel (-1 = no OSM relation, >= 0 = ride physically corresponds
// to that lift), and MethodACoverageGap treats every >= 0 value as covered
// regardless of Action. This test now exercises the no-relation case to make
// sure unrelated OSM lifts are still planned.
func TestMethodACoverageGap_PassthroughWithoutOSMRelationStillPlansLift(t *testing.T) {
	osmA := OSMLiftCandidate{OSMID: 100, SegmentID: "seg-A", LengthM: 600.0}
	finalResults := []SnapResult{
		// OSMCandidateIdx = -1 means the snap pipeline never associated this
		// ride with any OSM lift (no overlap above snapPartialCoverage).
		{RideIdx: 0, Action: SnapActionPassthrough, OSMCandidateIdx: -1},
	}
	plan := MethodACoverageGap(finalResults, []OSMLiftCandidate{osmA})
	if len(plan) != 1 {
		t.Fatalf("plan len = %d, want 1 (passthrough without OSM relation does not cover the lift)", len(plan))
	}
}

// TestMethodACoverageGap_PassthroughWithPartialCoverageBlocksStage3: a
// passthrough ride whose Stage-1 best coverage was 0.30..0.70 with no sibling
// (and therefore stayed Action=Passthrough but kept OSMCandidateIdx pointing
// at the OSM lift it physically rode on) MUST block Stage 3 — otherwise Stage
// 3 inserts an OSM-side segment_effort for the same physical ride that the
// heuristic detector already emitted as a candidate, double-counting in
// recomputeLiftStats. (Phase-4 critic finding F1.)
func TestMethodACoverageGap_PassthroughWithPartialCoverageBlocksStage3(t *testing.T) {
	osmA := OSMLiftCandidate{OSMID: 100, SegmentID: "seg-A", LengthM: 600.0}
	osmB := OSMLiftCandidate{OSMID: 101, SegmentID: "seg-B", LengthM: 200.0}
	finalResults := []SnapResult{
		// Lone partial-coverage passthrough on osmA (idx 0).
		{RideIdx: 0, Action: SnapActionPassthrough, OSMCandidateIdx: 0},
	}
	plan := MethodACoverageGap(finalResults, []OSMLiftCandidate{osmA, osmB})
	if len(plan) != 1 {
		t.Fatalf("plan len = %d, want 1 (only osmB uncovered)", len(plan))
	}
	if plan[0].OSMCandidateIdx != 1 {
		t.Errorf("plan[0].OSMCandidateIdx = %d, want 1 (osmB; osmA is blocked by the passthrough+partial-coverage ride)", plan[0].OSMCandidateIdx)
	}
}

// TestMethodACoverageGap_SuppressedSiblingsAlsoCount: when Stage 2 merges
// rideA+rideB into one snap on OSM idx 0, both the canonical and the
// SuppressedByMerge sibling reference OSMCandidateIdx=0 — Stage 3 should
// see the lift as covered exactly once.
func TestMethodACoverageGap_SuppressedSiblingsAlsoCount(t *testing.T) {
	osmA := OSMLiftCandidate{OSMID: 100, SegmentID: "seg-merge", LengthM: 600.0}
	finalResults := []SnapResult{
		{RideIdx: 0, Action: SnapActionSnap, OSMCandidateIdx: 0, IsCableHoldMerge: true,
			MergedRideIndices: []int{0, 1}, SegmentID: "seg-merge"},
		{RideIdx: 1, Action: SnapActionSnap, OSMCandidateIdx: 0, IsCableHoldMerge: true,
			SuppressedByMerge: true, SegmentID: "seg-merge"},
	}
	plan := MethodACoverageGap(finalResults, []OSMLiftCandidate{osmA})
	if len(plan) != 0 {
		t.Errorf("plan len = %d, want 0 (lift covered by the merged group)", len(plan))
	}
}

// --- Snap-pipeline corpus contract tests ---

func TestSnapCorpus_FullSnapStage1(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, start, 0, 600, 0, 1000, 1120)
	osm := OSMLiftCandidate{OSMID: 1, SegmentID: "seg-full", SegmentVersionID: "ver-full", LengthM: 600, BearingDeg: 90}
	overlaps := []RideOSMOverlap{{RideIdx: 0, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 600}}

	stage1 := SnapToOSMLifts([]RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, overlaps, ride.samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, []RideWithIndices{ride.indexed}, []OSMLiftCandidate{osm}, overlaps, ride.samples)
	stage3 := MethodACoverageGap(stage2, []OSMLiftCandidate{osm})

	if stage2[0].Action != SnapActionSnap {
		t.Fatalf("Stage 1 full snap action=%q, want snap", stage2[0].Action)
	}
	if stage2[0].IsCableHoldMerge {
		t.Fatalf("Stage 1 full snap must not be a cable-hold merge")
	}
	if len(stage3) != 0 {
		t.Fatalf("Stage 3 plan len=%d, want 0 for covered OSM lift", len(stage3))
	}
}

func TestSnapCorpus_MultiSectionSplitNoMerge(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, start, 0, 200, 0, 1000, 1040)
	rideB := newSyntheticRide(t, start.Add(5*time.Minute), 250, 450, 0, 1040, 1080)
	rideC := newSyntheticRide(t, start.Add(10*time.Minute), 500, 700, 0, 1080, 1120)
	rides, samples := combineSyntheticRides(rideA, rideB, rideC)
	osm := []OSMLiftCandidate{
		{OSMID: 10, SegmentID: "seg-a", SegmentVersionID: "ver-a", LengthM: 200, BearingDeg: 90},
		{OSMID: 11, SegmentID: "seg-b", SegmentVersionID: "ver-b", LengthM: 200, BearingDeg: 90},
		{OSMID: 12, SegmentID: "seg-c", SegmentVersionID: "ver-c", LengthM: 200, BearingDeg: 90},
	}
	overlaps := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 200},
		{RideIdx: 1, OSMIdx: 1, HausdorffM: 5, IntersectionLen: 200},
		{RideIdx: 2, OSMIdx: 2, HausdorffM: 5, IntersectionLen: 200},
	}

	stage1 := SnapToOSMLifts(rides, osm, overlaps, samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, rides, osm, overlaps, samples)
	stage3 := MethodACoverageGap(stage2, osm)

	for i, res := range stage2 {
		if res.Action != SnapActionSnap {
			t.Fatalf("ride[%d] action=%q, want snap to its own section", i, res.Action)
		}
		if res.IsCableHoldMerge {
			t.Fatalf("ride[%d] unexpectedly marked as cable-hold merge", i)
		}
		if res.OSMCandidateIdx != i {
			t.Fatalf("ride[%d] OSMCandidateIdx=%d, want %d", i, res.OSMCandidateIdx, i)
		}
	}
	if len(stage3) != 0 {
		t.Fatalf("Stage 3 plan len=%d, want 0 for three covered sections", len(stage3))
	}
}

func TestSnapCorpus_CableHoldMerge(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, start, 0, 300, 0, 1000, 1060)
	rideB := newSyntheticRide(t, start.Add(5*time.Minute), 310, 600, 0, 1060, 1120)
	rides, samples := combineSyntheticRides(rideA, rideB)
	osm := OSMLiftCandidate{OSMID: 20, SegmentID: "seg-hold", SegmentVersionID: "ver-hold", LengthM: 600, BearingDeg: 90}
	overlaps := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 300},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 300},
	}

	stage1 := SnapToOSMLifts(rides, []OSMLiftCandidate{osm}, overlaps, samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, rides, []OSMLiftCandidate{osm}, overlaps, samples)
	stage3 := MethodACoverageGap(stage2, []OSMLiftCandidate{osm})

	if stage2[0].Action != SnapActionSnap || !stage2[0].IsCableHoldMerge {
		t.Fatalf("canonical action/merge=%q/%v, want snap/true", stage2[0].Action, stage2[0].IsCableHoldMerge)
	}
	if !stage2[1].SuppressedByMerge {
		t.Fatalf("second ride must be an audit-only suppressed merge sibling")
	}
	if len(stage3) != 0 {
		t.Fatalf("Stage 3 plan len=%d, want 0 for merged OSM lift", len(stage3))
	}
}

func TestSnapCorpus_MidStationFalseMergeDoesNotSnap(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, start, 0, 300, 0, 1000, 1060)
	rideB := newSyntheticRide(t, start.Add(5*time.Minute), 360, 60, 0, 1060, 1120)
	rides, samples := combineSyntheticRides(rideA, rideB)
	osm := OSMLiftCandidate{OSMID: 30, SegmentID: "seg-mid", SegmentVersionID: "ver-mid", LengthM: 600, BearingDeg: 90}
	overlaps := []RideOSMOverlap{
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 300},
		{RideIdx: 1, OSMIdx: 0, HausdorffM: 5, IntersectionLen: 300},
	}

	stage1 := SnapToOSMLifts(rides, []OSMLiftCandidate{osm}, overlaps, samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, rides, []OSMLiftCandidate{osm}, overlaps, samples)
	stage3 := MethodACoverageGap(stage2, []OSMLiftCandidate{osm})

	for i, res := range stage2 {
		if res.Action != SnapActionPassthrough {
			t.Fatalf("ride[%d] action=%q, want passthrough after mid-station rejection", i, res.Action)
		}
		if res.IsCableHoldMerge {
			t.Fatalf("ride[%d] must not be marked as cable-hold merge", i)
		}
	}
	if len(stage3) != 0 {
		t.Fatalf("Stage 3 plan len=%d, want 0: Stage 2 mid-station rejection must not re-snap", len(stage3))
	}
}

func TestSnapCorpus_OSMOnlyCatchStage3(t *testing.T) {
	osm := OSMLiftCandidate{OSMID: 40, SegmentID: "seg-osm-only", SegmentVersionID: "ver-osm-only", LengthM: 120, BearingDeg: 90}

	stage3 := MethodACoverageGap(nil, []OSMLiftCandidate{osm})

	if len(stage3) != 1 {
		t.Fatalf("Stage 3 plan len=%d, want 1 for OSM-only catch", len(stage3))
	}
	if !stage3[0].IsLiftSegmentMatch || !stage3[0].IsMethodAOnly {
		t.Fatalf("Stage 3 flags = lift_match:%v method_a_only:%v, want both true",
			stage3[0].IsLiftSegmentMatch, stage3[0].IsMethodAOnly)
	}
}

func TestSnapCorpus_HeuristicOnlyNoOSM(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, start, 0, 600, 0, 1000, 1120)

	stage1 := SnapToOSMLifts([]RideWithIndices{ride.indexed}, nil, nil, ride.samples, nil)
	stage2 := MergeMultiRideForOSM(stage1, []RideWithIndices{ride.indexed}, nil, nil, ride.samples)
	stage3 := MethodACoverageGap(stage2, nil)

	if len(stage2) != 1 {
		t.Fatalf("stage2 len=%d, want 1 passthrough ride", len(stage2))
	}
	if stage2[0].Action != SnapActionPassthrough {
		t.Fatalf("no-OSM action=%q, want passthrough", stage2[0].Action)
	}
	if len(stage3) != 0 {
		t.Fatalf("Stage 3 plan len=%d, want 0 without OSM candidates", len(stage3))
	}
}

// TestComputeIsReverseFromSamples_MalformedIndicesDoesNotPanic exercises
// every degenerate index combination against a non-empty samples slice
// (Phase-4 critic finding F19). The previous asymmetric bounds check would
// panic on (StartSampleIdx >= len, EndSampleIdx in range) and similar
// inversions. The function must return false instead.
func TestComputeIsReverseFromSamples_MalformedIndicesDoesNotPanic(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, start, 0, 600, 0, 1000, 1100)

	cases := []struct {
		name string
		idx  RideWithIndices
	}{
		{"start_negative", RideWithIndices{StartSampleIdx: -1, EndSampleIdx: 5}},
		{"end_negative", RideWithIndices{StartSampleIdx: 0, EndSampleIdx: -1}},
		{"start_out_of_range", RideWithIndices{StartSampleIdx: len(ride.samples), EndSampleIdx: len(ride.samples) - 1}},
		{"end_out_of_range", RideWithIndices{StartSampleIdx: 0, EndSampleIdx: len(ride.samples) + 5}},
		{"start_after_end", RideWithIndices{StartSampleIdx: 10, EndSampleIdx: 5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("computeIsReverseFromSamples panicked: %v", r)
				}
			}()
			got := computeIsReverseFromSamples(ride.samples, tc.idx, 90.0)
			if got {
				t.Errorf("got IsReverse=true for malformed indices, want false (assume same direction)")
			}
		})
	}
}

// TestCableHoldSignalsHold_MalformedIndicesDoesNotPanic mirrors the F19
// guard for cableHoldSignalsHold. The Stage-2 helper must reject any merge
// where either ride has out-of-range or inverted indices instead of panicking
// when it dereferences samples[ra.StartSampleIdx] / samples[rb.EndSampleIdx].
func TestCableHoldSignalsHold_MalformedIndicesDoesNotPanic(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	ride := newSyntheticRide(t, start, 0, 600, 0, 1000, 1100)
	good := ride.indexed
	cases := []struct {
		name string
		ra   RideWithIndices
		rb   RideWithIndices
	}{
		{"ra_start_oor", RideWithIndices{StartSampleIdx: len(ride.samples), EndSampleIdx: len(ride.samples) - 1}, good},
		{"ra_end_negative", RideWithIndices{StartSampleIdx: 0, EndSampleIdx: -1}, good},
		{"rb_end_oor", good, RideWithIndices{StartSampleIdx: 0, EndSampleIdx: len(ride.samples) + 5}},
		{"rb_start_negative", good, RideWithIndices{StartSampleIdx: -1, EndSampleIdx: 5}},
		{"ra_inverted", RideWithIndices{StartSampleIdx: 10, EndSampleIdx: 5}, good},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("cableHoldSignalsHold panicked: %v", r)
				}
			}()
			got := cableHoldSignalsHold(tc.ra, tc.rb, []RideWithIndices{tc.ra, tc.rb}, ride.samples)
			if got {
				t.Errorf("got cable-hold=true on malformed indices, want false (conservative)")
			}
		})
	}
}

// TestMergeMultiRideForOSM_CableHoldSurvivesForeignRideSamples exercises
// Phase-4 critic finding F26: when an unrelated ride (different OSM lift)
// chronologically lies between the two cable-hold rides, its GPS samples
// were previously checked against the anchor's 10m drift and would always
// fail — rejecting the legitimate cable-hold merge. The fix skips foreign-
// ride samples in the gap-pattern loop so the merge succeeds.
func TestMergeMultiRideForOSM_CableHoldSurvivesForeignRideSamples(t *testing.T) {
	start := time.Unix(0, 0).UTC()
	rideA := newSyntheticRide(t, start, 0, 300, 0, 1000.0, 1050.0)
	// rideMid sits 500m to the south at startY = -500. Its samples are far
	// from rideA's anchor — without the foreign-ride skip they would force
	// the cable-hold merge to fail.
	rideMid := newSyntheticRide(t, start.Add(60*time.Second), 0, 300, -500, 1000.0, 1050.0)
	rideB := newSyntheticRide(t, start.Add(180*time.Second), 310, 600, 0, 1052.0, 1100.0)

	// Concatenate samples in chronological order: A, Mid, B.
	allSamples := append([]StreamPoint{}, rideA.samples...)
	midOffset := len(allSamples)
	allSamples = append(allSamples, rideMid.samples...)
	bOffset := len(allSamples)
	allSamples = append(allSamples, rideB.samples...)

	rideAIdx := rideA.indexed
	rideMidIdx := rideMid.indexed
	rideMidIdx.StartSampleIdx = midOffset
	rideMidIdx.EndSampleIdx = midOffset + len(rideMid.samples) - 1
	rideBIdx := rideB.indexed
	rideBIdx.StartSampleIdx = bOffset
	rideBIdx.EndSampleIdx = bOffset + len(rideB.samples) - 1

	osmCableHold := OSMLiftCandidate{
		SegmentID: "seg-merge", SegmentVersionID: "ver-merge",
		LengthM: 600.0, BearingDeg: 90.0,
	}
	osmMid := OSMLiftCandidate{
		SegmentID: "seg-mid", SegmentVersionID: "ver-mid",
		LengthM: 300.0, BearingDeg: 90.0,
	}
	matches := []RideOSMOverlap{
		// rideA + rideB hold OSM lift 0.
		{RideIdx: 0, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		{RideIdx: 2, OSMIdx: 0, HausdorffM: 5.0, IntersectionLen: 300.0},
		// rideMid is its own lift — no overlap with OSM 0.
		{RideIdx: 1, OSMIdx: 1, HausdorffM: 5.0, IntersectionLen: 300.0},
	}

	rides := []RideWithIndices{rideAIdx, rideMidIdx, rideBIdx}
	osmCandidates := []OSMLiftCandidate{osmCableHold, osmMid}

	stage1 := SnapToOSMLifts(rides, osmCandidates, matches, allSamples, nil)
	stage2 := MergeMultiRideForOSM(stage1, rides, osmCandidates, matches, allSamples)

	canonical := stage2[0]
	if canonical.Action != SnapActionSnap {
		t.Fatalf("rideA action=%q, want snap (cable-hold merge must succeed despite foreign ride between)", canonical.Action)
	}
	if !canonical.IsCableHoldMerge {
		t.Fatalf("rideA IsCableHoldMerge=false, want true")
	}
	suppressed := stage2[2]
	if !suppressed.SuppressedByMerge {
		t.Fatalf("rideB SuppressedByMerge=false, want true")
	}
}

// TestMergeMultiRideForOSM_LoneMemberDefensiveBlocksStage3 exercises
// Phase-4 critic finding F39: if a Stage-1 result tags exactly one ride as
// MultiRideGroup (the defensive branch in MergeMultiRideForOSM), Stage 2
// must still prevent Stage 3 from re-planning the same OSM lift, otherwise
// the consumer's defensive heuristic-only insert plus a Stage 3 OSM-side
// segment_effort double-count the same physical ride.
func TestMergeMultiRideForOSM_LoneMemberDefensiveBlocksStage3(t *testing.T) {
	osm := OSMLiftCandidate{OSMID: 50, SegmentID: "seg-lone", LengthM: 600.0, BearingDeg: 90.0}

	// Hand-crafted Stage-1 output: a single MultiRideGroup ride. (Stage 1
	// can't actually emit this configuration today — it requires
	// ridesPerOSM[osmIdx] >= 2 — but Stage 2 must still cope defensively if
	// upstream code ever produces it.)
	stage1 := []SnapResult{
		{RideIdx: 0, Action: SnapActionMultiRideGroup, OSMCandidateIdx: 0},
	}
	rides := []RideWithIndices{{StartSampleIdx: 0, EndSampleIdx: 10}}
	stage2 := MergeMultiRideForOSM(stage1, rides, []OSMLiftCandidate{osm}, nil, nil)

	if stage2[0].Action != SnapActionMultiRideGroup {
		t.Fatalf("lone defensive ride action=%q, want multi_ride_group preserved (consumer falls back to heuristic-only)", stage2[0].Action)
	}
	if !stage2[0].BlocksStage3 {
		t.Errorf("lone defensive ride BlocksStage3=false, want true (Stage 3 must not re-plan the same OSM lift)")
	}
	plan := MethodACoverageGap(stage2, []OSMLiftCandidate{osm})
	if len(plan) != 0 {
		t.Errorf("Stage 3 plan len=%d, want 0 (lone defensive ride already covers the lift)", len(plan))
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

func combineSyntheticRides(rides ...syntheticRide) ([]RideWithIndices, []StreamPoint) {
	outRides := make([]RideWithIndices, 0, len(rides))
	outSamples := []StreamPoint{}
	for _, ride := range rides {
		offset := len(outSamples)
		indexed := ride.indexed
		indexed.StartSampleIdx += offset
		indexed.EndSampleIdx += offset
		outRides = append(outRides, indexed)
		outSamples = append(outSamples, ride.samples...)
	}
	return outRides, outSamples
}
