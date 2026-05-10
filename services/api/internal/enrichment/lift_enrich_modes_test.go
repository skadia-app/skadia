package enrichment

import (
	"testing"

	"github.com/skadia-app/skadia/services/api/internal/enrichment/lifts"
)

// TestPipelineModeDispatch verifies that EffectiveLiftDetectionOpts correctly
// describes the four pipeline modes and that MethodACoverageGapForAllOSM
// produces the expected plan entries for pure-OSM mode.
//
// Full DB integration of all four modes is covered by lift_reject_persist_test.go
// and the existing enrichStreamLifts integration tests; this file focuses on
// the dispatch logic and the new helpers in isolation.
func TestPipelineModeDispatch(t *testing.T) {
	t.Parallel()

	t.Run("both_off_early_return", func(t *testing.T) {
		t.Parallel()
		opts := lifts.EffectiveLiftDetectionOpts(true, false, false, nil, nil)
		if opts.Heuristic || opts.OSMSnap {
			t.Errorf("expected both false, got heuristic=%v osm=%v", opts.Heuristic, opts.OSMSnap)
		}
	})

	t.Run("heuristic_only", func(t *testing.T) {
		t.Parallel()
		opts := lifts.EffectiveLiftDetectionOpts(true, false, true, nil, nil)
		if !opts.Heuristic {
			t.Error("expected Heuristic=true")
		}
		if opts.OSMSnap {
			t.Error("expected OSMSnap=false")
		}
	})

	t.Run("osm_only", func(t *testing.T) {
		t.Parallel()
		opts := lifts.EffectiveLiftDetectionOpts(true, true, false, nil, nil)
		if opts.Heuristic {
			t.Error("expected Heuristic=false")
		}
		if !opts.OSMSnap {
			t.Error("expected OSMSnap=true")
		}
	})

	t.Run("both_on", func(t *testing.T) {
		t.Parallel()
		opts := lifts.EffectiveLiftDetectionOpts(true, true, true, nil, nil)
		if !opts.Heuristic || !opts.OSMSnap {
			t.Errorf("expected both true, got heuristic=%v osm=%v", opts.Heuristic, opts.OSMSnap)
		}
	})
}

// TestMethodACoverageGapForAllOSM verifies that the pure-OSM variant emits one
// Stage3PlanEntry per candidate with no covered-map filter.
func TestMethodACoverageGapForAllOSM(t *testing.T) {
	t.Parallel()

	candidates := []lifts.OSMLiftCandidate{
		{OSMID: 1, SegmentID: "s1", SegmentVersionID: "sv1"},
		{OSMID: 2, SegmentID: "s2", SegmentVersionID: "sv2"},
		{OSMID: 3, SegmentID: "s3", SegmentVersionID: "sv3"},
	}

	plan := lifts.MethodACoverageGapForAllOSM(candidates)

	if len(plan) != len(candidates) {
		t.Fatalf("expected %d plan entries, got %d", len(candidates), len(plan))
	}
	for i, e := range plan {
		if e.OSMCandidateIdx != i {
			t.Errorf("entry %d: OSMCandidateIdx=%d, want %d", i, e.OSMCandidateIdx, i)
		}
		if !e.IsLiftSegmentMatch {
			t.Errorf("entry %d: IsLiftSegmentMatch should be true", i)
		}
		if !e.IsMethodAOnly {
			t.Errorf("entry %d: IsMethodAOnly should be true", i)
		}
	}
}

// TestMethodACoverageGapForAllOSM_Empty verifies that nil/empty input returns
// an empty (not nil) plan — the caller range-loops over the result.
func TestMethodACoverageGapForAllOSM_Empty(t *testing.T) {
	t.Parallel()

	plan := lifts.MethodACoverageGapForAllOSM(nil)
	if plan == nil {
		t.Error("expected non-nil slice for nil input")
	}
	if len(plan) != 0 {
		t.Errorf("expected empty slice, got len=%d", len(plan))
	}

	plan2 := lifts.MethodACoverageGapForAllOSM([]lifts.OSMLiftCandidate{})
	if len(plan2) != 0 {
		t.Errorf("expected empty slice for empty input, got len=%d", len(plan2))
	}
}

// TestSnapStagesNilSafe verifies that Stage 1 and Stage 2 are nil-safe when
// rides=nil (pure-OSM mode). No panics must occur.
func TestSnapStagesNilSafe(t *testing.T) {
	t.Parallel()

	candidates := []lifts.OSMLiftCandidate{
		{OSMID: 1, SegmentID: "s1", SegmentVersionID: "sv1", LengthM: 100, BearingDeg: 0},
	}

	// Stage 1 with nil rides: must return empty result, not panic.
	stage1 := lifts.SnapToOSMLifts(nil, candidates, nil, nil, nil)
	if len(stage1) != 0 {
		t.Errorf("Stage 1 with nil rides: expected empty result, got %d", len(stage1))
	}

	// Stage 2 with nil input: must return empty result, not panic.
	stage2 := lifts.MergeMultiRideForOSM(nil, nil, candidates, nil, nil)
	if len(stage2) != 0 {
		t.Errorf("Stage 2 with nil input: expected empty result, got %d", len(stage2))
	}
}
