package lifts

import (
	"testing"
)

// TestMethodACoverageGapForAllOSM_ParallelLiftsGetSeparateEntries verifies that
// MethodACoverageGapForAllOSM emits one Stage3PlanEntry per OSM candidate even
// when two lifts are geometrically close (parallel). Both entries are correct
// at the planning level — the caller's time-window SQL and deduplication logic
// handle the case where both lifts produce the same sample window.
//
// This test is a regression anchor for the F14 finding: if the function were
// to silently drop one of two parallel lifts it would incorrectly miss the
// second lift's segment entry. The per-candidate insert path in
// insertStage3SegmentEffortsBatched deduplicates on started_at to avoid the
// UNIQUE (activity_id, started_at) conflict — this test verifies the planning
// layer itself does not suppress either lift.
func TestMethodACoverageGapForAllOSM_ParallelLiftsGetSeparateEntries(t *testing.T) {
	// Two parallel lifts 50m apart — both within the 75m time-window radius.
	liftA := OSMLiftCandidate{OSMID: 200, SegmentID: "seg-parallel-A", LengthM: 600.0, BearingDeg: 90.0}
	liftB := OSMLiftCandidate{OSMID: 201, SegmentID: "seg-parallel-B", LengthM: 600.0, BearingDeg: 90.0}

	plan := MethodACoverageGapForAllOSM([]OSMLiftCandidate{liftA, liftB})
	if len(plan) != 2 {
		t.Fatalf("plan len = %d, want 2 (one entry per parallel lift)", len(plan))
	}
	if plan[0].OSMCandidateIdx != 0 {
		t.Errorf("plan[0].OSMCandidateIdx = %d, want 0", plan[0].OSMCandidateIdx)
	}
	if plan[1].OSMCandidateIdx != 1 {
		t.Errorf("plan[1].OSMCandidateIdx = %d, want 1", plan[1].OSMCandidateIdx)
	}
	for i, e := range plan {
		if !e.IsMethodAOnly {
			t.Errorf("plan[%d].IsMethodAOnly = false, want true", i)
		}
		if !e.IsLiftSegmentMatch {
			t.Errorf("plan[%d].IsLiftSegmentMatch = false, want true", i)
		}
	}
}

// TestMethodACoverageGapForAllOSM_EmptyInput returns no entries for no candidates.
func TestMethodACoverageGapForAllOSM_EmptyInput(t *testing.T) {
	plan := MethodACoverageGapForAllOSM(nil)
	if len(plan) != 0 {
		t.Errorf("plan len = %d, want 0", len(plan))
	}
}
